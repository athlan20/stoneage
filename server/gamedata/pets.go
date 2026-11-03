package gamedata

// 宠物模板表（STAGE-31，docs/ui.md 宠物页）。数据由 tools/gen-petbase.py 从原版服务端
// gmsv/data/enemybase.txt 与 petskill.txt 生成，名字已转成 UTF-8；客户端不需要宠物数据。

import (
	"bufio"
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

//go:embed petbase.tsv
var petbaseTSV string

//go:embed petskill.tsv
var petskillTSV string

// PetSkillSlots 是宠物技能栏的个数（enemybase.txt 的 E_T_PETSKILL1..7）。
const PetSkillSlots = 7

// 两张 TSV 的列数，与 tools/gen-petbase.py 的输出一致。
const (
	petColumns   = 27 // 编号、名字，然后 25 个数字列（见 PetTemplate；稀有度是最后一列）
	skillColumns = 9  // 编号、名字、函数、参数、场合、目标码、学习价格、说明、非法
)

// 技能的场合（petskill.txt 的场合列，原版 PETSKILL_FIELDTYPE）：
// 0 战斗和战斗外都能用，1 只能在战斗中用，2 只能在战斗外用（如加工、料理，STAGE-144）。
const (
	PetSkillFieldAll    = 0
	PetSkillFieldBattle = 1
	PetSkillFieldMap    = 2
)

// PetSkill 是宠物技能模板（petskill.txt 的一行）。Func 是技能的函数名
// （PETSKILL_NormalAttack 等），一期引擎按它分派执行；Arg 是函数的参数
// （如连击的次数）。Field 是场合，Target 是原版的目标码，换算成协议取值见 BattleTarget。
type PetSkill struct {
	ID      int
	Name    string
	Func    string
	Arg     string
	Field   int
	Target  int
	Cost    int    // 学习价格（PETSKILL_COST）：宠物技能店的收费，乘店的 skill_rate（STAGE-143）
	Comment string // 说明（PETSKILL_COMMENT）：宠物技能店的列表里显示
	Illegal bool   // 非法（PETSKILL_ILLEGAL，原版行首 T）：不上架
}

// PetTemplate 是宠物模板（enemybase.txt 的一行）。ID 就是“宠物编号”（E_T_TEMPNO）。
type PetTemplate struct {
	ID         int
	Name       string
	Image      int // E_T_IMGNUMBER：图像编号（一期客户端不显示）
	SkillSlots int // E_T_SLOT：技能栏位数
	PetFlag    int // E_T_PETFLG：> 0 才能当宠物给出去（见 stoneage-admin givepet）
	Size       int // E_T_SIZE
	LimitLevel int // E_T_LIMITLEVEL：0 表示没有限制
	InitNum    int // E_T_INITNUM：能力的基础值
	LvUpPoint  int // E_T_LVUPPOINT：每级增长（小数部分舍去，同原版服务端的 atoi）
	Base       Growth
	ModAI      int // E_T_MODAI：忠诚用的系数，≤ 0 时按 100
	Get        int // E_T_GET：捕捉难度（BATTLE_CaptureCheck 的 Df_Ge，越小越好抓）
	Rare       int // E_T_RARE：稀有度 0/1/2（宠物店卖价按它定系数，见 NpcPetshop.SellCost；不是品质 PetRank）
	Earth      int
	Water      int
	Fire       int
	Wind       int
	Skills     [PetSkillSlots]int // 技能编号，-1 表示这个栏位没有技能
}

var (
	petOnce    sync.Once
	petTable   map[int]PetTemplate
	skillOnce  sync.Once
	skillTable map[int]PetSkill
)

// PetByID 按宠物编号取模板；不认识的编号返回 false。
func PetByID(id int) (PetTemplate, bool) {
	petOnce.Do(loadPets)
	t, ok := petTable[id]
	return t, ok
}

// PetSkillName 按技能编号取技能名（UTF-8）；不认识的编号返回 false。
func PetSkillName(id int) (string, bool) {
	skillOnce.Do(loadPetSkills)
	t, ok := skillTable[id]
	return t.Name, ok
}

// PetSkillByID 按技能编号取模板；不认识的编号返回 false。
func PetSkillByID(id int) (PetSkill, bool) {
	skillOnce.Do(loadPetSkills)
	t, ok := skillTable[id]
	return t, ok
}

// loadPets 解析 petbase.tsv。列顺序见 tools/gen-petbase.py：编号、名字，然后
// 图像、栏位数、PETFLG、体型、等级上限、INITNUM、LVUPPOINT、资质四项、MODAI、地水火风、技能七项。
func loadPets() {
	petTable = make(map[int]PetTemplate, 952)
	sc := bufio.NewScanner(strings.NewReader(petbaseTSV))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != petColumns {
			continue
		}
		num := func(i int) int {
			v, _ := strconv.Atoi(f[i])
			return v
		}
		t := PetTemplate{
			ID: num(0), Name: f[1], Image: num(2), SkillSlots: num(3), PetFlag: num(4), Size: num(5),
			LimitLevel: num(6), InitNum: num(7), LvUpPoint: num(8),
			Base:  Growth{Vital: num(9), Strength: num(10), Toughness: num(11), Dexterity: num(12)},
			ModAI: num(13), Get: num(14), Earth: num(15), Water: num(16), Fire: num(17), Wind: num(18),
			Rare: num(26),
		}
		for i := range t.Skills {
			t.Skills[i] = num(19 + i) // 技能1..7 在第 19–25 列（风属性之后）
		}
		petTable[t.ID] = t
	}
}

// loadPetSkills 解析 petskill.tsv：技能编号、名字、函数、参数、场合、目标码、学习价格、说明、非法。
func loadPetSkills() {
	skillTable = make(map[int]PetSkill, 32)
	sc := bufio.NewScanner(strings.NewReader(petskillTSV))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != skillColumns {
			continue
		}
		id, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		field, _ := strconv.Atoi(f[4])
		target, _ := strconv.Atoi(f[5])
		cost, _ := strconv.Atoi(f[6])
		skillTable[id] = PetSkill{ID: id, Name: f[1], Func: f[2], Arg: f[3], Field: field, Target: target,
			Cost: cost, Comment: f[7], Illegal: f[8] == "1"}
	}
}
