package gamewiki

import (
	"fmt"
	"strings"
)

// renderEntry 把词条渲染成给 agent 读的纯文本：先是摘要，再是字段、表格、备注和链接。
func renderEntry(e *Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s（%s）\n", e.Name, e.Key)
	fmt.Fprintf(&b, "分类：%s\n", categoryName(e.Kind))
	writeLine(&b, "简介", e.Description)
	writeLine(&b, "位置", e.Location)
	writeLine(&b, "等级", e.Level)
	writeLine(&b, "血量", e.HP)
	writeLine(&b, "技能", e.Skills)
	writeLine(&b, "分组", e.Group)
	if len(e.Fields) > 0 {
		b.WriteString("\n字段:\n")
		for _, f := range e.Fields {
			fmt.Fprintf(&b, "  %s：%s\n", f.Label, oneLine(f.Value))
		}
	}
	for _, t := range e.Tables {
		fmt.Fprintf(&b, "\n表格「%s」:\n", t.Title)
		if len(t.Columns) > 0 {
			fmt.Fprintf(&b, "  %s\n", strings.Join(t.Columns, " | "))
		}
		for _, row := range t.Rows {
			fmt.Fprintf(&b, "  %s\n", strings.Join(row, " | "))
		}
	}
	if len(e.Notes) > 0 {
		b.WriteString("\n备注:\n")
		for _, n := range e.Notes {
			fmt.Fprintf(&b, "  - %s\n", oneLine(n))
		}
	}
	if len(e.Links) > 0 {
		b.WriteString("\n相关:\n")
		for _, l := range e.Links {
			if l.Key != "" {
				fmt.Fprintf(&b, "  - %s（%s）\n", l.Name, l.Key)
			} else {
				fmt.Fprintf(&b, "  - %s\n", l.Name)
			}
		}
	}
	if len(e.Sources) > 0 {
		b.WriteString("\n来源: " + strings.Join(e.Sources, "；") + "\n")
	}
	return b.String()
}

func writeLine(b *strings.Builder, label, value string) {
	if value != "" {
		fmt.Fprintf(b, "%s：%s\n", label, oneLine(value))
	}
}

// oneLine 把换行压成空格，保证列表和字段每项占一行。
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// truncRunes 按字符数截断，超出时补省略号。
func truncRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
