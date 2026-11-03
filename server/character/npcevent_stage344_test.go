package character

// STAGE-344：四圣石任务宠物核对。四只宠物都是 NPC 的 ACCEPT 块里的 GetPet 送的：金格萨贝鲁（poru/mary01，怪物 310，
// 模板 194）、迪欧（poru/mary06，1481 → 719）、鲁乌（poru/mary04，1483 → 720）、加美（poru/mary05，1482 → 721）。
// 原版数据：original-data/gmsv/data/enemy.txt（怪物行）与 enemybase.txt（模板行），生成规则见
// char/enemy.c 的 ENEMY_createPetFromEnemyIndex。这里用真实的 GetPet 块经 planEvent 生成宠物，逐项对照下面的常量。
// 资质的随机（RAND）用两个极端夹住：rnd 恒 0 与恒 n-1，结果是确定的。

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// s344Want 是一只任务宠物的原版数值（enemybase.txt 的模板行，列名见 tools/gen-petbase.py）。
// base 是资质 体力、腕力、耐久、速度（原版 E_T_BASEVITAL..BASEDEX）；elem 是 地、水、火、风；
// skills 是技能栏（原版空栏是 0，即“待机”，转换表写 -1，见 petbase.tsv 的说明，这里按转换表写）。
type s344Want struct {
	npc, enemy, tmpl int
	name             string
	img, slots       int
	petFlg, size     int
	limit            int // E_T_LIMITLEVEL：0 表示没有限制
	initNum, lvUp    int
	base             [4]int
	modAI, get, rare int
	elem             [4]int
	skills           [gamedata.PetSkillSlots]int
	rank             int // ENEMY_getRank：资质和分档（100/95/90/85/80/0 → 0..5）
}

// s344Pets 是四只宠物的原版数值。金格萨贝鲁的 CRITICAL、COUNTER 是 3、3（原版只抄写，见文档），这里不比。
var s344Pets = []s344Want{
	{npc: 1316, enemy: 310, tmpl: 194, name: "金格萨贝鲁", img: 100330, slots: 7, petFlg: 1, size: 1, limit: 0,
		initNum: 25, lvUp: 4, base: [4]int{19, 29, 18, 29}, modAI: 270, get: -1, rare: 1, elem: [4]int{30, 0, 0, 70},
		skills: [7]int{1, 2, 40, -1, -1, -1, -1}, rank: 1},
	{npc: 1327, enemy: 1481, tmpl: 719, name: "迪欧", img: 100401, slots: 7, petFlg: 1, size: 0, limit: 30,
		initNum: 20, lvUp: 5, base: [4]int{35, 35, 20, 10}, modAI: 100, get: 0, rare: 0, elem: [4]int{0, 50, 50, 0},
		skills: [7]int{1, 2, -1, -1, -1, -1, -1}, rank: 0},
	{npc: 1318, enemy: 1483, tmpl: 720, name: "鲁乌", img: 100432, slots: 7, petFlg: 1, size: 0, limit: 30,
		initNum: 20, lvUp: 5, base: [4]int{35, 35, 20, 10}, modAI: 100, get: 0, rare: 0, elem: [4]int{0, 50, 50, 0},
		skills: [7]int{1, 2, -1, -1, -1, -1, -1}, rank: 0},
	{npc: 1322, enemy: 1482, tmpl: 721, name: "爱丽儿队的加美", img: 100259, slots: 5, petFlg: 1, size: 0, limit: 33,
		initNum: 22, lvUp: 4, base: [4]int{22, 23, 19, 23}, modAI: 210, get: 7, rare: 0, elem: [4]int{0, 0, 60, 40},
		skills: [7]int{1, 2, -1, -1, -1, -1, -1}, rank: 3},
}

// s344Lo 与 s344Hi 是资质随机的两个极端：RAND(0,4)-2 取 -2 或 +2，每次成长取第 0 项（体力）或第 3 项（速度）。
func s344Lo(int) int   { return 0 }
func s344Hi(n int) int { return n - 1 }

// s344Block 找 NPC 脚本里送这只怪物（GetPet:enemy）的块，找不到就失败。
func s344Block(t *testing.T, npc, enemy int) *gamedata.EventBlock {
	t.Helper()
	script, ok := gamedata.NpcEventOf(npc)
	if !ok {
		t.Fatalf("NPC %d 没有任务脚本", npc)
	}
	for i := range script.Blocks {
		if b := &script.Blocks[i]; len(b.GetPet) == 1 && b.GetPet[0] == enemy {
			return b
		}
	}
	t.Fatalf("NPC %d（%s）没有 GetPet:%d 的块", npc, script.File, enemy)
	return nil
}

