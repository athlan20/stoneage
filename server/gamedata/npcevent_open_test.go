package gamedata

// STAGE-214：配置开关 STONEAGE_NPC_EVENT_OPEN 的解析和放开判断。脚本名都从数据里现找，不写死会变的名字；
// 白名单里的 jaruga/event/event04_1 和一个现找的整本支持、未放开的脚本作参照（写死的脚本会被后续需求放开）。

import (
	"reflect"
	"testing"
)

// resetNpcEventOpen 测试结束时把开关恢复成默认（空），免得影响同一个包里的其他测试。
func resetNpcEventOpen(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetNpcEventOpen("") })
}

// pickFullFiles 在数据里找两类脚本：整本都支持、不在白名单里的（ok）；含不支持块、不在白名单里的（bad）；
// 同时数出整本都支持、不在白名单里的个数（extra）。按文件名取最小的，结果稳定。
func pickFullFiles() (ok, bad string, extra int) {
	for file, full := range npcFileFull() {
		if npcEventEnabled[file] {
			continue
		}
		if full {
			extra++
			if ok == "" || file < ok {
				ok = file
			}
		} else if bad == "" || file < bad {
			bad = file
		}
	}
	return ok, bad, extra
}

func TestNpcEventOpenDefault(t *testing.T) {
	resetNpcEventOpen(t)
	sum := SetNpcEventOpen("")
	if sum.Mode != "whitelist" || sum.Extra != 0 || len(sum.Unknown) != 0 {
		t.Fatalf("默认应只看白名单: %+v", sum)
	}
	ok, _, _ := pickFullFiles()
	if ok == "" || NpcEventEnabled(ok) {
		t.Errorf("默认时白名单之外的脚本 %q 不应放开", ok)
	}
	if !NpcEventEnabled("jaruga/event/event04_1") {
		t.Error("默认时白名单里的脚本应放开")
	}
	if got := SetNpcEventOpen("   "); got.Mode != "whitelist" || (ok != "" && NpcEventEnabled(ok)) {
		t.Errorf("只有空白也应恢复默认: %+v", got)
	}
}

func TestNpcEventOpenAll(t *testing.T) {
	resetNpcEventOpen(t)
	ok, bad, extra := pickFullFiles()
	if ok == "" || bad == "" || extra == 0 {
		t.Fatalf("数据里应同时有整本支持和含不支持块的脚本（ok=%q bad=%q extra=%d）", ok, bad, extra)
	}
	// 没有块的脚本（如 genout/msg_1006_18_22）不算整本都支持，不放开。
	SetNpcEventOpen("all")
	if NpcEventEnabled("genout/msg_1006_18_22") {
		t.Error("没有块的脚本不应因 all 放开")
	}
	for _, spec := range []string{"all", " ALL ", "All"} {
		sum := SetNpcEventOpen(spec)
		if sum.Mode != "all" || sum.Extra != extra || sum.Unknown != nil {
			t.Fatalf("%q 的解析结果应为 all、额外 %d 个: %+v", spec, extra, sum)
		}
		if !NpcEventEnabled(ok) {
			t.Errorf("%q：整本支持的 %s 应放开", spec, ok)
		}
		if NpcEventEnabled(bad) {
			t.Errorf("%q：含不支持块的 %s 不应放开", spec, bad)
		}
		if !NpcEventEnabled("jaruga/event/event04_1") {
			t.Errorf("%q：白名单里的脚本应一直放开", spec)
		}
	}
}

func TestNpcEventOpenList(t *testing.T) {
	resetNpcEventOpen(t)
	// 空项、前后空白、重复项都要忽略；ruri05 含不支持块（Break），指定了也放开；nope/x 数据里没有。
	ok := pickOKFile(t)
	sum := SetNpcEventOpen(" " + ok + " , ,jaruga/event/ruri05," + ok + ",nope/x ")
	if sum.Mode != "list" || sum.Extra != 2 || !reflect.DeepEqual(sum.Unknown, []string{"nope/x"}) {
		t.Fatalf("列表解析错误: %+v", sum)
	}
	if !NpcEventEnabled(ok) {
		t.Error("列出的整本支持脚本应放开")
	}
	if !NpcEventEnabled("jaruga/event/ruri05") {
		t.Error("列出的含不支持块脚本也应放开（指定名单按指定放开）")
	}
	if NpcEventEnabled("poru/poruman") {
		t.Error("没列的脚本不应放开")
	}
	if !NpcEventEnabled("jaruga/event/event04_1") {
		t.Error("列表模式下白名单里的脚本应一直放开")
	}
	// 列出的全是白名单里的：不算额外放开。
	if sum := SetNpcEventOpen("jaruga/event/event04_1"); sum.Mode != "list" || sum.Extra != 0 {
		t.Errorf("白名单里的脚本不算额外放开: %+v", sum)
	}
}

func TestNpcEventOpenReset(t *testing.T) {
	resetNpcEventOpen(t)
	SetNpcEventOpen("all")
	if !NpcEventEnabled(pickOKFile(t)) {
		t.Fatal("先开了 all，前提不成立")
	}
	sum := SetNpcEventOpen("")
	if sum.Mode != "whitelist" || NpcEventEnabled(pickOKFile(t)) {
		t.Errorf("SetNpcEventOpen(\"\") 应恢复默认: %+v", sum)
	}
}

// pickOKFile 取一个整本支持、不在白名单里的脚本文件名（找不到就失败）。
func pickOKFile(t *testing.T) string {
	t.Helper()
	ok, _, _ := pickFullFiles()
	if ok == "" {
		t.Fatal("数据里找不到整本支持的脚本")
	}
	return ok
}
