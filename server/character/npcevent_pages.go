package character

// 任务 NPC 的分页台词（STAGE-97，docs/quest.md「分页台词」）：原版 AcceptMsg1..N、RequestMsg1..N、ThanksMsg1..N、
// NomalWindowMsg1..N，窗口按页翻（对照 npc_exchangeman.c 的 WINDOW_BUTTONTYPE_NEXT / PREV）。翻页只换显示的页，
// 不改任何数据：问答的“是/否”只在最后一页出现，交易在选“是”之后才做。窗口的选项见 docs/protocol.md 5.35。

import (
	"github.com/athlan20/stoneage/server/gamedata"
)

// pageAct 是窗口上一个选项的作用。
type pageAct int

const (
	actYes     pageAct = iota + 1 // 是（问答的最后一页）
	actNo                         // 否（问答的最后一页）
	actNext                       // 下一页
	actPrev                       // 上一页（第 2 页起）
	actKeyword                    // 说暗号（读完的最后一页上，脚本还有没完成的暗号块时）
)

// pageLabels 是选项的文字（编号按出现顺序从 1 排）。
var pageLabels = map[pageAct]string{
	actYes: "是", actNo: "否", actNext: "下一页", actPrev: "上一页", actKeyword: "说暗号",
}

// eventOpt 是窗口上的一个选项和它的作用；No 从 1 开始，和窗口的选项编号一致。
type eventOpt struct {
	npcOption
	act pageAct
}

// eventPageOpts 算出当前页的选项：不是最后一页时是“下一页”；最后一页上问答是“是、否”，读完的台词上是可能的
// “说暗号”；第 2 页起多一个“上一页”，放在最后。
func eventPageOpts(ev *eventSession) []eventOpt {
	var acts []pageAct
	last := ev.page == len(ev.pages)-1
	switch {
	case !last:
		acts = append(acts, actNext)
	case ev.ask == askRead:
		if ev.keyword {
			acts = append(acts, actKeyword)
		}
	default:
		acts = append(acts, actYes, actNo)
	}
	if ev.page > 0 {
		acts = append(acts, actPrev)
	}
	opts := make([]eventOpt, len(acts))
	for i, act := range acts {
		opts[i] = eventOpt{npcOption: npcOption{No: i + 1, Label: pageLabels[act]}, act: act}
	}
	return opts
}

// turn 返回翻到下一页或上一页之后的窗口（其他字段不变，选项在 openEventPage 里重算）。
func (ev *eventSession) turn(act pageAct) *eventSession {
	next := *ev
	if act == actNext {
		next.page++
	} else {
		next.page--
	}
	return &next
}

// openEventPage 开一个分页窗口（只有一页时就是普通的问答或读完的台词）：正文是当前页，选项按当前页算出来。
func openEventPage(p *Presence, npc gamedata.NPC, ev *eventSession) {
	ev.opts = eventPageOpts(ev)
	options := make([]npcOption, len(ev.opts))
	for i := range ev.opts {
		options[i] = ev.opts[i].npcOption
	}
	p.npcWindow = &npcWindow{npc: npc.ID, options: options, event: ev}
	p.pushAll("npc_window", npcWindowView(npc, ev.pages[ev.page], options...))
}

// openEventAsk 开一个问答窗口（ACCEPT、REQUEST、CLEAN、StopMsg），pages 是问话的各页（最后一页才问“是/否”），
// 记下等的是哪一问、命中的块和说的暗号。
func openEventAsk(p *Presence, npc gamedata.NPC, ask eventAsk, block int, said string, pages []string) {
	openEventPage(p, npc, &eventSession{ask: ask, block: block, said: said, pages: pagesOrBlank(pages)})
}

// openEventRead 开一个读完的台词窗口（感谢话、MESSAGE 台词）：只翻页，没有问答。只有一页且不带“说暗号”时和
// 普通台词一样（openEventText）；台词为空时关掉原来的窗口。keyword 为真时最后一页上有“说暗号”（STAGE-92）。
func openEventRead(p *Presence, npc gamedata.NPC, pages []string, keyword bool) {
	switch {
	case len(pages) == 0 || len(pages) == 1 && pages[0] == "":
		openEventText(p, npc, "")
	case len(pages) == 1 && !keyword:
		openEventText(p, npc, pages[0])
	default:
		openEventPage(p, npc, &eventSession{ask: askRead, pages: pages, keyword: keyword})
	}
}

// openEventTalk 开闲聊句或 MESSAGE 块的台词（都是读完的台词）：脚本还有没完成的暗号块时，最后一页上带“说暗号”。
func openEventTalk(p *Presence, r *record, npc gamedata.NPC, ev *gamedata.EventScript, pages []string) {
	openEventRead(p, npc, pages, hasPendingKeyword(r, ev))
}

// pagesOrBlank 保证至少有一页（没有台词时窗口是空的，原版也是这样）。
func pagesOrBlank(pages []string) []string {
	if len(pages) == 0 {
		return []string{""}
	}
	return pages
}

// eventTexts 取台词 kind 的各页并替换金币占位（%8d）；没有台词返回 nil。
func eventTexts(m gamedata.EventMsg, kind string, cost int) []string {
	pages := m.Pages(kind)
	out := make([]string, len(pages))
	for i, text := range pages {
		out[i] = eventText(text, cost)
	}
	return out
}
