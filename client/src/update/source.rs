//! 查询 GitHub Releases 的最新版本（STAGE-99）。只读，不下载安装包。

use std::time::Duration;

use semver::Version;

use super::version::parse_tag;

/// 发布仓库的「最新发布」接口；GitHub 的 `/latest` 已排除预发布和草稿。
pub const LATEST_RELEASE_URL: &str = "https://api.github.com/repos/athlan20/stoneage/releases/latest";

/// 测试或内网镜像用的环境变量，设置后代替 `LATEST_RELEASE_URL`。
pub const URL_ENV: &str = "STONEAGE_UPDATE_URL";

/// 最新发布接口的地址：有 `STONEAGE_UPDATE_URL` 用它，否则用 GitHub。
pub fn latest_url() -> String {
    std::env::var(URL_ENV)
        .ok()
        .filter(|u| !u.is_empty())
        .unwrap_or_else(|| LATEST_RELEASE_URL.to_string())
}

/// 请求最新发布并解析出版本号。超时、网络错误、HTTP 错误、返回内容不对都作为 `Err`。
pub fn fetch_latest(url: &str, timeout: Duration) -> Result<Version, String> {
    let agent: ureq::Agent = ureq::Agent::config_builder()
        .timeout_global(Some(timeout))
        .user_agent(concat!("stoneage-client/", env!("CARGO_PKG_VERSION")))
        .build()
        .into();
    let mut resp = agent
        .get(url)
        .header("Accept", "application/vnd.github+json")
        .call()
        .map_err(|e| e.to_string())?;
    let body = resp.body_mut().read_to_string().map_err(|e| e.to_string())?;
    parse_release(&body)
}

/// 从「最新发布」的 JSON 里取出版本号。草稿或预发布不算，标签不是版本号也不算。
pub fn parse_release(body: &str) -> Result<Version, String> {
    let json: serde_json::Value = serde_json::from_str(body).map_err(|e| format!("发布信息不是合法的 JSON：{e}"))?;
    if json["draft"].as_bool().unwrap_or(false) || json["prerelease"].as_bool().unwrap_or(false) {
        return Err("最新发布是预发布版本".into());
    }
    let tag = json["tag_name"].as_str().ok_or("发布信息里没有 tag_name")?;
    parse_tag(tag).ok_or_else(|| format!("无法识别的版本标签：{tag}"))
}

#[cfg(test)]
mod tests {
    use std::net::TcpListener;
    use std::thread;
    use std::time::Instant;

    use super::super::tests::serve_once;
    use super::*;

    #[test]
    fn parse_release_reads_tag() {
        let v = parse_release(r#"{"tag_name":"v0.3.1","prerelease":false,"draft":false}"#).unwrap();
        assert_eq!(v.to_string(), "0.3.1");
    }

    #[test]
    fn parse_release_rejects_prerelease_draft_and_bad_tag() {
        assert!(parse_release(r#"{"tag_name":"v0.3.1","prerelease":true}"#).is_err());
        assert!(parse_release(r#"{"tag_name":"v0.3.1","draft":true}"#).is_err());
        assert!(parse_release(r#"{"tag_name":"nightly"}"#).is_err());
        assert!(parse_release(r#"{"name":"no tag"}"#).is_err());
        assert!(parse_release("not json").is_err());
    }

    #[test]
    fn fetch_latest_reads_local_release() {
        let url = serve_once("200 OK", r#"{"tag_name":"v9.0.0"}"#);
        let v = fetch_latest(&url, Duration::from_secs(5)).unwrap();
        assert_eq!(v.to_string(), "9.0.0");
    }

    #[test]
    fn fetch_latest_fails_on_http_error() {
        let url = serve_once("404 Not Found", r#"{"message":"Not Found"}"#);
        assert!(fetch_latest(&url, Duration::from_secs(5)).is_err());
    }

    #[test]
    fn fetch_latest_times_out_without_hanging() {
        // 接受连接但永远不回应，超时应该很快返回错误。
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let addr = listener.local_addr().unwrap();
        thread::spawn(move || {
            let _held = listener.accept();
            thread::sleep(Duration::from_secs(5));
        });
        let started = Instant::now();
        let result = fetch_latest(&format!("http://{addr}/"), Duration::from_millis(300));
        assert!(result.is_err());
        assert!(started.elapsed() < Duration::from_secs(3), "超时没有生效：{:?}", started.elapsed());
    }
}
