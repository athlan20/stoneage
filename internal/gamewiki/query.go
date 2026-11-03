package gamewiki

// /wiki/q：给 agent 查百科用的纯文本接口（STAGE-118）。
//   GET /wiki/q?q=关键词&kind=分类&limit=20  → 匹配词条的紧凑列表（每行一条，制表符分隔）
//   GET /wiki/q?key=map:123                  → 这个词条的文本详情
// 索引在创建 Handler 时读入嵌入快照里当前版本的 index.json.gz（只有检索用的文本，约 2 万行）；
// 详情按需读对应的分片，不常驻内存。

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	queryDefaultLimit = 20
	queryMaxLimit     = 50
	querySnippetRunes = 80
)

// queryIndex 是启动时读入的检索索引。rows 的列顺序和 StaticIndex.Rows 一致。
type queryIndex struct {
	revision string
	rows     [][]string
}

// 检索行的列号（见 BuildSnapshotWithMedia）。
const (
	colKey, colKind, colName, colDesc, colLocation, colSearch, colShard = 0, 1, 2, 3, 4, 9, 10
)

func loadQueryIndex() *queryIndex {
	catalog, err := readGzipJSON[StaticIndex]("snapshot/catalog.json.gz")
	if err != nil {
		return &queryIndex{}
	}
	index, err := readGzipJSON[StaticIndex]("snapshot/" + catalog.Revision + "/index.json.gz")
	if err != nil {
		return &queryIndex{revision: catalog.Revision}
	}
	return &queryIndex{revision: catalog.Revision, rows: index.Rows}
}

func readGzipJSON[T any](name string) (T, error) {
	var v T
	raw, err := content.ReadFile(name)
	if err != nil {
		return v, err
	}
	z, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return v, err
	}
	defer z.Close()
	data, err := io.ReadAll(z)
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(data, &v)
	return v, err
}

// categoryName 返回分类的中文名；未知分类返回空字符串。
func categoryName(id string) string {
	for _, c := range categories {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}

// search 按关键词（不区分大小写，匹配名称和检索文本）和分类筛选，返回前 limit 条和总匹配数。
func (x *queryIndex) search(q, kind string, limit int) ([][]string, int) {
	q = strings.ToLower(strings.TrimSpace(q))
	var hits [][]string
	total := 0
	for _, row := range x.rows {
		if kind != "" && row[colKind] != kind {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(row[colName]), q) && !strings.Contains(row[colSearch], q) {
			continue
		}
		total++
		if len(hits) < limit {
			hits = append(hits, row)
		}
	}
	return hits, total
}

// serveQuery 处理 /wiki/q。key 优先于 q。
func (h *Handler) serveQuery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	query := r.URL.Query()
	if key := strings.TrimSpace(query.Get("key")); key != "" {
		text, ok := h.detail(key)
		if !ok {
			http.Error(w, "没有这个词条: "+key+"（先用 ?q= 搜索，编号形如 map:123）", http.StatusNotFound)
			return
		}
		fmt.Fprint(w, text)
		return
	}
	q, kind := strings.TrimSpace(query.Get("q")), strings.TrimSpace(query.Get("kind"))
	if q == "" && kind == "" {
		http.Error(w, "用法: /wiki/q?q=关键词&kind=分类&limit=20 或 /wiki/q?key=编号。分类: "+categoryIDs(), http.StatusBadRequest)
		return
	}
	if kind != "" && categoryName(kind) == "" {
		http.Error(w, "未知分类 "+kind+"，可选: "+categoryIDs(), http.StatusBadRequest)
		return
	}
	limit := queryDefaultLimit
	if s := query.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			http.Error(w, "limit 必须是正整数", http.StatusBadRequest)
			return
		}
		limit = min(n, queryMaxLimit)
	}
	hits, total := h.query.search(q, kind, limit)
	var b strings.Builder
	fmt.Fprintf(&b, "共 %d 条匹配，显示 %d 条。详情: /wiki/q?key=编号\n", total, len(hits))
	b.WriteString("编号\t分类\t名称\t位置\t简介\n")
	for _, row := range hits {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\n", row[colKey], categoryName(row[colKind]), row[colName], oneLine(row[colLocation]), truncRunes(oneLine(row[colDesc]), querySnippetRunes))
	}
	fmt.Fprint(w, b.String())
}

// detail 读取词条的完整内容并渲染成文本。
func (h *Handler) detail(key string) (string, bool) {
	var shard string
	for _, row := range h.query.rows {
		if row[colKey] == key {
			shard = row[colShard]
			break
		}
	}
	if shard == "" {
		return "", false
	}
	entries, err := readGzipJSON[map[string]*Entry]("snapshot/" + h.query.revision + "/" + shard + ".json.gz")
	if err != nil || entries[key] == nil {
		return "", false
	}
	return renderEntry(entries[key]), true
}

func categoryIDs() string {
	ids := make([]string, 0, len(categories))
	for _, c := range categories {
		ids = append(ids, c.ID)
	}
	return strings.Join(ids, ", ")
}
