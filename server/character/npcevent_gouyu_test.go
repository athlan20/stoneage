package character

// STAGE-333「萨姆吉尔的勾玉」（任务 111，事件 185）：长老（640）接任务 → 萨姆吉尔（1311）依次发五块勾玉 →
// 孙女（1241）五块齐时兑换。和红暴（任务 40，事件 63）脱钩：红暴已完成的角色也能接、能拿勾玉；只有四块时孙女不给；
// 兑换后不能重复领。纯逻辑（不连数据库），用真实白名单和真实选块；协议层不在这里测。
// 数据检查（白名单、事件号只属于这一条任务、NPC 类型和地图）放在文件末尾。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	gouyuElder     = 640   // 长老（overlay/gouyu_elder，5500，32,22）：任务 111 接取
	gouyuSam       = 1311  // 萨姆吉尔（poru/samugiru）：五块勾玉发放
	gouyuGirl      = 1241  // 孙女（poru/girl）：五块齐兑换
	gouyuEvent     = 185   // 任务 111 的完成和进行中事件
	gouyuQuestNo   = 111   // 任务编号（任务表 EXTRA_QUESTS）
	gouyuRing      = 18537 // 萨姆吉尔的首饰（兑换给一件）
	gouyuBless     = 18543 // 英雄的祝福（兑换给五件）
	gouyuElderFile = "overlay/gouyu_elder"
)

// gouyuHold 是五块萨姆吉尔的勾玉（18538~18542）。
var gouyuHold = []int{18538, 18539, 18540, 18541, 18542}

// gouyuSamItems 是萨姆吉尔五个发放块要的东西（块 1 的耐久力回复药、块 2 的气力回复药、块 3–5 的烤猪排、肉汤、蘑菇）。
var gouyuSamItems = []int{1501, 1512, 12667, 12507, 12849}

// gouyuSamAll 让五块勾玉依次到手（萨姆吉尔块 1–5，每块都是接受交易）。
func gouyuSamAll(t *testing.T, s *Service, r *record) {
	t.Helper()
	for b := 1; b <= 5; b++ {
		kingRun(t, r, poruPick(t, s, r, gouyuSam, b))
	}
}

// 正向：长老接任务 → 五块勾玉依次拿齐 → 孙女兑换（得首饰、五个英雄的祝福，勾玉没了）→ 任务完成，之后长老只道谢。
func TestGouyuFullChain(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(gouyuSamItems...), nil, nil)

	kingRun(t, r, poruPick(t, s, r, gouyuElder, 0))
	if !r.eventDoing(gouyuEvent) || r.eventDone(gouyuEvent) {
		t.Fatalf("长老接取后事件 185 应进行中，实际 flags=%v", r.EventFlags)
	}

	gouyuSamAll(t, s, r)
	for _, id := range gouyuHold {
		if countItem(r.Items, id) != 1 {
			t.Fatalf("五块勾玉齐后应各有一块，%d 实际 %d（背包 %v）", id, countItem(r.Items, id), r.Items)
		}
	}

	kingRun(t, r, poruPick(t, s, r, gouyuGirl, 0))
	if countItem(r.Items, gouyuRing) != 1 || countItem(r.Items, gouyuBless) != 5 {
		t.Fatalf("兑换应给首饰 1 件、英雄的祝福 5 件，实际 %v", r.Items)
	}
	for _, id := range gouyuHold {
		if countItem(r.Items, id) != 0 {
			t.Fatalf("兑换后勾玉 %d 应被收走，实际还有 %v", id, r.Items)
		}
	}
	if !r.eventDone(gouyuEvent) || r.eventDoing(gouyuEvent) {
		t.Fatalf("兑换后事件 185 应完成、不再进行中，实际 flags=%v", r.EventFlags)
	}

	poruPick(t, s, r, gouyuElder, 2)
}

// 已完成红暴（事件 63）的角色：长老照样能接，萨姆吉尔照样发勾玉，孙女照样兑换。
func TestGouyuOpenAfterRedDone(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(gouyuSamItems...), []int{poruQ40}, nil)

	kingRun(t, r, poruPick(t, s, r, gouyuElder, 0))
	gouyuSamAll(t, s, r)
	kingRun(t, r, poruPick(t, s, r, gouyuGirl, 0))

	if countItem(r.Items, gouyuBless) != 5 || !r.eventDone(gouyuEvent) {
		t.Fatalf("红暴已完成的角色应能完成新任务，实际 items=%v flags=%v", r.Items, r.EventFlags)
	}
	if !r.eventDone(poruQ40) {
		t.Fatalf("新任务不应动红暴的完成位")
	}
}

// 只有四块勾玉时孙女不给：块 0 的五块齐条件不成立，也不会误走别的块，任务还在进行中。
func TestGouyuFourPiecesNotGiven(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(18538, 18539, 18540, 18541), nil, []int{gouyuEvent})

	poruPick(t, s, r, gouyuGirl, -1)
	if countItem(r.Items, gouyuRing) != 0 || countItem(r.Items, gouyuBless) != 0 {
		t.Fatalf("只有四块时孙女不应给东西，实际 %v", r.Items)
	}
	if r.eventDone(gouyuEvent) || !r.eventDoing(gouyuEvent) {
		t.Fatalf("只有四块时任务应仍在进行中，实际 flags=%v", r.EventFlags)
	}
}

