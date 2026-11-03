package gamedata

import (
	"bufio"
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

//go:embed itemset.tsv
var itemsetTSV string

// 物品类型（item.h 的 ITEM_CATEGORY 枚举，即 itemset.txt 的 ItemType 列）。
// 穿脱校验用各部位；战斗用武器类别换算（battle 包的 CounterTbl）。
const (
	ItemTypeFist       = 0  // 拳套（空手）
	ItemTypeAxe        = 1  // 斧
	ItemTypeClub       = 2  // 棍棒
	ItemTypeSpear      = 3  // 枪
	ItemTypeBow        = 4  // 弓
	ItemTypeBoomerang  = 17 // 回旋标
	ItemTypeBoundThrow = 18 // 投掷斧头
	ItemTypeBreakThrow = 19 // 投掷石
)

// ItemTemplate 是物品模板（docs/protocol.md 5.17）：名字和效果文字已在生成时转成 UTF-8，
// 客户端不需要物品数据。Type 是物品表里的类型，决定能穿在哪个部位（equip.go）。
// Level、Attack、Defence、Agility 是穿戴要求和数值加成（docs/ui.md 背包页 B，见 tools/gen-itemset.py）。
// Critical 是武器的暴击加成（battle_event.c 的 At_Soubi，STAGE-60）。
// Luck、Charm 是装备时的运气、魅力修正（item.c 的 ITEM_equipEffect，STAGE-61）。
// UseFunc、Argument、AbleUse、Target 是使用物品的字段（STAGE-30，docs/protocol.md 5.31）。
// Cost 是商店买卖的基价（STAGE-70，itemset.txt 第 18 列，原版 ITEM_COST）。
type ItemTemplate struct {
	Name     string
	Effect   string
	Type     int
	Cost     int
	Level    int
	Attack   int
	Defence  int
	Agility  int
	Luck     int
	Charm    int
	Critical int
	UseFunc  string // 使用函数（ITEM_USEFUNC），空表示不是消耗品
	Argument string // 使用参数（ITEM_ARGUMENT），如 “体40”“气80”“毒”
	AbleUse  int    // 可使用场合（ITEM_ABLEUSEFIELD）：0 都可以、1 战斗中、2 战斗外
	Target   int    // 目标（ITEM_TARGET）：0 自己、1 他人、2 全己方、3 全敌方、4 全体
	// MagicID、MagicMP 是装备自带的精灵（STAGE-190，ITEM_MAGICID / ITEM_MAGICUSEMP）：穿在身上就能在战斗里施放，
	// MagicID 是魔法表编号，-1 表示没有；MP 以物品为准，不用魔法表里推算的值。
	MagicID int
	MagicMP int
	// CanMergeFrom、CanMergeTo 是合成标记（STAGE-205，itemset.txt 第 82、83 列）：能作材料、能作配方产物。
	// 原子见 ItemIngredients。
	CanMergeFrom bool
	CanMergeTo   bool
	// Durability、MaxDurability 是装备的当前耐久与最大耐久（STAGE-209，itemset.txt 第 30、31 列，原版
	// ITEM_DAMAGECRUSHE / ITEM_MAXDAMAGECRUSHE）。最大耐久为 0 的物品不损坏；只有人物的装备会损坏。
	Durability    int
	MaxDurability int
	// STAGE-297：精工（docs/inlay.md 2.4）。InlayCode 是精工类别码（原版 ITEM_TYPECODE，itemset.txt 第 4 列），
	// 只有类别码对得上的材料才能镶；InlayInit 是初始镶嵌码（ITEM_INLAYCODE，第 5 列）。两者都为空表示不能精工。
	// 覆盖表（inlay_overlay.go）可以补写。OtherDamage、OtherDefc 是伤、吸（ITEM_OTHERDAMAGE / ITEM_OTHERDEFC，第 32、33 列）；
	// HP、MP 是装备的生命、魔力修正（ITEM_MODIFYHP / ITEM_MODIFYMP，第 43、45 列）。这四项战斗里还没接入（docs/inlay.md 2.6）。
	InlayCode   string
	InlayInit   string
	OtherDamage int
	OtherDefc   int
	HP          int
	MP          int
}

// itemColumns 是 itemset.tsv 的列数，与 tools/gen-itemset.py 的输出一致。
const itemColumns = 38

var (
	itemOnce  sync.Once
	itemTable map[int]ItemTemplate
)

// ItemByID 按物品编号取模板；不认识的编号返回 false。
func ItemByID(id int) (ItemTemplate, bool) {
	itemOnce.Do(loadItems)
	t, ok := itemTable[id]
	return t, ok
}

// loadItems 解析 tools/gen-itemset.py 生成的 TSV：编号、名字、效果、类型、基价、等级、攻击、防御、敏捷、
// 运气、魅力、暴击修正、使用函数、使用参数、可使用场合、目标、精灵编号、精灵 MP（STAGE-70 起有基价列，STAGE-190 起有精灵两列），
// 之后是合成的可作材料、可作配方和 5 组原子（STAGE-205，第 18–29 列，原子名与数值见 synthesis.go）。
func loadItems() {
	itemTable = make(map[int]ItemTemplate, 12620)
	itemIngredients = make(map[int][]Ingredient)
	sc := bufio.NewScanner(strings.NewReader(itemsetTSV))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != itemColumns {
			continue
		}
		id, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		var nums [7]int
		for i := range nums {
			nums[i], _ = strconv.Atoi(f[5+i])
		}
		itemTable[id] = ItemTemplate{
			Name: f[1], Effect: f[2],
			Type: atoi(f[3]), Cost: atoi(f[4]),
			Level: nums[0], Attack: nums[1], Defence: nums[2], Agility: nums[3],
			Luck: nums[4], Charm: nums[5],
			// nums 覆盖第 5–11 列（等级、攻防敏、运魅）；暴击、使用函数、参数、场合、目标从第 12 列起单独取。
			Critical: atoi(f[11]),
			UseFunc:  f[12], Argument: f[13],
			AbleUse: atoi(f[14]), Target: atoi(f[15]),
			MagicID: atoi(f[16]), MagicMP: atoi(f[17]),
			CanMergeFrom: atoi(f[18]) == 1, CanMergeTo: atoi(f[19]) == 1,
			// STAGE-209：耐久（docs/durability.md）；最大耐久为 0 的物品不损坏。
			Durability: atoi(f[30]), MaxDurability: atoi(f[31]),
			// STAGE-297：精工和伤、吸、HP、MP（TSV 第 32–37 列，追加在最后）。
			InlayCode: f[32], InlayInit: f[33],
			OtherDamage: atoi(f[34]), OtherDefc: atoi(f[35]), HP: atoi(f[36]), MP: atoi(f[37]),
		}
		if ings := parseIngredients(f[20:30]); len(ings) > 0 {
			itemIngredients[id] = ings
		}
	}
}

// atoi 是出错当 0 的整数解析（TSV 里的空白列）。
func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
