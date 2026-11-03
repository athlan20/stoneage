package webpages

import (
	"context"
	"io"
	"io/fs"
	"net"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/athlan20/stoneage/docs"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

func readDoc(t *testing.T, name string) string {
	t.Helper()
	raw, err := docs.FS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// section 返回 “## n.” 开头的一节（到下一个 “## ” 为止）。
func section(t *testing.T, doc string, n string) string {
	t.Helper()
	start := strings.Index(doc, "\n## "+n+". ")
	if start < 0 {
		t.Fatalf("手册里没有第 %s 节", n)
	}
	rest := doc[start+1:]
	if end := strings.Index(rest[3:], "\n## "); end >= 0 {
		return rest[:end+3]
	}
	return rest
}

// STAGE-118：手册里写的“第 X.Y 节”，docs/protocol.md 里都要有对应标题。
func TestGuideSectionReferences(t *testing.T) {
	guide, proto := readDoc(t, "agent-guide.md"), readDoc(t, "protocol.md")
	refs := regexp.MustCompile(`第 (\d+(?:\.\d+)?) 节`).FindAllStringSubmatch(guide, -1)
	if len(refs) < 10 {
		t.Fatalf("引用太少，正则可能写错了: %d", len(refs))
	}
	for _, m := range refs {
		num := m[1]
		prefix := "\n### " + num + " "
		if !strings.Contains(num, ".") {
			prefix = "\n## " + num + ". "
		}
		if !strings.Contains(proto, prefix) {
			t.Errorf("手册引用了第 %s 节，但 protocol.md 里没有这一节", num)
		}
	}
}

// STAGE-118：服务端所有“操作”类请求，都必须出现在手册第 6 节的三级表里。
func TestGuideClassifiesEveryOperation(t *testing.T) {
	guide := readDoc(t, "agent-guide.md")
	sec := section(t, guide, "6")
	for _, m := range protocol.Requests {
		if m.Category != protocol.CategoryOperation {
			continue
		}
		if !strings.Contains(sec, "`"+m.Type+"`") {
			t.Errorf("操作类请求 %s 没有写进手册第 6 节的先问玩家分级里", m.Type)
		}
	}
}

// 手册第 9 节列出的错误码，都必须在协议里有定义。
func TestGuideErrorCodesExistInProtocol(t *testing.T) {
	guide, proto := readDoc(t, "agent-guide.md"), readDoc(t, "protocol.md")
	sec := section(t, guide, "9")
	codes := regexp.MustCompile("(?m)^\\| `([A-Z_]+)`").FindAllStringSubmatch(sec, -1)
	if len(codes) < 8 {
		t.Fatalf("第 9 节的错误码太少: %d", len(codes))
	}
	for _, m := range codes {
		if !strings.Contains(proto, "`"+m[1]+"`") {
			t.Errorf("手册里的错误码 %s 在 protocol.md 里没有定义", m[1])
		}
	}
}

// STAGE-118：protocol.Requests 登记的每个请求，docs/protocol.md 里都必须有对应条目。
func TestProtocolDocumentsEveryRequest(t *testing.T) {
	proto := readDoc(t, "protocol.md")
	for _, m := range protocol.Requests {
		if !strings.Contains(proto, "`"+m.Type+"`") {
			t.Errorf("请求 %s 登记在 protocol.Requests 里，但 protocol.md 没有写", m.Type)
		}
	}
}

// 参考脚本和手册都要能从服务端的 WebSocket 端口拿到，石器百科也在同一端口。
func TestServedOnWebSocketPort(t *testing.T) {
	srv := gateway.New(gateway.Options{})
	Install(srv, "https://cdn.example.com/stoneage")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.ServeWS(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	base := "http://" + ln.Addr().String()

	cases := []struct {
		path, want string
	}{
		{"/agent.md", "石器时代 agent 使用手册"},
		{"/protocol.md", "客户端 ↔ 服务端协议"},
		{"/agent-connect.py", "石器时代 agent 参考连接脚本"},
		{"/leveling.md", "练级路线（1 → 120，agent 执行手册）"},
		{"/install.md", "发布页"},
		{"/wiki/", "<html"},
		{"/wiki/q?q=加加村&limit=2", "共 "},
	}
	for _, c := range cases {
		resp, err := http.Get(base + c.path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), c.want) {
			t.Errorf("%s: 状态 %d，内容没有 %q", c.path, resp.StatusCode, c.want)
		}
	}
	resp, err := http.Post(base+"/agent.md", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /agent.md 应 405，实际 %d", resp.StatusCode)
	}
}

// STAGE-180：protocol.md 和 agent-guide.md 里用相对路径链接的 .md，服务端都要能按同一路径取到，
// 否则 agent 照着链接读会 404（协议 5.50 等处的字段说明就是这样丢的）。
func TestRelativeDocLinksAreServed(t *testing.T) {
	linkRE := regexp.MustCompile(`\]\(([^)#\s]+\.md)\)`)
	served := map[string]bool{}
	for _, f := range textFiles {
		served[f.file] = true
	}
	for _, doc := range []string{"protocol.md", "agent-guide.md"} {
		body, err := fs.ReadFile(docs.FS, doc)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range linkRE.FindAllStringSubmatch(string(body), -1) {
			link := m[1]
			if strings.Contains(link, "://") || strings.HasPrefix(link, "/") {
				continue
			}
			if !served[link] {
				t.Errorf("%s 链接的 %s 没有发布（webpages.textFiles 里缺少它）", doc, link)
			}
		}
	}
}

// STAGE-188：协议或手册改了，agent-changes.md 必须登记当前的 docs_rev，agent 才知道要重读。
// 不同分支合入的组合各有一个修订号，所以只要求当前的修订号有一条登记，不要求它在最上面。
func TestChangesRegistersCurrentDocsRev(t *testing.T) {
	changes := readDoc(t, "agent-changes.md")
	want := docs.Rev()
	for _, m := range regexp.MustCompile(`(?m)^## .*docs_rev ([0-9a-f]{12})\s*$`).FindAllStringSubmatch(changes, -1) {
		if m[1] == want {
			return
		}
	}
	t.Fatalf("protocol.md 或 agent-guide.md 改过了（当前 docs_rev %s），请在 agent-changes.md 最上面追加变更，标题写 docs_rev %s", want, want)
}
