package protocol

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// STAGE-246：压缩编解码的单元测试。

func TestEncodeGzipRoundTrip(t *testing.T) {
	plain := []byte(`{"id":1,"type":"status","ok":true,"data":{"text":"` + strings.Repeat("石器", 3000) + `"}}`)
	line, err := EncodeGzip(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(line, []byte("\n")) || bytes.Count(line, []byte("\n")) != 1 {
		t.Fatalf("压缩行应只有结尾一个换行: %q", line[:20])
	}
	if !bytes.HasPrefix(line, []byte(`{"z":"gzip","d":"`)) {
		t.Fatalf("外壳格式不对: %q", line[:30])
	}
	if len(line) >= len(plain) {
		t.Fatalf("重复内容应当变小：压缩后 %d，原文 %d", len(line), len(plain))
	}
	got, perr := DecodeLine(bytes.TrimSuffix(line, []byte("\n")))
	if perr != nil {
		t.Fatalf("解压失败: %v", perr)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("解压结果和原文不一致")
	}
}

func TestDecodePlainLinePassesThrough(t *testing.T) {
	for _, line := range []string{
		`{"id":1,"type":"ping","data":{}}`,
		// 带 type 的行即使有 z 字段，也是普通消息，原样返回。
		`{"id":2,"type":"x","z":"gzip","d":"!!"}`,
		// z 不是字符串：不当作压缩行。
		`{"z":1,"id":3,"type":"ping"}`,
		`不是 JSON "z"`,
	} {
		got, perr := DecodeLine([]byte(line))
		if perr != nil || string(got) != line {
			t.Errorf("普通行应原样返回 %q，得到 %q, %v", line, got, perr)
		}
	}
}

func TestDecodeBadCompressedLines(t *testing.T) {
	gz := func(s string) string {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write([]byte(s))
		_ = zw.Close()
		return base64.StdEncoding.EncodeToString(buf.Bytes())
	}
	big := strings.Repeat("a", MaxInflatedBytes+1)
	truncated := gz(strings.Repeat("b", 4096))
	truncated = truncated[:len(truncated)/2]
	cases := map[string]string{
		"不支持的压缩方式":    `{"z":"zstd","d":"AAAA"}`,
		"缺少 d":        `{"z":"gzip"}`,
		"d 不是 base64": `{"z":"gzip","d":"!!!"}`,
		"不是 gzip":     `{"z":"gzip","d":"` + base64.StdEncoding.EncodeToString([]byte("hello")) + `"}`,
		"gzip 被截断":    `{"z":"gzip","d":"` + truncated + `"}`,
		"解压后超限":       `{"z":"gzip","d":"` + gz(big) + `"}`,
	}
	for name, line := range cases {
		got, perr := DecodeLine([]byte(line))
		if perr == nil {
			t.Errorf("%s：应当返回错误，得到 %d 字节", name, len(got))
			continue
		}
		if perr.Code != CodeBadRequest {
			t.Errorf("%s：错误码应为 BAD_REQUEST，实际 %s", name, perr.Code)
		}
	}
}

func TestDecodeAcceptsLimitSized(t *testing.T) {
	// 恰好 1 MiB 的原文可以解压，超过一个字节才算超限。
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write(bytes.Repeat([]byte("a"), MaxInflatedBytes))
	_ = zw.Close()
	line, _ := json.Marshal(zOut{Z: CompressGzip, D: base64.StdEncoding.EncodeToString(buf.Bytes())})
	got, perr := DecodeLine(line)
	if perr != nil || len(got) != MaxInflatedBytes {
		t.Fatalf("恰好 1 MiB 应当能解压，得到 %d 字节, %v", len(got), perr)
	}
}
