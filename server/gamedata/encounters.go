package gamedata

// 遇敌数据（STAGE-40，docs/protocol.md 第 10 章）：遇敌区域、敌群、敌人三张表，
// 由 tools/gen-encounters.py 从原版服务端的 encount.txt / group1.txt / enemy.txt 生成。
// 选取算法（按地图和坐标找区域、按权重抽敌群和敌人、数量上限）在 server/battle/encounter.go，
// 照 gmsv/char/enemy.c 的 ENEMY_getEnemy 移植。

import (
	"bufio"
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

//go:embed encounts.tsv
var encountTSV string

//go:embed enemygroups.tsv
var enemyGroupTSV string

//go:embed enemies.tsv
var enemyTSV string

// 遇敌三张表的列数，与 tools/gen-encounters.py 的输出一致。
const (
	encountColumns      = 33 // 区域ID、区域编号、地图、x1、y1、x2、y2、zorder、最多敌人数、CEP 上下限、原始行号、原始序号，然后 10 组（敌群、权重）
	enemyGroupColumns   = 23 // 敌群编号、两个道具条件，然后 10 组（敌人、权重）
	enemyVariantColumns = 32 // 编号、模板、等级、同场、经验、PetFlag，然后 10 组（掉落、概率）、随机替换、战术串、战术模式、武器样式
)

// EncounterArea 是 encount.txt 的一行：一个矩形遇敌区域。
type EncounterArea struct {
	Key        string // 稳定区域 ID：地图-原编号-矩形（同键重复时追加序号，STAGE-152）
	ID         int    // 原始区域编号（不同地图上可能重复，如 430、639）
	Floor      int
	X1, Y1     int // 矩形的对角两点（已按小大排好）
	X2, Y2     int
	ZOrder     int // ≤ 0 的行查找时忽略；重叠取最大，平局取文件靠前
	MaxEnemies int // 这一区域一场战斗的最多敌人数
	// CEPMin/CEPMax 是原版走格遇敌概率的上下限。本服务端不实现 CEP 累积，
	// 只留静态判断：上限 0 表示原版“永不遇敌”的安全区（城镇、室内）。
	CEPMin, CEPMax int
	SourceLine     int // encount.txt 里的行号，诊断用
	SourceSeq      int // encount.txt 里第几条有效区域记录（0 起），文件顺序即覆盖平局顺序
	Groups         [10]EncounterGroupRef
}

// NeverEncounter 报告该区域在原版是否永不遇敌（CEP 上限 0）。
func (a *EncounterArea) NeverEncounter() bool { return a.CEPMax <= 0 }

// EncounterGroupRef 是区域里的一个敌群槽位：编号加抽取权重；ID 为 -1 表示空槽。
type EncounterGroupRef struct {
	ID     int
	Weight int
}

// Contains 报告坐标是否落在区域里（闭区间，同原版 PointInRect）。
func (a *EncounterArea) Contains(x, y int) bool {
	return x >= a.X1 && x <= a.X2 && y >= a.Y1 && y <= a.Y2
}

// EnemyGroup 是 group1.txt 的一行：一个敌群。
type EnemyGroup struct {
	ID            int
	AppearItem    int // 持有这个道具的物品编号才出现（-1 无条件）
	NotAppearItem int // 持有这个道具时不出现（-1 无条件）
	Enemies       [10]EnemyRef
}

// EnemyRef 是敌群里的一个敌人槽位：编号加生成权重；ID 为 -1 表示空槽。
type EnemyRef struct {
	ID     int
	Weight int
}

// EnemyVariant 是 enemy.txt 的一行：遇敌用的一个敌人（同一种怪物可以有多个变体）。
type EnemyVariant struct {
	ID            int
	Template      int // enemybase.txt 的模板编号（宠物模板表 PetByID 用同一个编号）
	LVMin         int
	LVMax         int
	CreateMax     int // 同一场里这种敌人最多出现几份（乘以敌群里的同名槽位数）
	CreateMin     int
	Exp           int // -1 表示按公式（ENEMY_getExp）算，STAGE-60 起结算优先用这里的值
	PetFlag       int
	Drops         [10]EnemyDrop
	RandomOf      []int  // ENEMY_RandomEnemyArray 的替换候选；空表示普通敌人
	TacticsOption string // 怪物 AI 的选项串（battle_ai.c 的 BATTLE_ai_normal，STAGE-60）
	Tactics       int    // 战术模式：1 = ai_normal；其他值原版没有对应实现（不动）
	Style         int    // 武器样式 1–7：怪物出生时配对应的武器（enemy.c），决定反击系数与暴击加成
}

// EnemyDrop 是一个掉落槽位：物品编号与概率（原版的数值，语义在 STAGE-60 接入）。
type EnemyDrop struct {
	Item int // -1 表示空槽
	Prob int
}

var (
	encountOnce     sync.Once
	encountAreas    []*EncounterArea // 按文件顺序
	encountIndex    map[int][]*EncounterArea
	enemyGroupOnce  sync.Once
	enemyGroupTable map[int]*EnemyGroup
	enemyOnce       sync.Once
	enemyTable      map[int]*EnemyVariant
)

// EncounterAreaAt 按地图和坐标找遇敌区域：多个区域重叠时取 zorder 最大的，
// 平局取文件里靠前的行（ENCOUNT_getEncountAreaArray）；不在任何区域里返回 nil。
func EncounterAreaAt(floor, x, y int) *EncounterArea {
	encountOnce.Do(loadEncounts)
	var best *EncounterArea
	for _, a := range encountAreas {
		if a.Floor != floor || a.ZOrder <= 0 || !a.Contains(x, y) {
			continue
		}
		if best == nil || a.ZOrder > best.ZOrder {
			best = a
		}
	}
	return best
}

// EnemyGroupByID 按编号取敌群；没有这个编号（比如区域引用了缺失敌群）返回 false。
func EnemyGroupByID(id int) (*EnemyGroup, bool) {
	enemyGroupOnce.Do(loadEnemyGroups)
	g, ok := enemyGroupTable[id]
	return g, ok
}

// EnemyByID 按编号取敌人；不认识返回 false。
func EnemyByID(id int) (*EnemyVariant, bool) {
	enemyOnce.Do(loadEnemies)
	e, ok := enemyTable[id]
	return e, ok
}

func loadEncounts() {
	encountAreas = nil
	encountIndex = map[int][]*EncounterArea{}
	eachTSV(encountTSV, encountColumns, func(f []string) {
		a := &EncounterArea{
			Key: f[0], ID: tsvNum(f[1]), Floor: tsvNum(f[2]),
			X1: tsvNum(f[3]), Y1: tsvNum(f[4]), X2: tsvNum(f[5]), Y2: tsvNum(f[6]),
			ZOrder: tsvNum(f[7]), MaxEnemies: tsvNum(f[8]),
			CEPMin: tsvNum(f[9]), CEPMax: tsvNum(f[10]),
			SourceLine: tsvNum(f[11]), SourceSeq: tsvNum(f[12]),
		}
		if a.X1 > a.X2 {
			a.X1, a.X2 = a.X2, a.X1
		}
		if a.Y1 > a.Y2 {
			a.Y1, a.Y2 = a.Y2, a.Y1
		}
		for i := range a.Groups {
			a.Groups[i] = EncounterGroupRef{ID: tsvNum(f[13+i*2]), Weight: tsvNum(f[14+i*2])}
		}
		encountAreas = append(encountAreas, a)
		encountIndex[a.Floor] = append(encountIndex[a.Floor], a)
	})
}

// EncounterAreasOn 返回这张地图的所有区域行（文件顺序），不做覆盖过滤、也不看 CEP。
// 场景页的“遇敌”列在它之上再做生效范围与敌人配置的筛选（server/character 的 encounter_areas）。
func EncounterAreasOn(floor int) []*EncounterArea {
	encountOnce.Do(loadEncounts)
	return encountIndex[floor]
}

// EncounterAreaByKey 按稳定区域 ID 取区域；找不到返回 false。
func EncounterAreaByKey(key string) (*EncounterArea, bool) {
	encountOnce.Do(loadEncounts)
	for _, a := range encountAreas {
		if a.Key == key {
			return a, true
		}
	}
	return nil, false
}

func loadEnemyGroups() {
	enemyGroupTable = map[int]*EnemyGroup{}
	eachTSV(enemyGroupTSV, enemyGroupColumns, func(f []string) {
		g := &EnemyGroup{ID: tsvNum(f[0]), AppearItem: tsvNum(f[1]), NotAppearItem: tsvNum(f[2])}
		for i := range g.Enemies {
			g.Enemies[i] = EnemyRef{ID: tsvNum(f[3+i*2]), Weight: tsvNum(f[4+i*2])}
		}
		enemyGroupTable[g.ID] = g
	})
}

func loadEnemies() {
	enemyTable = map[int]*EnemyVariant{}
	eachTSV(enemyTSV, enemyVariantColumns, func(f []string) {
		e := &EnemyVariant{
			ID: tsvNum(f[0]), Template: tsvNum(f[1]),
			LVMin: tsvNum(f[2]), LVMax: tsvNum(f[3]),
			CreateMax: tsvNum(f[4]), CreateMin: tsvNum(f[5]),
			Exp: tsvNum(f[6]), PetFlag: tsvNum(f[7]),
		}
		for i := range e.Drops {
			e.Drops[i] = EnemyDrop{Item: tsvNum(f[8+i*2]), Prob: tsvNum(f[9+i*2])}
		}
		if cand := f[28]; cand != "" {
			for _, s := range strings.Split(cand, ",") {
				e.RandomOf = append(e.RandomOf, tsvNum(s))
			}
		}
		e.TacticsOption = f[29]
		e.Tactics = tsvNum(f[30])
		e.Style = tsvNum(f[31])
		enemyTable[e.ID] = e
	})
}

// eachTSV 按行拆 TSV：跳过注释行和列数不对的行，f 是列数组。
func eachTSV(text string, columns int, fn func(f []string)) {
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != columns {
			continue
		}
		fn(f)
	}
}

func tsvNum(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}
