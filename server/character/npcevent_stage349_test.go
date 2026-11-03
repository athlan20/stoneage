package character

// STAGE-349：先交涛（poru/team02 ACCEPT 56）把采伐许可证 2478 交掉的角色，赛尚（jaruga/event/20501ev5）的手环块
// 原来要求持有 2478，命中不了，手环拿不到，岚线的木竿敌群（持有手环才出现）也出不来。覆盖层给手环块加第二组条件
// （涛已交付 + 14 个良木 + 没有手环 + 没发过手环，不要求许可证），块 0 在涛已交付时不再抢先命中。纯逻辑，真实白名单与选块。

import "testing"

const s349TaoDone = s337Ev45 // 涛已交付的事件 56

// 先交涛、没有许可证，14 个良木：赛尚给手环和 20000 石币，良木清空，一次性事件设上。
func TestStage349BraceletAfterTao(t *testing.T) {
	s := kingService()
	r := kingRecord(s345Items(false, 14), []int{s337Ev43, s337Ev46, s349TaoDone}, nil)

	kingRun(t, r, kingPick(t, s, r, s345Sainasu, 1))
	if countItem(r.Items, s345Bracelet) != 1 || countItem(r.Items, s345Wood) != 0 {
		t.Fatalf("涛已交付、无许可证也应得手环并清走良木，实际 items=%v", r.Items)
	}
	if !r.eventDone(s345LicEv) {
		t.Fatalf("发过手环后应设一次性事件 %d，实际 %v", s345LicEv, r.EventFlags)
	}
	// 再来一次不重复给（手环在身上，或手环丢了但一次性事件已设）。
	r.Items = append(r.Items[:0:0], s345Items(false, 14)...)
	if idx := s345Pick(t, s, r, s345Sainasu); idx == 1 {
		t.Fatalf("发过手环后手环块不应再命中")
	}
}

// 没交涛、没有许可证：仍是块 0 的提示，不给手环、不收良木（原版行为）。
func TestStage349NoTaoNoLicenseStillBlocked(t *testing.T) {
	s := kingService()
	r := kingRecord(s345Items(false, 14), []int{s337Ev43}, []int{s337Ev41})
	if idx := s345Pick(t, s, r, s345Sainasu); idx != 0 {
		t.Fatalf("没交涛、没许可证应说块 0 的提示，实际第 %d 块", idx)
	}
}

// 涛已交付但良木不足 14：没有块命中（不给手环，也不抢良木）。
func TestStage349TaoDoneFewWoodNothing(t *testing.T) {
	s := kingService()
	r := kingRecord(s345Items(false, 13), []int{s337Ev43, s337Ev46, s349TaoDone}, nil)
	if idx := s345Pick(t, s, r, s345Sainasu); idx != -1 {
		t.Fatalf("涛已交付、13 个良木、无许可证不应命中任何块，实际第 %d 块", idx)
	}
}

// 先手环后涛的老顺序不变（STAGE-345 的整链用例仍在），这里再验：持有许可证、涛未交付，14 个良木仍走原来的条件组。
func TestStage349OldOrderStillWorks(t *testing.T) {
	s := kingService()
	r := kingRecord(s345Items(true, 14), []int{s337Ev43, s337Ev46}, []int{s337Ev41, s337Ev45})
	kingRun(t, r, kingPick(t, s, r, s345Sainasu, 1))
	if countItem(r.Items, s345Bracelet) != 1 || countItem(r.Items, s337Cert) != 1 {
		t.Fatalf("先手环：应得手环且许可证仍在，实际 items=%v", r.Items)
	}
}
