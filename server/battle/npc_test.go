package battle

// NPC 发起的战斗（STAGE-96）：阵容按敌人编号的顺序占格（10 只占满敌方一方），大体型的规则（全场最多 5 只、
// 第 6 个起换到前排），以及 battle_start、battle、battle_end 里的 npc 对象（台词只在该出现的地方出现）。

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

const (
	bigTemplate    = 8 // 体型 1（大型）的宠物模板（gamedata/petbase.tsv）
	normalTemplate = 1 // 普通体型的宠物模板
)

// npcTestSide 是玩家方的最小快照（阵容测试不关心玩家的数值）。
func npcTestSide() *SideInput {
	return &SideInput{Name: "勇者小明", Level: 1, HP: 50, MaxHP: 50, MP: 10, MaxMP: 10, Attack: 10, Defense: 10, Agility: 10, WeaponType: -1}
}

func zeroRnd(int) int { return 0 }

// 里昂蛙王的阵容：1 只黑蛙王（1685）加 9 只黑蛙（1681），按顺序占满敌方的 10 格（前排 15–19 先，后排 10–14 接着）。
func TestBuildNPCPlacesTenInOrder(t *testing.T) {
	enemies := []int{1685, 1681, 1681, 1681, 1681, 1681, 1681, 1681, 1681, 1681}
	b, ok := BuildNPC("b_1", "蛙洞", npcTestSide(), enemies, zeroRnd)
	if !ok {
		t.Fatal("里昂蛙王的阵容应当摆得出来")
	}
	var nos []int
	for no := 0; no < 20; no++ {
		if u := b.Unit(no); u != nil && u.Kind == KindMonster {
			nos = append(nos, no)
		}
	}
	if want := []int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}; !reflect.DeepEqual(nos, want) {
		t.Errorf("敌方单位号 = %v，要 %v", nos, want)
	}
	if first := b.Unit(15); first == nil || first.TemplateID != 842 {
		t.Errorf("15 号应是黑蛙王（模板 842），实际 %+v", first)
	}
	if b.Unit(10).TemplateID != 841 {
		t.Errorf("后排的 10 号应是黑蛙（模板 841）")
	}
}

// 大体型最多 5 只：第 6 个起要和前 5 个里的普通体型换位（大型站前排）。
func TestPlaceEnemyBigRule(t *testing.T) {
	big := &gamedata.EnemyVariant{ID: 9001, Template: bigTemplate, LVMin: 1, LVMax: 1}
	normal := &gamedata.EnemyVariant{ID: 9002, Template: normalTemplate, LVMin: 1, LVMax: 1}

	var out []*gamedata.EnemyVariant
	bigCnt := 0
	for i := 0; i < 5; i++ {
		var res seatResult
		out, res = placeEnemy(out, big, &bigCnt)
		if res != seated {
			t.Fatalf("前 5 只大型应当都放下，第 %d 只结果 %v", i+1, res)
		}
	}
	if _, res := placeEnemy(out, big, &bigCnt); res != seatFull {
		t.Errorf("大型已满 5 只，第 6 只应当不出（seatFull），实际 %v", res)
	}

	// 前 5 个里有普通体型时，新来的大型换到前排：原来的普通体型挪到第 6 格。
	out = []*gamedata.EnemyVariant{normal, normal, normal, normal, normal}
	bigCnt = 0
	out, res := placeEnemy(out, big, &bigCnt)
	if res != seated || len(out) != 6 || out[0] != big || out[5] != normal || bigCnt != 1 {
		t.Errorf("大型应换到前排：res %v 阵容 %v bigCnt %d", res, out, bigCnt)
	}
}

// 协议里的 npc 对象：battle_start 带开战台词；battle 查询只带身份，不带台词；battle_end 只在赢了且有结束台词时带。
func TestNPCFightViews(t *testing.T) {
	b, ok := BuildNPC("b_1", "蛙洞", npcTestSide(), []int{1685}, zeroRnd)
	if !ok {
		t.Fatal("阵容应当摆得出来")
	}
	b.SetNPC(NPCFight{NPCRef: NPCRef{ID: 1735, Name: "里昂蛙王"}, Start: "看我怎么修理你们这些人类！", End: "我不甘心啊！…"})

	start := b.StartView()
	if start.Kind != "npc" || start.NPC == nil || start.NPC.Say != "看我怎么修理你们这些人类！" {
		t.Fatalf("battle_start 的 kind 或 npc 不对: %+v", start)
	}
	if raw, _ := json.Marshal(start.NPC); string(raw) != `{"id":1735,"name":"里昂蛙王","say":"看我怎么修理你们这些人类！"}` {
		t.Errorf("battle_start 的 npc 序列化 = %s", raw)
	}

	q := b.QueryView(0, 0).QueryCore
	if q.NPC == nil || q.NPC.ID != 1735 || q.Kind != "npc" {
		t.Fatalf("battle 查询应带 npc 身份: %+v", q)
	}
	if raw, _ := json.Marshal(q.NPC); string(raw) != `{"id":1735,"name":"里昂蛙王"}` {
		t.Errorf("battle 查询的 npc 不带台词，实际 %s", raw)
	}

	b.result = "win"
	if end := b.endView(); end.NPC == nil || end.NPC.Say != "我不甘心啊！…" {
		t.Errorf("赢了且有结束台词，battle_end 应带 npc.say: %+v", end.NPC)
	}
	b.result = "lose"
	if end := b.endView(); end.NPC != nil {
		t.Errorf("输了 battle_end 不带 npc: %+v", end.NPC)
	}
	b.result = "win"
	b.NPC.End = ""
	if end := b.endView(); end.NPC != nil {
		t.Errorf("没有结束台词的赢不带 npc: %+v", end.NPC)
	}
}

// 野外战斗不带 npc，kind 仍是 wild（同之前的协议）。
func TestWildBattleHasNoNPC(t *testing.T) {
	b, err := Build("b_2", npcTestSide(), "win")
	if err != nil {
		t.Fatal(err)
	}
	if b.Kind != "wild" || b.StartView().NPC != nil || b.QueryView(0, 0).QueryCore.NPC != nil {
		t.Errorf("野外战斗不应带 NPC: kind %s", b.Kind)
	}
}