// s344Give 用真实的 planEvent 跑一趟交易（空背包、没有宠物），返回送出的宠物。选块的条件不在这里测。
func s344Give(t *testing.T, blk *gamedata.EventBlock, rnd func(int) int) petRec {
	t.Helper()
	plan, fail, ok := planEvent(&record{Level: 1}, blk, eventSel{group: -1}, rnd)
	if !ok || len(plan.pets) != 1 {
		t.Fatalf("事件 %d（%s 块）应送出一只宠物，实际 ok=%v fail=%q pets=%+v", blk.Event, blk.Type, ok, fail, plan.pets)
	}
	return plan.pets[0]
}

// s344Expect 按原版公式算一只 1 级宠物的资质、能力和血量（char/enemy.c：资质 ±2 之后、成长 +10 之前的值是 ALLOCPOINT；
// 能力 = ((等级-1)×LVUPPOINT + INITNUM) × 成长后的资质；血量 = (4×体力 + 腕力 + 耐久 + 敏捷) ÷ 100，取整）。
func s344Expect(w s344Want, rnd func(int) int) (gamedata.Growth, gamedata.Ability, int) {
	base := gamedata.Growth{
		Vital: w.base[0] + rnd(5) - 2, Strength: w.base[1] + rnd(5) - 2,
		Toughness: w.base[2] + rnd(5) - 2, Dexterity: w.base[3] + rnd(5) - 2,
	}
	grown := base
	for i := 0; i < 10; i++ {
		switch rnd(4) {
		case 0:
			grown.Vital++
		case 1:
			grown.Strength++
		case 2:
			grown.Toughness++
		case 3:
			grown.Dexterity++
		}
	}
	perLevel := w.initNum // 1 级：(1-1)×LVUPPOINT + INITNUM
	stats := gamedata.Ability{
		Vital: perLevel * grown.Vital, Strength: perLevel * grown.Strength,
		Toughness: perLevel * grown.Toughness, Dexterity: perLevel * grown.Dexterity,
	}
	hp := max(1, (4*stats.Vital+stats.Strength+stats.Toughness+stats.Dexterity)/100)
	return base, stats, hp
}

// TestStage344TemplatesMatchOriginal：模板表（petbase.tsv）逐项对照原版 enemybase.txt；怪物（enemies.tsv）对照 enemy.txt。
func TestStage344TemplatesMatchOriginal(t *testing.T) {
	for _, w := range s344Pets {
		tmpl, ok := gamedata.PetByID(w.tmpl)
		if !ok {
			t.Fatalf("模板 %d（%s）不在 petbase 里", w.tmpl, w.name)
		}
		if tmpl.Name != w.name || tmpl.Image != w.img || tmpl.SkillSlots != w.slots || tmpl.PetFlag != w.petFlg ||
			tmpl.Size != w.size || tmpl.LimitLevel != w.limit || tmpl.InitNum != w.initNum || tmpl.LvUpPoint != w.lvUp ||
			tmpl.ModAI != w.modAI || tmpl.Get != w.get || tmpl.Rare != w.rare {
			t.Errorf("模板 %d 与原版不一致：%+v", w.tmpl, tmpl)
		}
		if tmpl.Base != (gamedata.Growth{Vital: w.base[0], Strength: w.base[1], Toughness: w.base[2], Dexterity: w.base[3]}) {
			t.Errorf("模板 %d 资质 = %+v，原版 %v", w.tmpl, tmpl.Base, w.base)
		}
		if tmpl.Earth != w.elem[0] || tmpl.Water != w.elem[1] || tmpl.Fire != w.elem[2] || tmpl.Wind != w.elem[3] {
			t.Errorf("模板 %d 地水火风 = %d/%d/%d/%d，原版 %v", w.tmpl, tmpl.Earth, tmpl.Water, tmpl.Fire, tmpl.Wind, w.elem)
		}
		if tmpl.Skills != w.skills {
			t.Errorf("模板 %d 技能栏 = %v，原版 %v", w.tmpl, tmpl.Skills, w.skills)
		}
		for _, id := range tmpl.Skills {
			if id < 0 {
				continue
			}
			if _, ok := gamedata.PetSkillName(id); !ok {
				t.Errorf("模板 %d 的技能 %d 在 petskill 里查不到", w.tmpl, id)
			}
		}
		if got := gamedata.PetRank(tmpl); got != w.rank {
			t.Errorf("模板 %d 的资质分档 = %d，原版 ENEMY_getRank 应为 %d", w.tmpl, got, w.rank)
		}

		enemy, ok := gamedata.EnemyByID(w.enemy)
		if !ok {
			t.Fatalf("怪物 %d 不在 enemies 里", w.enemy)
		}
		if enemy.Template != w.tmpl || enemy.LVMin != 1 || enemy.LVMax != 1 {
			t.Errorf("怪物 %d：模板 %d 等级 %d-%d，原版模板 %d 等级 1-1", w.enemy, enemy.Template, enemy.LVMin, enemy.LVMax, w.tmpl)
		}
		script, ok := gamedata.NpcEventOf(w.npc)
		if !ok {
			t.Errorf("NPC %d 没有任务脚本", w.npc)
		} else if !gamedata.NpcEventEnabled(script.File) {
			t.Errorf("NPC %d（%s）不在白名单里，对话会说“还不能对话”", w.npc, script.File)
		}
	}
}

