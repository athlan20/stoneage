package gamedata

// 配置开关 STONEAGE_NPC_EVENT_OPEN（STAGE-214，docs/server.md）：测试环境在白名单之外再放开任务脚本，生产环境默认不变。
// 取值：空（只看白名单 npcEventEnabled）；all（白名单之外，再放开每个块的 Unsupported 都为空的脚本）；
// 逗号分隔的脚本文件名（白名单之外，再放开这几个，含不支持块的也放开）。开关只影响“脚本是否放开”这一道判断。

import (
	"strings"
	"sync"
	"sync/atomic"
)

// NpcEventOpenSummary 是 SetNpcEventOpen 的解析结果，启动时据此打日志。
type NpcEventOpenSummary struct {
	Mode    string   // whitelist（默认）、all、list
	Extra   int      // 白名单之外多放开的脚本数
	Unknown []string // list 模式下数据里找不到的脚本文件名（按写的顺序）
}

// npcEventOpenSpec 是解析后的开关。SetNpcEventOpen 整体替换它，读的时候不加锁。
type npcEventOpenSpec struct {
	all    bool
	listed map[string]bool
}

var npcEventOpenCur atomic.Pointer[npcEventOpenSpec]

// SetNpcEventOpen 按 spec 设置开关（前后空白去掉，空字符串恢复默认），返回解析结果。启动时调一次；测试里可以反复调。
func SetNpcEventOpen(spec string) NpcEventOpenSummary {
	spec = strings.TrimSpace(spec)
	files := npcFileFull()
	cur := &npcEventOpenSpec{}
	sum := NpcEventOpenSummary{Mode: "whitelist"}
	switch {
	case spec == "":
	case strings.EqualFold(spec, "all"):
		sum.Mode, cur.all = "all", true
		for file, full := range files {
			if full && !npcEventEnabled[file] {
				sum.Extra++
			}
		}
	default:
		sum.Mode, cur.listed = "list", map[string]bool{}
		for _, item := range strings.Split(spec, ",") {
			item = strings.TrimSpace(item)
			if item == "" || cur.listed[item] {
				continue
			}
			cur.listed[item] = true
			if _, ok := files[item]; !ok {
				sum.Unknown = append(sum.Unknown, item)
			} else if !npcEventEnabled[item] {
				sum.Extra++
			}
		}
	}
	if spec == "" {
		npcEventOpenCur.Store(nil)
	} else {
		npcEventOpenCur.Store(cur)
	}
	return sum
}

// npcEventOpenFor 报告脚本文件按当前开关是否放开（不看白名单，白名单由 NpcEventEnabled 判断）。
func npcEventOpenFor(file string) bool {
	cur := npcEventOpenCur.Load()
	switch {
	case cur == nil:
		return false
	case cur.all:
		return npcFileFull()[file]
	default:
		return cur.listed[file]
	}
}

var (
	npcFileOnce       sync.Once
	npcFileFullByName map[string]bool
)

// NpcEventFileFull 返回每个脚本文件是否整本都支持（同 npcFileFull，STAGE-213 的分析工具用）。返回的是内部缓存，调用方不要修改。
func NpcEventFileFull() map[string]bool { return npcFileFull() }

// npcFileFull 返回数据里每个脚本文件 → 是否整本都支持（每个块的 Unsupported 都为空，且至少有一个块）。
// 同一个文件可能挂在几个任务 NPC 上，只要有一个不支持就算不支持。结果缓存，只算一次。
func npcFileFull() map[string]bool {
	npcFileOnce.Do(func() {
		NpcEventOf(0) // 先解析 npc-event.json（只做一次）
		npcFileFullByName = map[string]bool{}
		for _, s := range npcEventByID {
			full := len(s.Blocks) > 0
			for _, b := range s.Blocks {
				if b.Unsupported != "" {
					full = false
					break
				}
			}
			if prev, seen := npcFileFullByName[s.File]; seen {
				full = full && prev
			}
			npcFileFullByName[s.File] = full
		}
	})
	return npcFileFullByName
}
