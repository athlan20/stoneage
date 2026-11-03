package main

// 等级需求（STAGE-213）：每个事实的“最低需要的最大等级”，即所有推导里“链路上 LV 条件要求的最大值”的最小值。
// 和闭包一样是不动点：值只会变小，所以一定停。LV<n 不要求等级，LV>n 要 n+1 级，LV=n 要 n 级。

// Levels 对允许的块求每个事实的等级需求。没有出现在结果里的事实不可达。
func (m *Model) Levels(allow Allow) map[Fact]int {
	lv := map[Fact]int{}
	levelOf := func(f Fact) (int, bool) {
		if m.external(f) {
			return 0, true
		}
		v, ok := lv[f]
		return v, ok
	}
	for changed := true; changed; {
		changed = false
		for _, u := range m.Units {
			if !allow(u) {
				continue
			}
			best, ok := unitLevel(u, levelOf)
			if !ok {
				continue
			}
			for _, f := range u.produce {
				if cur, seen := lv[f]; !seen || best < cur {
					lv[f] = best
					changed = true
				}
			}
		}
	}
	return lv
}

// unitLevel 是块执行时的等级需求：前提的等级和候选组里各项的等级（LV 要求，以及用到的事实的等级）取最大，
// 再在能满足的候选组之间取最小。
func unitLevel(u *Unit, levelOf func(Fact) (int, bool)) (int, bool) {
	pre := 0
	for _, p := range u.pre {
		v, ok := levelOf(p)
		if !ok {
			return 0, false
		}
		pre = max(pre, v)
	}
	best, found := 0, false
	for _, group := range u.groups {
		lvl, ok := pre, true
		for _, t := range group {
			if t.bad {
				ok = false
				break
			}
			lvl = max(lvl, t.lv)
			if len(t.alts) == 0 {
				continue
			}
			tl, reached := minLevel(t.alts, levelOf)
			if !reached {
				ok = false
				break
			}
			lvl = max(lvl, tl)
		}
		if ok && (!found || lvl < best) {
			best, found = lvl, true
		}
	}
	return best, found
}

// minLevel 取候选事实里等级最低的一个。
func minLevel(alts []Fact, levelOf func(Fact) (int, bool)) (int, bool) {
	best, found := 0, false
	for _, a := range alts {
		if v, ok := levelOf(a); ok && (!found || v < best) {
			best, found = v, true
		}
	}
	return best, found
}
