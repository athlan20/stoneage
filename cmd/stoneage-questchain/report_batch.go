package main

// 报告的批次和方法（STAGE-213）：推荐放开批次按累积的闭包算每批新走通的任务；方法与局限写明模型的近似。

import (
	"fmt"
	"strings"
)

// writeBatches 写推荐放开批次。第 n 批的结果是白名单加前 n 批脚本的闭包；每批列出新走通的任务、
// 它的推导里用到本批的脚本和依赖前批的脚本，以及本批里没有单独走通任务的脚本（要和后批一起放开）。
func (a *analysis) writeBatches(b *strings.Builder) {
	batches := a.rules.PlanBatches(a.m.Files)
	b.WriteString("## 推荐放开批次\n\n")
	b.WriteString("按整条任务链能走完分批：`king` 目录（精灵王的传说，STAGE-198 精灵少女转生的前置）已在 STAGE-220 放开；下面是其余目录的批次，第一批是 STAGE-197 新补齐的六个，之后按 `batchDirs` 的顺序。")
	b.WriteString("每批的结果是白名单加前面所有批次的脚本，所以后批的任务依赖前批的脚本时会写明。整本支持的脚本才进批次；带不支持块的脚本见“只有 C 能走通的任务”。\n\n")
	prev := a.m.CloseCumulative(a.rules, batches, 0)
	for i, batch := range batches {
		cur := a.m.CloseCumulative(a.rules, batches, i+1)
		fmt.Fprintf(b, "### 第 %d 批：%s（%d 个脚本）\n\n", i+1, batch.Name, len(batch.Files))
		fmt.Fprintf(b, "本批放开：%s\n\n", codeList(batch.Files))
		inBatch := map[string]bool{}
		for _, f := range batch.Files {
			inBatch[f] = true
		}
		used := map[string]bool{}
		var lines []string
		for _, row := range a.rows {
			end := Fact{Kind: KindEnd, N: row.q.EndEvent}
			if !cur.Has[end] || prev.Has[end] {
				continue
			}
			der := cur.Derive(end)
			var own, before []string
			for _, f := range unitFiles(der.Units) {
				switch {
				case inBatch[f]:
					own = append(own, f)
					used[f] = true
				case !a.rules.Enabled(f):
					before = append(before, f)
				}
			}
			line := fmt.Sprintf("- 任务 %d %s（事件 %d）：本批用到 %s", row.q.No, row.q.Name, row.q.EndEvent, codeListOrNone(own))
			if len(before) > 0 {
				line += "；依赖前批 " + codeList(before)
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			b.WriteString("本批没有新走通的任务。\n\n")
		} else {
			fmt.Fprintf(b, "本批新走通 %d 个任务：\n", len(lines))
			for _, l := range lines {
				b.WriteString(l + "\n")
			}
			b.WriteString("\n")
		}
		var idle, needed []string
		for _, f := range batch.Files {
			if used[f] {
				needed = append(needed, f)
			} else {
				idle = append(idle, f)
			}
		}
		if len(needed) > 0 {
			fmt.Fprintf(b, "其中新走通的任务用到的（最少要放的）：%s\n\n", codeList(needed))
		}
		if len(idle) > 0 {
			fmt.Fprintf(b, "其余脚本（新走通的任务用不到，可以先不放）：%s\n\n", codeList(idle))
		}
		prev = cur
	}
	quests, _ := a.count(scenB) // 全部批次的闭包和场景 B 相同（realdata_test.go 检查）
	fmt.Fprintf(b, "全部批次之后能走完 %d 个任务，与场景 B 一致。\n\n", quests)
	a.writeBlockedByWholeFile(b, batches)
}

// writeBlockedByWholeFile 写逐块都支持、但没有任何批次能覆盖的任务：卡在整本放不开的脚本上（同“整本放不开的任务”）。
// 这些任务要等脚本里的不支持块修好，才能进某一批。
func (a *analysis) writeBlockedByWholeFile(b *strings.Builder, batches []Batch) {
	final := a.m.CloseCumulative(a.rules, batches, len(batches))
	var names []string
	for _, row := range a.rows {
		end := Fact{Kind: KindEnd, N: row.q.EndEvent}
		if a.closD.Has[end] && !final.Has[end] {
			names = append(names, fmt.Sprintf("%d %s", row.q.No, row.q.Name))
		}
	}
	if len(names) == 0 {
		return
	}
	fmt.Fprintf(b, "不在任何批次里的任务（逐块都支持，但卡在整本放不开的脚本上，修好它们的不支持块之后才能进批次）：%s。\n\n", strings.Join(names, "；"))
}

// codeListOrNone 和 codeList 一样，空的时候写“（无）”。
func codeListOrNone(files []string) string {
	if len(files) == 0 {
		return "（无）"
	}
	return codeList(files)
}

// writeMethod 写方法与局限：模型怎么近似，哪些地方会误判。
func writeMethod(b *strings.Builder) {
	b.WriteString(methodText)
}

const methodText = `## 方法与局限

**模型**（对照 ` + "`server/character/npcevent*.go`" + `）：

- 块能执行，当且仅当它没有 ` + "`Unsupported`" + `（场景 C 忽略这一条），并且条件的某个候选组（逗号分的，外层）里每一项都能满足（& 分的，内层）。
- 状态是四个只增的集合：完成的事件号（ends）、进行中的事件号（nows）、拿到的道具（items）、拿到的宠物模板（pets）。块执行后：
  EndSetFlg 的事件号进 ends；MESSAGE 和 REQUEST 的 event（不是 -1）进 nows；ACCEPT 的 event 不在它的 EndSetFlg 里时也进 nows（STAGE-341 的 keepDoing）；GetItem、GetRandItem 的道具进 items；
  GetPet 的怪物换成宠物模板进 pets。CLEAN 只清进度，不产生效果。
- 块只有在运行时也会产生效果时才算：MESSAGE 没有台词、ACCEPT 和 REQUEST 没有感谢话，运行时什么都不做，这里也不产生效果。
- 空条件的块：支持的块没有条件时运行时永远不命中（与运行时一致，不执行）；不支持且条件没解析的块（数据里的例子见“只有 C 能走通的任务”）在场景 C 里视为满足。
- 条件：` + "`ITEM=n`" + ` 要 n 在 items 里（或外部可得）；` + "`ENDEV=n`" + ` 要 n 在 ends 里；` + "`NOWEV=n`" + ` 要 n 在 nows 或 ends 里；
  ` + "`PET`" + ` 要宠物模板在 pets 里（或可捕捉）；` + "`ITEM!=`、`ENDEV!=`、`TIME!=`" + ` 视为满足（单调近似）。
- 外部可得：没有任何脚本会给的道具（商店、战斗掉落）；宠物表里 ` + "`PetFlag > 0`" + ` 的可捕捉宠物；条件里引用了、但任务脚本一个都不设置的事件（` + "`ENDEV=`" + ` 或 ` + "`NOWEV=`" + `）。
  这些都不需要脚本，但会列在“清单”里，要人工确认。事件的来源是在原版服务端 C 源码里查的（` + "`sources.go`" + `），查不到的写“来源未确认”，不要当成已验证。
  如果把它们当成不可得，会出现“脚本没写，但别的系统会设”的误判（比如 71 由宠物转生设进行中，见“清单”）。
- DelPet 直接写的宠物是交易的前提（缺了整笔不做，运行时也是这样），所以模型里要求宠物模板可得。DelItem 不是前提（运行时不够也照删，不做检查）。

**等级需求**：每个事实的等级是所有推导里“链路上 LV 条件要求的最大值”的最小值。LV>n 要 n+1 级，LV=n 要 n 级，LV<n 从 1 级起就满足。
任务的最低等级取它所在的最窄场景里的值。

**推导和“脚本数”**：闭包分三趟跑（先白名单，再整本支持，最后全部），每个事实记下第一次成立时的块，推导沿它展开。
这不是用最少脚本的推导，但能看出哪些脚本是白名单里的、哪些是新放开的；“用到的脚本”和“需要新放开的脚本”都按这条推导数。

**忽略的东西**（会让模型比运行时宽松，所以 ✗ 仍然可靠，✓ 只是“可能”）：

- 道具数量（` + "`ITEM=n*k`" + ` 只看有没有）、宠物等级和数量（` + "`PET<30-n`" + ` 只看模板）、金币（DelStone 视为可得）、背包空位。
- 块的选择顺序：运行时第一个命中的块挡住后面的块，这里所有命中的块都能执行。运行时遇到不支持的块会拒绝整段对话（选块在条件判断之前就返回），
  模型里它只是不能执行，后面的块仍然可执行。
- 已完成的块会被跳过、CleanFlg 会重置事件号、放弃（StopMsg）、魅力、NpcWarp（NPC 移位）、位置、时段（TIME）。
- NOWEV 的进行中近似为“进行中或已完成”，因为进行中的块要在接任务之后才会出现。

**结论的用法**：标 ✗ 的任务确定走不通（前提是数据和规则对）；标 ✓ 的还要端到端测试验证（第二阶段）。
本报告是 STAGE-213 的结论，取代 ` + "`docs/quest.md`" + ` 里 STAGE-94 的“本批任务能否走完”那一节的旧结论。
`
