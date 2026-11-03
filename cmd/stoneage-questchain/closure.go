package main

// 单调闭包（STAGE-213）：事实只增不减，块在条件满足时执行一次（效果只增，重复执行没有新东西），直到没有新事实为止。
// 分多趟（phases）跑：前一趟的事实保留，后一趟放开更多块。这样事实的第一条推导尽量只用前面放开的块，
// 报告里“用到的脚本”才能看出是不是新放开的脚本。

import "sort"

// Allow 决定一个块在这一趟里能不能执行（场景 A、B、C 和批次都用它）。
type Allow func(u *Unit) bool

// Firing 是一个块的一次执行：块本身，以及触发时用到的事实（条件里选中的那一组，加上 DelPet 的前提）。
type Firing struct {
	Unit  *Unit
	Needs []need
}

// need 是触发时用到的一条事实。ext 为真表示当时还没成立、靠外部可得（不需要脚本），推导时作为叶子；
// 否则它是在这次触发之前就成立的，推导时沿它的第一条推导展开（这样推导不会绕回来）。
type need struct {
	f   Fact
	ext bool
}

// Closure 是闭包的结果：每条成立的事实，以及它的第一条推导（Why；外部可得的事实没有 Why）。
type Closure struct {
	M     *Model
	Has   map[Fact]bool
	Why   map[Fact]*Firing
	fired []bool // 按块的 ID
}

// Close 跑单调闭包。phases 依次是每一趟允许执行的块；最后一趟之后的事实就是结果。
func (m *Model) Close(phases ...Allow) *Closure {
	c := &Closure{M: m, Has: map[Fact]bool{}, Why: map[Fact]*Firing{}, fired: make([]bool, len(m.Units))}
	for _, allow := range phases {
		for added := true; added; {
			added = false
			for _, u := range m.Units {
				if c.fired[u.ID] || !allow(u) {
					continue
				}
				needs, ok := c.check(u)
				if !ok {
					continue
				}
				c.fired[u.ID] = true
				fr := &Firing{Unit: u, Needs: needs}
				for _, f := range u.produce {
					if !c.Has[f] {
						c.Has[f] = true
						c.Why[f] = fr
						added = true
					}
				}
			}
		}
	}
	return c
}

// avail 报告事实现在能不能用：已经成立，或者是外部可得的。
func (c *Closure) avail(f Fact) bool { return c.Has[f] || c.M.external(f) }

// check 判断块能不能执行：前提都可用，且某个候选组里的每一项都能满足。返回选中的组用到的事实。
func (c *Closure) check(u *Unit) ([]need, bool) {
	var pre []need
	for _, p := range u.pre {
		if !c.avail(p) {
			return nil, false
		}
		pre = append(pre, c.use(p))
	}
	for _, group := range u.groups {
		needs := append([]need(nil), pre...)
		ok := true
		for _, t := range group {
			if t.bad {
				ok = false
				break
			}
			if len(t.alts) == 0 {
				continue
			}
			picked := false
			for _, a := range t.alts {
				if c.avail(a) {
					needs = append(needs, c.use(a))
					picked = true
					break
				}
			}
			if !picked {
				ok = false
				break
			}
		}
		if ok {
			return needs, true
		}
	}
	return nil, false
}

// Derivation 是一个事实的推导：经过的块（按文件和块序号排列，去重），以及用到的外部可得事实（道具、宠物）。
type Derivation struct {
	Units    []*Unit
	External []Fact
}

// use 记下一条用到的事实：成立时沿它的推导展开，否则靠外部可得。
func (c *Closure) use(f Fact) need { return need{f: f, ext: !c.Has[f]} }

// Derive 沿第一条推导展开事实 f。f 不成立时返回空的推导。
func (c *Closure) Derive(f Fact) Derivation {
	var d Derivation
	if !c.avail(f) {
		return d
	}
	seenFact := map[Fact]bool{}
	seenUnit := map[int]bool{}
	var walk func(n need)
	walk = func(n need) {
		if seenFact[n.f] {
			return
		}
		seenFact[n.f] = true
		if n.ext {
			d.External = append(d.External, n.f)
			return
		}
		fr := c.Why[n.f]
		if !seenUnit[fr.Unit.ID] {
			seenUnit[fr.Unit.ID] = true
			d.Units = append(d.Units, fr.Unit)
		}
		for _, sub := range fr.Needs {
			walk(sub)
		}
	}
	walk(c.use(f))
	sort.Slice(d.Units, func(i, j int) bool { return d.Units[i].ID < d.Units[j].ID })
	sort.Slice(d.External, func(i, j int) bool { return factLess(d.External[i], d.External[j]) })
	return d
}

// factLess 给外部事实排序（种类、编号）。
func factLess(a, b Fact) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.N < b.N
}
