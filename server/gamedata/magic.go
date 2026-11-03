package gamedata

// 精灵（魔法）模板表（STAGE-48、STAGE-77，docs/protocol.md 10.5 的 magic 指令）。
// 数据由 tools/gen-magic.py 从原版服务端 gmsv/data/magic.txt 生成，名字已转 UTF-8；
// MP 消耗是按等级推算的（原版存在人物习得表里，见生成脚本的说明）。

import (
	"bufio"
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

//go:embed magic.tsv
var magicTSV string

// magicColumns 是 TSV 的列数，与 tools/gen-magic.py 的输出一致（第 11 列是参数，STAGE-202；
// 第 12、13 列是原版目标码和攻击精灵的 3×5 掩码，STAGE-218；第 14 列是攻击精灵的等级，STAGE-227）。
const magicColumns = 14

// MaxMagicSlots 是人物的精灵栏位数（协议 10.5：magic 0–4）。
const MaxMagicSlots = 5

// 精灵的函数（TSV 的“函数”列）：攻击、自愈、治疗他人、状态、解状态、复活，
// STAGE-200 的场地属性、属性反转，以及 STAGE-199 的防御（光、镜子、守护）和复活防御（复活光、复活镜、复活守护）。
const (
	MagicFuncAttack       = "attack"
	MagicFuncHeal         = "heal"
	MagicFuncHealOther    = "heal_other"
	MagicFuncStatus       = "status"
	MagicFuncCure         = "cure"
	MagicFuncRevive       = "revive"
	MagicFuncField        = "field"         // 场地属性：整场战斗的属性、威力、回合（STAGE-200，magic_field.go）
	MagicFuncReverse      = "reverse"       // 属性反转：翻转单位的反转标志并轮换四属性（STAGE-200）
	MagicFuncDefend       = "defend"        // 己方一个挂上防御次数（吸收、反射或无效）（STAGE-199）
	MagicFuncReviveDefend = "revive_defend" // 复活己方一个倒下的单位，同时挂上防御次数（STAGE-199）
	MagicFuncMetamo       = "metamo"        // 变幻的精灵：战斗外变身成自己的宠物，Power 是秒数（STAGE-203）
	MagicFuncDragon       = "dragon"        // 召唤龙阵（奔龙阵、强恩阵、奔雷阵、GM 烟火）：Arg 是图像号，Power 是威力（STAGE-202，magic_dragon.go）
	MagicFuncAttSkill     = "attskill"      // 精灵版宠物技能：Arg 是宠物技能编号，人物用它施放（STAGE-202，magic_attskill.go）
	MagicFuncResist       = "resist"        // 魔抗系：己方一个或全体挂上魔抗、火抗、电抗、冰抗（STAGE-228，battle/magic_resist.go）
)

// MagicTemplate 是一个精灵（magic.txt 的一行）。
type MagicTemplate struct {
	ID    int
	Name  string
	Func  string // 见上面的 MagicFunc 常量
	Elem  string // 攻击系的属性：地水火风；其他为 "-"
	Power int    // 威力（治疗系是回复量，复活是复活的血量）
	MP    int    // 消耗的气力
	All   bool   // true 时不用选目标（攻击系目标码 11、治疗系奶己方全体、状态打敌方全体、解状态救己方全体）
	// 攻击精灵（MagicFuncAttack）的原版目标码和命中掩码（STAGE-218）：Target 是 magic.txt 的第 7 列
	// （1 他人、9 敌方某一个、10 敌方某一排、11 敌方所有人）；Mask 是 attmagic.bin 的 siField，
	// 15 位行优先（3 行 × 5 列），决定实际命中哪些格子。其他函数的 Mask 全为 false。
	Target int
	// 攻击精灵的等级（STAGE-227）：原版参数第 3 段，BATTLE_getMagicAdjustInt 的 MagicLv；其他精灵为 0。
	Lv   int
	Mask [15]bool
	// 状态精灵（MagicFuncStatus）与解状态精灵（MagicFuncCure）的状态名（协议 10.2 的取值，
	// 解状态的“全”是 "all"）；Turns 是挂上的回合数，Success 是成功率的偏移（原版 BATTLE_StatusAttackCheck 的 PerOffset）。
	// 防御精灵（MagicFuncDefend、MagicFuncReviveDefend）的 Status 是 absorb / reflect / vanish，
	// Turns 是防御次数（原版计数器，不随回合减少），Success 为 0。
	Status  string
	Turns   int
	Success int
	// 魔抗系（MagicFuncResist）的 Status 是 magic_resist / fire_resist / thunder_resist / ice_resist，Turns 是回合数，
	// Power 是魔抗的加成百分比（原版参数第 3 段，魔抗 20；火、电、冰抗 10 原版只占槽、不生效）。
	// Arg 是参数列（STAGE-202）：召唤龙阵是图像号（动画编号，终端不画），精灵版宠物技能是宠物技能编号；其余为空。
	Arg string
}

var (
	magicOnce sync.Once
	magicTab  map[int]MagicTemplate
)

// MagicByID 按编号取精灵模板；不认识的编号返回 false。
func MagicByID(id int) (MagicTemplate, bool) {
	magicOnce.Do(loadMagic)
	t, ok := magicTab[id]
	return t, ok
}

// AttSkill 是精灵版宠物技能（MagicFuncAttSkill，STAGE-202）用的宠物技能：Arg 是技能编号。
// 不是这个函数，或编号在 petskill.txt 里找不到时返回 false。
func (t MagicTemplate) AttSkill() (PetSkill, bool) {
	if t.Func != MagicFuncAttSkill {
		return PetSkill{}, false
	}
	id, err := strconv.Atoi(t.Arg)
	if err != nil {
		return PetSkill{}, false
	}
	return PetSkillByID(id)
}

// loadMagic 解析 magic.tsv。列顺序见 tools/gen-magic.py：编号、名字、函数、属性、威力、MP、目标、状态、回合、成功率、参数、
// 原版目标码、掩码、攻击等级。
func loadMagic() {
	magicTab = make(map[int]MagicTemplate, 128)
	sc := bufio.NewScanner(strings.NewReader(magicTSV))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != magicColumns {
			continue
		}
		num := func(i int) int {
			v, _ := strconv.Atoi(f[i])
			return v
		}
		t := MagicTemplate{
			ID: num(0), Name: f[1], Func: f[2], Elem: f[3],
			Power: num(4), MP: num(5), All: f[6] == "all",
			Status: f[7], Turns: num(8), Success: num(9), Arg: f[10],
			Target: num(11), Lv: num(13),
		}
		if t.Status == "-" {
			t.Status = ""
		}
		if t.Arg == "-" {
			t.Arg = ""
		}
		for i, c := range f[12] {
			if i >= len(t.Mask) {
				break
			}
			t.Mask[i] = c != '0'
		}
		magicTab[t.ID] = t
	}
}