// 提示跟着新任务走：185 进行中、手上什么都没有时，萨姆吉尔说第一步的药品提示（块 9）；只有红暴（63）进行中则什么都不说。
func TestGouyuHintFollowsNewQuest(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(), nil, []int{gouyuEvent})
	poruPick(t, s, r, gouyuSam, 9)

	r = kingRecord(kingItems(), nil, []int{63})
	poruPick(t, s, r, gouyuSam, -1)
}

// 兑换后再来不重复领：任务已完成，五块勾玉（若再拿到）也不再换，长老只道谢、不再接。
func TestGouyuNoRepeatAfterExchange(t *testing.T) {
	s := kingService()
	r := kingRecord(kingItems(gouyuHold...), []int{gouyuEvent}, nil) // 已完成的状态，手里又拿到了五块
	poruPick(t, s, r, gouyuElder, 2)
	poruPick(t, s, r, gouyuGirl, 1) // 完成后只剩块 1（原块 6）“你好棒”，不再命中五块齐的兑换块
	if countItem(r.Items, gouyuBless) != 0 || countItem(r.Items, gouyuRing) != 0 {
		t.Fatalf("任务完成后再来的五块勾玉不应再换，实际 %v", r.Items)
	}

	// 同一次进度里：兑换一次后任务完成，孙女再遇到时不再给。
	r = kingRecord(kingItems(gouyuHold...), nil, []int{gouyuEvent})
	kingRun(t, r, poruPick(t, s, r, gouyuGirl, 0))
	poruPick(t, s, r, gouyuGirl, 1)
	if countItem(r.Items, gouyuBless) != 5 || countItem(r.Items, gouyuRing) != 1 {
		t.Fatalf("兑换一次后再来不应重复领，实际 %v", r.Items)
	}
}

// 红暴（任务 40）的接取行为不变：老爷爷接任务只进行中 63，不碰新事件 185。
func TestGouyuRedUnchanged(t *testing.T) {
	s := kingService()
	r := kingRecord(nil, nil, nil)
	kingRun(t, r, poruPick(t, s, r, poruGrandpa, 3))
	if !r.eventDoing(poruQ40) || r.eventDoing(gouyuEvent) || r.eventDone(gouyuEvent) {
		t.Fatalf("接红暴后应只有 63 进行中，实际 flags=%v", r.EventFlags)
	}
}

// 数据检查：白名单放开了改写过的脚本和长老的覆盖脚本；红暴的 boy02、boy04 仍在。
func TestGouyuWhitelist(t *testing.T) {
	for _, f := range []string{"poru/girl", "poru/samugiru", "poru/samugiru04", gouyuElderFile, "poru/boy02", "poru/boy04"} {
		if !gamedata.NpcEventEnabled(f) {
			t.Errorf("%s 应在白名单里", f)
		}
	}
}

// 数据检查：事件 185 只出现在这四个脚本里（脚本层的事件号唯一），任务表里也只属于任务 111。
func TestGouyuEventOnlyOnItsScripts(t *testing.T) {
	want := map[string]bool{"poru/girl": true, "poru/samugiru": true, "poru/samugiru04": true, gouyuElderFile: true}
	got := map[string]bool{}
	for _, sc := range gamedata.NpcEventScripts() {
		for _, b := range sc.Blocks {
			if b.Event == gouyuEvent {
				got[sc.File] = true
			}
			for _, g := range b.Cond {
				for _, term := range g {
					if term.N == gouyuEvent && (term.Op == "NOWEV" || term.Op == "ENDEV") {
						got[sc.File] = true
					}
				}
			}
		}
	}
	for f := range want {
		if !got[f] {
			t.Errorf("%s 应用到事件 185", f)
		}
	}
	for f := range got {
		if !want[f] {
			t.Errorf("%s 不应用到事件 185", f)
		}
	}
	for _, q := range gamedata.Quests() {
		if q.No != gouyuQuestNo && (q.EndEvent == gouyuEvent || q.NowEvent == gouyuEvent) {
			t.Errorf("任务 %d 和萨姆吉尔的勾玉共用事件 185", q.No)
		}
	}
	if q, ok := gamedata.QuestByNo(gouyuQuestNo); !ok || q.Name != "萨姆吉尔的勾玉" || q.EndEvent != gouyuEvent || q.NowEvent != gouyuEvent {
		t.Errorf("任务 111 应是萨姆吉尔的勾玉（事件 185），实际 %+v %v", q, ok)
	}
}

// 数据检查：长老是任务类型、在正式地图 5500 的 32,22，脚本是覆盖层的，编号不变（仍是 640）。
func TestGouyuElderNPC(t *testing.T) {
	npc, ok := gamedata.NPCByID(gouyuElder)
	if !ok || npc.Kind != "changeevent" || npc.Type != "任务" || npc.Floor != 5500 || npc.X != 32 || npc.Y != 22 || npc.Name != "长老" {
		t.Fatalf("长老 NPC 640 应是 changeevent 任务、5500 地图 32,22，实际 %+v ok=%v", npc, ok)
	}
	script, ok := gamedata.NpcEventOf(gouyuElder)
	if !ok || script.File != gouyuElderFile {
		t.Fatalf("长老的脚本应是 %s，实际 %+v ok=%v", gouyuElderFile, script, ok)
	}
}
