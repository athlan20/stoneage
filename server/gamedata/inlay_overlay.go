package gamedata

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

// STAGE-297：精工覆盖表（docs/inlay.md 2.4）。原版数据里精工类别码和初始镶嵌码全是空的（精工没启用），
// 覆盖表可以在不改 itemset.txt 的前提下给指定物品加上这两项。随程序发布的默认覆盖表只有表头，不改任何物品；
// 服务端启动时可以用 -inlay-overlay 指定另一份（见 cmd/stoneage-server）。

//go:embed inlay_overlay.tsv
var inlayOverlayDefault string

// inlayOverlayRow 是覆盖表的一行：物品编号、精工类别码、初始镶嵌码（都是 UTF-8 文字）。
type inlayOverlayRow struct {
	ID         int
	Code, Init string
	Line       int // 行号（从 1 开始），报错用
}

// DefaultInlayOverlay 返回随程序发布的覆盖表（默认只有表头）。
func DefaultInlayOverlay() string { return inlayOverlayDefault }

// LoadInlayOverlay 把覆盖表套到物品模板上，只在服务端启动时调用一次（之后模板只读）。
// 覆盖表里写了物品表里不存在的编号就整体报错，不改任何模板，免得悄悄漏掉。
func LoadInlayOverlay(tsv string) error {
	itemOnce.Do(loadItems)
	rows, err := parseInlayOverlay(tsv)
	if err != nil {
		return err
	}
	for _, r := range rows { // 记下改动前的模板，UnloadInlayOverlay 还原（只给测试用，正常启动不卸载）
		if _, done := inlayOverlayOrig[r.ID]; !done {
			if t, ok := itemTable[r.ID]; ok {
				inlayOverlayOrig[r.ID] = t
			}
		}
	}
	return applyInlayOverlay(itemTable, tsv)
}

// inlayOverlayOrig 是被覆盖表改过的物品改动前的模板。
var inlayOverlayOrig = map[int]ItemTemplate{}

// UnloadInlayOverlay 把 LoadInlayOverlay 改过的物品还原成原来的模板。只给测试用：同一个测试进程里
// 加载了覆盖表，之后读背包输出的测试（如 STAGE-297 的回归）要先还原，否则结果被污染。
func UnloadInlayOverlay() {
	itemOnce.Do(loadItems)
	for id, t := range inlayOverlayOrig {
		itemTable[id] = t
	}
	inlayOverlayOrig = map[int]ItemTemplate{}
}

// applyInlayOverlay 先检查整张覆盖表，全部通过再写到 table 上。
func applyInlayOverlay(table map[int]ItemTemplate, tsv string) error {
	rows, err := parseInlayOverlay(tsv)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if _, ok := table[r.ID]; !ok {
			return fmt.Errorf("精工覆盖表第 %d 行：物品编号 %d 在物品表里不存在", r.Line, r.ID)
		}
	}
	for _, r := range rows {
		t := table[r.ID]
		t.InlayCode, t.InlayInit = r.Code, r.Init
		table[r.ID] = t
	}
	return nil
}

// parseInlayOverlay 解析覆盖表：每行 物品编号\t类别码\t初始镶嵌码。空行、# 注释行和表头跳过；
// 列数不对、编号不是整数、编号重复都报错。
func parseInlayOverlay(tsv string) ([]inlayOverlayRow, error) {
	var rows []inlayOverlayRow
	seen := make(map[int]int)
	for i, line := range strings.Split(tsv, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "物品编号") {
			continue
		}
		lineNo := i + 1
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			return nil, fmt.Errorf("精工覆盖表第 %d 行：应有 3 列（物品编号、类别码、初始镶嵌码），现在 %d 列", lineNo, len(f))
		}
		id, err := strconv.Atoi(strings.TrimSpace(f[0]))
		if err != nil {
			return nil, fmt.Errorf("精工覆盖表第 %d 行：物品编号 %q 不是整数", lineNo, f[0])
		}
		if prev, dup := seen[id]; dup {
			return nil, fmt.Errorf("精工覆盖表第 %d 行：物品编号 %d 和第 %d 行重复", lineNo, id, prev)
		}
		seen[id] = lineNo
		rows = append(rows, inlayOverlayRow{ID: id, Code: strings.TrimSpace(f[1]), Init: strings.TrimSpace(f[2]), Line: lineNo})
	}
	return rows, nil
}
