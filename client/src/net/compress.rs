//! STAGE-246：应用层压缩的解压，与服务端 `server/protocol/compress.go` 保持一致。
//!
//! 服务端对 hello 里声明了 `"compress":["gzip"]` 的连接，把超过 10 KiB 的消息发成一行
//! `{"z":"gzip","d":"<base64 of gzip(原文)>"}`。收到的行先过 [`unwrap_line`]：普通消息原样返回，
//! 压缩行解压成原文再交给协议解析。解压后的原文上限 1 MiB，防止解压炸弹。

use std::borrow::Cow;
use std::io::{Read, Write};

use base64::Engine;
use base64::engine::general_purpose::STANDARD;
use flate2::Compression;
use flate2::read::GzDecoder;
use flate2::write::GzEncoder;
use serde_json::Value;

/// 客户端声明并接受的压缩方式，hello 的 `compress` 数组里写它。
pub const GZIP: &str = "gzip";

/// 解压后原文的上限（字节），与服务端 `protocol.MaxInflatedBytes` 相同。
pub const MAX_INFLATED: usize = 1 << 20;

/// 收到的一行如果是压缩行，返回解压后的原文；普通行原样借用返回。
/// 压缩行本身损坏（方式不认识、base64 错、gzip 坏了、超过上限）时返回错误，调用方按协议错误断开连接。
pub fn unwrap_line(text: &str) -> Result<Cow<'_, str>, String> {
    if !text.contains("\"z\"") {
        return Ok(Cow::Borrowed(text));
    }
    let Ok(v) = serde_json::from_str::<Value>(text) else {
        return Ok(Cow::Borrowed(text));
    };
    // 带 type 的是普通消息；z 不是字符串也不是压缩行（与服务端一致）。
    let Some(codec) = v.get("z").and_then(Value::as_str) else {
        return Ok(Cow::Borrowed(text));
    };
    if v.get("type").is_some() {
        return Ok(Cow::Borrowed(text));
    }
    if codec != GZIP {
        return Err(format!("不支持的压缩方式：{codec}"));
    }
    let d = v.get("d").and_then(Value::as_str).ok_or("压缩消息缺少 d 字段")?;
    let raw = STANDARD
        .decode(d)
        .map_err(|_| "压缩消息的 d 不是合法的 base64".to_string())?;
    let mut out = Vec::new();
    GzDecoder::new(raw.as_slice())
        .take(MAX_INFLATED as u64 + 1)
        .read_to_end(&mut out)
        .map_err(|_| "压缩消息无法解压".to_string())?;
    if out.len() > MAX_INFLATED {
        return Err("压缩消息解压后超过 1 MiB".to_string());
    }
    String::from_utf8(out)
        .map(Cow::Owned)
        .map_err(|_| "压缩消息解压后不是合法的 UTF-8".to_string())
}

/// 把一行原文压缩成压缩行（结尾不带换行）。模拟服务端用它发压缩消息。
pub fn gzip_line(plain: &str) -> String {
    let mut enc = GzEncoder::new(Vec::new(), Compression::default());
    let _ = enc.write_all(plain.as_bytes());
    let gz = enc.finish().unwrap_or_default();
    serde_json::json!({"z": GZIP, "d": STANDARD.encode(gz)}).to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn plain_lines_pass_through_borrowed() {
        for line in [
            r#"{"id":1,"type":"ping","ok":true,"data":{}}"#,
            // 带 type 的行即使有 z 字段，也是普通消息。
            r#"{"id":2,"type":"x","z":"gzip","d":"!!"}"#,
            // z 不是字符串：不是压缩行。
            r#"{"z":1,"id":3}"#,
            "不是 JSON \"z\"",
        ] {
            match unwrap_line(line) {
                Ok(Cow::Borrowed(s)) => assert_eq!(s, line),
                other => panic!("普通行应原样借用返回，实际 {other:?}"),
            }
        }
    }

    #[test]
    fn gzip_round_trip() {
        let plain = format!(
            r#"{{"id":1,"type":"status","ok":true,"data":{{"text":"{}"}}}}"#,
            "石器".repeat(3000)
        );
        let line = gzip_line(&plain);
        assert!(line.len() < plain.len(), "重复内容压缩后应变小");
        assert!(line.starts_with(r#"{"d":"#) || line.starts_with(r#"{"z":"#));
        let got = unwrap_line(&line).expect("应能解压");
        assert_eq!(got, plain);
    }

    #[test]
    fn broken_compressed_lines_are_errors() {
        let not_gzip = format!(r#"{{"z":"gzip","d":"{}"}}"#, STANDARD.encode(b"hello"));
        let limit_plus_one = gzip_line(&"a".repeat(MAX_INFLATED + 1));
        let cases = [
            (r#"{"z":"zstd","d":"AAAA"}"#, "不支持的压缩方式"),
            (r#"{"z":"gzip"}"#, "缺少 d"),
            (r#"{"z":"gzip","d":"!!!"}"#, "base64"),
            (not_gzip.as_str(), "无法解压"),
            (limit_plus_one.as_str(), "超过 1 MiB"),
        ];
        for (line, want) in cases {
            let err = unwrap_line(line).expect_err("坏的压缩行应当报错");
            assert!(err.contains(want), "错误应包含 {want:?}，实际 {err:?}");
        }
    }
}
