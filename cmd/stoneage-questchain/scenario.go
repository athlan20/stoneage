package main

// 三个场景和推荐批次（STAGE-213 的“一、模型”）：
//   - A 现状：白名单里的脚本，支持的块；
//   - B 开 all：白名单加整本支持的脚本，支持的块（和 STAGE-214 的 STONEAGE_NPC_EVENT_OPEN=all 一样）；
//   - C 理想：所有脚本、所有块（忽略 Unsupported）。
// 闭包分趟跑：先 A，再 B，最后 C，所以每个事实的第一条推导尽量只用靠前的脚本。

import (
	"sort"
	"strings"
)

// Rules 是放开规则：白名单（npcEventEnabled）和整本支持的脚本。正式运行时用 gamedata，测试里换成小表。
type Rules struct {
	Enabled func(file string) bool
	Full    func(file string) bool
}

func (r Rules) allowA(u *Unit) bool {
	return r.Enabled(u.File) && u.Block.Unsupported == ""
}

func (r Rules) allowB(u *Unit) bool {
	return (r.Enabled(u.File) || r.Full(u.File)) && u.Block.Unsupported == ""
}

func allowC(*Unit) bool { return true }

// CloseA、CloseB、CloseC 是三个场景的闭包。
func (m *Model) CloseA(r Rules) *Closure { return m.Close(r.allowA) }
func (m *Model) CloseB(r Rules) *Closure { return m.Close(r.allowA, r.allowB) }
func (m *Model) CloseC(r Rules) *Closure { return m.Close(r.allowA, r.allowB, allowC) }

// AllowA、AllowB、AllowC 给等级需求用（Levels 的 allow 参数）。
func (r Rules) AllowA() Allow { return r.allowA }
func (r Rules) AllowB() Allow { return r.allowB }
func (r Rules) AllowC() Allow { return allowC }

// Batch 是推荐的一批放开的脚本。
type Batch struct {
	Name  string
	Files []string // 本批新放开的脚本（不含白名单），升序
}

// batchDirs 是目录的顺序（king 已在 STAGE-220 放开，保留在最前，其余按这个顺序）。
var batchDirs = []string{"king", "jaruga", "sainasu", "giiru", "poru", "quiz", "extra", "seimu", "genout", "chatroom"}

// stage197Files 是 STAGE-197 新补齐的脚本，推荐批次里排第二批。
var stage197Files = []string{
	"jaruga/event/bait_03a2",
	"jaruga/event/bait_03b2",
	"jaruga/event/bait_03c2",
	"jaruga/event/event06_1",
	"jaruga/event/oev_ed1",
	"jaruga/event/oev_edb",
}

// dirOf 返回脚本所在的目录（第一段路径）；没有目录的文件归到“(根)”。
func dirOf(file string) string {
	if i := strings.Index(file, "/"); i >= 0 {
		return file[:i]
	}
	return "(根)"
}

// PlanBatches 把还没放开、但整本支持的脚本分成推荐批次：STAGE-197 的六个，其余目录（按 batchDirs 的顺序，
// 没列出的目录按名字排在最后）。空批次不返回；king 目录已在 STAGE-220 放开，所以不再出现在批次里。
func (r Rules) PlanBatches(files []string) []Batch {
	var cand []string
	for _, f := range files {
		if r.Full(f) && !r.Enabled(f) {
			cand = append(cand, f)
		}
	}
	taken := map[string]bool{}
	var batches []Batch
	add := func(name string, match func(f string) bool) {
		var list []string
		for _, f := range cand {
			if !taken[f] && match(f) {
				taken[f] = true
				list = append(list, f)
			}
		}
		if len(list) > 0 {
			sort.Strings(list)
			batches = append(batches, Batch{Name: name, Files: list})
		}
	}
	add("king 目录（精灵王的传说）", func(f string) bool { return dirOf(f) == "king" })
	add("STAGE-197 补齐的 6 个", func(f string) bool { return contains(stage197Files, f) })
	for _, dir := range batchDirs[1:] {
		d := dir
		add(d+" 目录", func(f string) bool { return dirOf(f) == d })
	}
	add("其余目录", func(string) bool { return true })
	return batches
}

// contains 报告 list 里有没有 s。
func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// CloseCumulative 是白名单加上前 n 批脚本的闭包（批次分析用）。
func (m *Model) CloseCumulative(r Rules, batches []Batch, n int) *Closure {
	on := map[string]bool{}
	for _, b := range batches[:n] {
		for _, f := range b.Files {
			on[f] = true
		}
	}
	return m.Close(func(u *Unit) bool {
		return (r.Enabled(u.File) || on[u.File]) && u.Block.Unsupported == ""
	})
}