// TestStage344GivenPetMatchesOriginal：真实的 GetPet 块生成的宠物，逐项对照原版公式（资质随机取两个极端）。
func TestStage344GivenPetMatchesOriginal(t *testing.T) {
	for _, w := range s344Pets {
		blk := s344Block(t, w.npc, w.enemy)
		if blk.Type != "ACCEPT" {
			t.Fatalf("%s 的 GetPet 块应是 ACCEPT（原版走 NPC_AcceptDel，mode 1），实际 %s", w.name, blk.Type)
		}
		for _, rc := range []struct {
			label string
			rnd   func(int) int
		}{{"资质最低", s344Lo}, {"资质最高", s344Hi}} {
			p := s344Give(t, blk, rc.rnd)
			base, stats, hp := s344Expect(w, rc.rnd)
			if p.TemplateID != w.tmpl || p.Name != w.name || p.Level != 1 || p.Rank != w.rank {
				t.Errorf("%s（%s）：模板 %d 名字 %q 等级 %d 分档 %d，原版 模板 %d %s 1 级 分档 %d",
					w.name, rc.label, p.TemplateID, p.Name, p.Level, p.Rank, w.tmpl, w.name, w.rank)
			}
			if p.Base != base {
				t.Errorf("%s（%s）资质 = %+v，原版公式应为 %+v", w.name, rc.label, p.Base, base)
			}
			if p.Stats != stats {
				t.Errorf("%s（%s）能力 = %+v，原版公式应为 %+v", w.name, rc.label, p.Stats, stats)
			}
			if p.HP != hp {
				t.Errorf("%s（%s）血量 = %d，原版公式应为 %d（满血）", w.name, rc.label, p.HP, hp)
			}
			if p.Skills != w.skills {
				t.Errorf("%s（%s）技能栏 = %v，原版 %v", w.name, rc.label, p.Skills, w.skills)
			}
			if p.Origin == nil || p.Origin.Level != 1 || p.Origin.Stats != stats {
				t.Errorf("%s（%s）起始值 = %+v，应是 1 级、能力 %+v", w.name, rc.label, p.Origin, stats)
			}
			if p.VariableAI != 0 || p.LimitLevel != nil || p.Kills != 0 || p.Transmigration != 0 {
				t.Errorf("%s（%s）初始忠诚变动、等级上限覆盖、击杀、转生应为 0/空，实际 %+v", w.name, rc.label, p)
			}
			if p.EventGiven {
				t.Errorf("%s 是 ACCEPT 块送的，原版 mode 1 不打 ENDEVENT，不应标记为任务送的宠物", w.name)
			}
		}
	}
}

// TestStage344EventGivenFollowsBlockType：任务送的标记只在 REQUEST 路径上打（原版 NPC_EventAddPet 的 mode 0，
// 由 NPC_EventAdd 调用）；ACCEPT 和 MESSAGE 经 NPC_AcceptDel（mode 1），不打。同一个块换个类型来验证。
func TestStage344EventGivenFollowsBlockType(t *testing.T) {
	w := s344Pets[0]
	base := s344Block(t, w.npc, w.enemy)
	for _, c := range []struct {
		typ  string
		want bool
	}{{"REQUEST", true}, {"ACCEPT", false}, {"MESSAGE", false}} {
		blk := *base
		blk.Type = c.typ
		if got := s344Give(t, &blk, s344Lo).EventGiven; got != c.want {
			t.Errorf("%s 块送的金格萨贝鲁 EventGiven = %v，原版应为 %v", c.typ, got, c.want)
		}
	}
}
