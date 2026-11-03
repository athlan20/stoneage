package character

// STAGE-348：赛尚（jaruga/event/20501ev5）块 15 条件是“有采伐许可证 2478、没有良木”，台词“怎么连一根都没有”，
// 原版选「是」就收走 2478，没有任何奖励；许可证补不回来，涛线（poru/team02 ACCEPT 56）从此交不了。
// 覆盖层让这一块不再收 2478（tools/npc_event_overlay.py 的 LICENSE_PATCHES）。纯逻辑，真实白名单与选块。

import "testing"

// 带着许可证、没有良木：命中块 15，选「是」后许可证仍在；之后仍能把它交给涛线。
func TestStage348NoWoodKeepsLicense(t *testing.T) {
	s := kingService()
	r := kingRecord(s345Items(true, 0), []int{s337Ev43, s337Ev46}, []int{s337Ev41, s337Ev45})

	kingRun(t, r, kingPick(t, s, r, s345Sainasu, 15))
	if countItem(r.Items, s337Cert) != 1 {
		t.Fatalf("没有良木时赛尚不应收走许可证，实际 items=%v", r.Items)
	}

	r.Pets = s337Pets(30)
	kingRun(t, r, poruPick(t, s, r, s337Team, 3))
	if countItem(r.Items, s337Stone) != 1 || countItem(r.Items, s337Cert) != 0 {
		t.Fatalf("之后应能把许可证交给涛线换涛之圣石，实际 items=%v", r.Items)
	}
}

// 块 2~14（带 1~13 个良木换石币）有收益，保持原版：仍收走许可证（STAGE-345 的同名用例已覆盖 13、1 个，这里补 7 个）。
func TestStage348CoinBlocksUnchanged(t *testing.T) {
	s := kingService()
	r := kingRecord(s345Items(true, 7), nil, nil)
	kingRun(t, r, kingPick(t, s, r, s345Sainasu, 8))
	if countItem(r.Items, s337Cert) != 0 || countItem(r.Items, s345Wood) != 0 {
		t.Fatalf("7 个良木的块应照原版收走许可证和良木，实际 items=%v", r.Items)
	}
}
