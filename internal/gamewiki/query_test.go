package gamewiki

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func queryGet(t *testing.T, h http.Handler, target string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec.Code, rec.Body.String()
}

// STAGE-118：/wiki/q 的搜索、详情和参数错误。
func TestWikiQuery(t *testing.T) {
	h := NewHandler()
	if len(h.query.rows) < 1000 {
		t.Fatalf("检索索引太少: %d 行", len(h.query.rows))
	}

	code, body := queryGet(t, h, "/wiki/q?q=加加村&kind=map&limit=3")
	if code != http.StatusOK || !strings.Contains(body, "加加村") || !strings.Contains(body, "共 ") {
		t.Fatalf("搜索地图失败: %d\n%s", code, body)
	}
	// 列表第二行起是“编号 分类 名称 位置 简介”，取第一条的编号去查详情。
	lines := strings.Split(body, "\n")
	if len(lines) < 3 {
		t.Fatalf("搜索结果太少:\n%s", body)
	}
	key := strings.Split(lines[2], "\t")[0]
	code, body = queryGet(t, h, "/wiki/q?key="+key)
	if code != http.StatusOK || !strings.Contains(body, "（"+key+"）") || !strings.Contains(body, "分类：地图") {
		t.Fatalf("详情 %s 不对: %d\n%s", key, code, body)
	}

	if code, _ = queryGet(t, h, "/wiki/q?key=map:999999999"); code != http.StatusNotFound {
		t.Errorf("不存在的词条应 404，实际 %d", code)
	}
	if code, body = queryGet(t, h, "/wiki/q"); code != http.StatusBadRequest || !strings.Contains(body, "用法") {
		t.Errorf("空查询应给用法，实际 %d %q", code, body)
	}
	if code, _ = queryGet(t, h, "/wiki/q?q=a&kind=nope"); code != http.StatusBadRequest {
		t.Errorf("未知分类应 400，实际 %d", code)
	}
	if code, _ = queryGet(t, h, "/wiki/q?q=a&limit=0"); code != http.StatusBadRequest {
		t.Errorf("limit=0 应 400，实际 %d", code)
	}
	if _, body = queryGet(t, h, "/wiki/q?q=加加村&limit=999"); strings.Count(body, "\n") > queryMaxLimit+2 {
		t.Errorf("limit 应封顶 %d", queryMaxLimit)
	}
}

// STAGE-118：百科的「Agent」分类指向 /agent.md，搜得到，详情里也有这句话。
func TestWikiAgentCategoryPointsToManual(t *testing.T) {
	h := NewHandler()
	code, body := queryGet(t, h, "/wiki/q?kind=agent")
	if code != http.StatusOK || !strings.Contains(body, "共 1 条匹配") || !strings.Contains(body, "agent:manual") {
		t.Fatalf("Agent 分类应有一条 agent:manual: %d\n%s", code, body)
	}
	code, body = queryGet(t, h, "/wiki/q?q=agent.md")
	if code != http.StatusOK || !strings.Contains(body, "agent:manual") {
		t.Errorf("搜 agent.md 应命中 agent:manual: %d\n%s", code, body)
	}
	code, body = queryGet(t, h, "/wiki/q?key=agent:manual")
	if code != http.StatusOK || !strings.Contains(body, "/agent.md") || !strings.Contains(body, "分类：Agent") {
		t.Errorf("详情应指向 /agent.md: %d\n%s", code, body)
	}
}
