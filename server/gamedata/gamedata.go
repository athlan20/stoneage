// Package gamedata 放从原版服务端搬过来的游戏规则数据：出生点、新手配置、数值公式、外形编号和地图名。
// 出处写在各自的注释里；游戏规则以原版服务端 C 源码（original-data/gmsv）为准。
package gamedata

// Hometown 是出生地（村庄）：出生后站在对应村庄的村长之家里。
// 出处：gmsv/char/char_data.c 的 elders 表。
type Hometown struct {
	Name  string
	Floor int
	X, Y  int
}

// Hometowns 按出生地序号排列：0 萨姆吉尔、1 玛丽娜丝、2 加加村、3 卡鲁它那（docs/protocol.md 5.6）。
var Hometowns = []Hometown{
	{"萨姆吉尔", 1006, 15, 22},
	{"玛丽娜丝", 2006, 20, 16},
	{"加加村", 3006, 21, 16},
	{"卡鲁它那", 4006, 14, 20},
}

// 战斗胜利的金币倍率：gold += 人物等级 × BattleGold（setup.cf 的 BATTLEGOLD=10，
// BATTLE_GetExp 的 _BATTLE_GOLD 分支，STAGE-60）。
const BattleGold = 10

// 新手配置。出处：gmsv/setup.cf 的 TRANS、LV、GOLD，以及 gmsv/char/char.c 的 CHAR_createNewChar
// （MP 固定 100，HP 创建时填满）。新手赠送的宠物见 newplayerpets.go（STAGE-62）；
// 物品原版 ITEM1–ITEM15 为空，不送。
const (
	NewPlayerLevel          = 1
	NewPlayerTransmigration = 0
	NewPlayerGold           = 1000000
	NewPlayerMaxMP          = 100
	// DefaultDirection 是出生时的朝向（0 为北，顺时针每次加 45°，4 为南，面向屏幕）。
	DefaultDirection = 4
)

// MaxHP 是最大生命值。出处：gmsv/char/char.c 的 CHAR_complianceParameter：
// (体力×4 + 腕力 + 耐久力 + 速度) 按点数计。
func MaxHP(vital, strength, toughness, dexterity int) int {
	return vital*4 + strength + toughness + dexterity
}

// 外形个数（docs/login.md）。
const (
	BodyCount  = 12
	ColorCount = 4
	EyeCount   = 5
	MouthCount = 5
)

// BodyImage 是身体图像编号：100000 + 外形×20 + 颜色×5。
func BodyImage(body, color int) int { return 100000 + body*20 + color*5 }

// FaceImage 是头像编号：30000 + 外形×100 + 颜色×25 + 眼型×5 + 嘴型。
// 这个换算满足原版服务端 CHAR_checkFaceImageNumber 的校验。
func FaceImage(body, color, eye, mouth int) int {
	return 30000 + body*100 + color*25 + eye*5 + mouth
}

// MapName 返回地图名（UTF-8）；不认识的 floor 返回空字符串和 false。
func MapName(floor int) (string, bool) {
	n, ok := mapNames[floor]
	return n, ok
}
