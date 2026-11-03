package character

// STAGE-345：采伐许可证 2478 只有一张，涛线（poru/team02 ACCEPT 56）和岚线的赛尚（jaruga/event/20501ev5 块 1，
// 14 个良木 2468 换杉的手环 2481）原来都收走它。覆盖层让手环块不再收 2478（见 tools/npc_event_overlay.py 的 LICENSE_PATCHES）。
// 这里用真实白名单、真实选块和真实交易（纯逻辑，不连数据库）：整链先手环后涛之圣石；反向是不足 14 个良木的块、
// 没有许可证、已有手环、发过手环后再来。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	s345Sainasu  = 1274 // 赛尚（jaruga/event/20501ev5）：块 1 是 14 个良木换手环
	s345Wood     = 2468 // 良木
	s345Bracelet = 2481 // 杉的手环
	s345LicEv    = 187  // 手环只给一次的事件号（覆盖层的 LICENSE_EVENT）
)

// s345Items 造背包：许可证（cert 为真时）、wood 个良木、extra 其他道具，每件占一格。
func s345Items(cert bool, wood int, extra ...int) []itemRec {
	ids := []int{}
	if cert {
		ids = append(ids, s337Cert)
	}
	for i := 0; i < wood; i++ {
		ids = append(ids, s345Wood)
	}
	return kingItems(append(ids, extra...)...)
}

// s345Pick 取 NPC 真实命中的块序号，没有块时为 -1。
func s345Pick(t *testing.T, s *Service, r *record, npc int) int {
	t.Helper()
	script, ok := gamedata.NpcEventOf(npc)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", npc)
	}
	idx, refuse := s.pickEventBlock(r, script, "")
	if refuse {
		t.Fatalf("NPC %d 应该能对话，实际拒绝", npc)
	}
	return idx
}

// 整链：同一角色有 2478 和 14 个良木 → 赛尚给手环，2478 仍在包里，一次性事件设上 → 再把 2478 交给米兰达队长
// （事件 56 进行中，30 级迪欧），换涛之圣石。两步都成功。
func TestStage345ChainBraceletThenPoru(t *testing.T) {
	s := kingService()
	r := kingRecord(s345Items(true, 14), []int{s337Ev43, s337Ev46}, []int{s337Ev41, s337Ev45})

	kingRun(t, r, kingPick(t, s, r, s345Sainasu, 1))
	if countItem(r.Items, s345Bracelet) != 1 || countItem(r.Items, s337Cert) != 1 || countItem(r.Items, s345Wood) != 0 {
		t.Fatalf("赛尚给手环后应有手环和许可证、良木清空，实际 items=%v", r.Items)
	}
	if !r.eventDone(s345LicEv) {
		t.Fatalf("发过手环后应设一次性事件 %d，实际 %v", s345LicEv, r.EventFlags)
	}

	r.Pets = s337Pets(30)
	kingRun(t, r, poruPick(t, s, r, s337Team, 3))
	if countItem(r.Items, s337Stone) != 1 || countItem(r.Items, s337Cert) != 0 || len(r.Pets) != 0 {
		t.Fatalf("交给涛线后应得涛之圣石、收走许可证和迪欧，实际 items=%v pets=%+v", r.Items, r.Pets)
	}
	if !r.eventDone(s337Ev45) || countItem(r.Items, s345Bracelet) != 1 {
		t.Fatalf("涛线完成后 56 应完成，手环仍在，实际 flags=%v items=%v", r.EventFlags, r.Items)
	}
}

// 不足 14 个良木的块仍收 2478（原版如此，不属于这次的改动）：13 个给 16000（块 2），1 个给 400（块 14）。
func TestStage345FewerWoodStillTakesLicense(t *testing.T) {
	cases := []struct{ wood, block int }{
		{13, 2},
		{1, 14},
	}
	for _, c := range cases {
		s := kingService()
		r := kingRecord(s345Items(true, c.wood), nil, nil)
		kingRun(t, r, kingPick(t, s, r, s345Sainasu, c.block))
		if countItem(r.Items, s337Cert) != 0 || countItem(r.Items, s345Bracelet) != 0 || countItem(r.Items, s345Wood) != 0 {
			t.Fatalf("%d 个良木的块 %d 应收走许可证和良木、不给手环，实际 items=%v", c.wood, c.block, r.Items)
		}
		if r.eventDone(s345LicEv) {
			t.Fatalf("不给手环的块不应设一次性事件 %d", s345LicEv)
		}
	}
}

// 没有许可证：14 个良木只听“没有采伐许可证的人有来往喔”（块 0，原版台词），不给手环、不收良木。
func TestStage345NoLicenseNoBracelet(t *testing.T) {
	s := kingService()
	r := kingRecord(s345Items(false, 14), nil, nil)
	if idx := s345Pick(t, s, r, s345Sainasu); idx != 0 {
		t.Fatalf("没有许可证、14 个良木时赛尚应说块 0 的台词，实际第 %d 块", idx)
	}
	if countItem(r.Items, s345Bracelet) != 0 || countItem(r.Items, s345Wood) != 14 {
		t.Fatalf("没有许可证不应给手环也不收良木，实际 items=%v", r.Items)
	}
}

// 手环不重复给：已有手环，或发过一次性事件（即使手环丢了），14 个良木 + 许可证都不再命中手环块（块 1）。
// 实测落到块 2（13 个良木的块：收许可证、给 16000 石币）。手环块不再命中时，选块按原版顺序取第一个成立的块，
// 所以许可证会被换成石币；这是原版的同一套选块规则，见 docs/quest.md「采伐许可证与涛、岚两线（STAGE-345）」。
func TestStage345BraceletNotGivenTwice(t *testing.T) {
	s := kingService()

	holding := kingRecord(s345Items(true, 14, s345Bracelet), nil, nil)
	if idx := s345Pick(t, s, holding, s345Sainasu); idx == 1 {
		t.Fatalf("已有手环时手环块不应命中")
	}
	if countItem(holding.Items, s345Bracelet) != 1 {
		t.Fatalf("已有手环时手环数量应为 1，实际 %v", holding.Items)
	}

	lost := kingRecord(s345Items(true, 14), []int{s345LicEv}, nil)
	if idx := s345Pick(t, s, lost, s345Sainasu); idx == 1 {
		t.Fatalf("发过一次性事件后即使手环丢了，手环块也不应再命中")
	}
}
