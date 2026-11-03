// 宠物（STAGE-31，docs/protocol.md 5.19、5.20，docs/ui.md 宠物页）：pets 查询、GivePet 管理命令，
// 以及 character_pets_changed 通知后的重读。栏位 0–19（MaxPetSlots）。JSON 的内容在 petview.go。

package character

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// DefaultPetNotifyChannel 是“角色宠物变了”的 PostgreSQL 通知频道：stoneage-admin givepet
// 在同一个事务里 NOTIFY，运行中的服务端收到后重读该角色的宠物并推送 pets_changed。
const DefaultPetNotifyChannel = "character_pets_changed"

// MaxPetSlots 是宠物栏位数（STAGE-346：20 个，编号 0–19；docs/ui.md 宠物页）。
const MaxPetSlots = 20

// MaxPetLevel 是 givepet 能给的最高等级（gmsv/setup.cf 的 MAXLEVEL=160）。
const MaxPetLevel = 160

// 宠物状态（character_pets.state，0–4）。出战最多 1 只、待命最多 3 只（STAGE-32），骑乘最多 1 只（STAGE-123）。
const (
	petStateRest = iota
	petStateStandby
	petStateBattle
	petStatePooled // 寄放在宠物店（STAGE-73，数据库 state=3、slot=50+池位）
	petStateRide   // 骑乘（STAGE-123，数据库 state=4，docs/ride.md）
)

// petStateNames 是协议里 state 的取值（docs/protocol.md 5.19）；寄放（3）不出现在随身列表，
// 宠物店窗口自会说明，协议 state 值不变。
var petStateNames = [...]string{"rest", "standby", "battle", "pooled", "ride"}

// petRec 是 character_pets 表的一行。Base（资质）只留在服务端，不进协议。
type petRec struct {
	Slot           int
	TemplateID     int
	Name           string
	Level          int
	Exp            int64
	Stats          gamedata.Ability // 能力，1/100 点
	Base           gamedata.Growth  // 资质：生成时 ±2 之后、+10 之前的值（CHAR_ALLOCPOINT）
	Rank           int
	VariableAI     int
	HP             int
	State          int
	Skills         [gamedata.PetSkillSlots]int // 技能编号，-1 为空栏位
	Kills          int                         // 击杀数（STAGE-150）：出过手的宠物每击败一只怪 +1，只在服务端记录
	EventGiven     bool                        // 任务送的宠物（STAGE-197）：普通 PET 条件不数它，不对玩家显示
	Transmigration int                         // 转生次数（STAGE-196）：骑乘要求不超过 2（RIDE_TRANS_LIMIT）
	Origin         *PetOrigin                  // STAGE-215：起始等级与能力，进库时的值，之后不改；nil 为历史宠物，没有基准
	LimitLevel     *int                        // 个体等级上限（STAGE-234）：对话改写过才有值，nil 用模板的 LIMITLEVEL，见 pettalk.go
}

// carried 报告宠物在随身栏位（0–MaxPetSlots-1），而不是寄放在宠物店（slot ≥ PooledSlotBase）。
// 战斗、捕捉、战后写回、护士只认随身宠物：寄放的宠物既不能上战场，也不占随身栏位、不回血、不长经验。
func (p *petRec) carried() bool { return p.Slot >= 0 && p.Slot < MaxPetSlots }

const petColumns = `slot, template_id, name, level, exp,
	vital, strength, toughness, dexterity, base_vital, base_strength, base_toughness, base_dexterity,
	rank, variable_ai, hp, state, skills, kills, event_given, transmigration, ` + petOriginColumns + `, limit_level`

// scanPet 读一行 character_pets（列顺序同 petColumns）。
func scanPet(row interface{ Scan(dest ...any) error }) (petRec, error) {
	var p petRec
	var skills []int32
	var oLevel, oVital, oStrength, oToughness, oDexterity *int // STAGE-215：起始值五列，可空
	var limit *int                                             // STAGE-234：个体等级上限，可空
	err := row.Scan(&p.Slot, &p.TemplateID, &p.Name, &p.Level, &p.Exp,
		&p.Stats.Vital, &p.Stats.Strength, &p.Stats.Toughness, &p.Stats.Dexterity,
		&p.Base.Vital, &p.Base.Strength, &p.Base.Toughness, &p.Base.Dexterity,
		&p.Rank, &p.VariableAI, &p.HP, &p.State, &skills, &p.Kills, &p.EventGiven, &p.Transmigration,
		&oLevel, &oVital, &oStrength, &oToughness, &oDexterity, &limit)
	if err != nil {
		return p, err
	}
	p.LimitLevel = limit
	p.Origin = originOf(oLevel, oVital, oStrength, oToughness, oDexterity)
	for i := range p.Skills {
		if i < len(skills) {
			p.Skills[i] = int(skills[i])
		}
	}
	return p, nil
}

// loadPets 读角色现有的宠物，按栏位从小到大。
func (s *Service) loadPets(ctx context.Context, charID int64) ([]petRec, error) {
	return readPets(ctx, s.db, charID, false)
}

// readPets 读角色的宠物，按栏位从小到大；lock 为真时加行锁（在事务里用，见 pets_manage.go）。
func readPets(ctx context.Context, q dbQuerier, charID int64, lock bool) ([]petRec, error) {
	query := `SELECT ` + petColumns + ` FROM character_pets WHERE character_id = $1 ORDER BY slot`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := q.Query(ctx, query, charID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []petRec
	for rows.Next() {
		p, err := scanPet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// handlePets 是查看类请求（docs/protocol.md 5.19）：从内存里的角色数据出宠物快照。
func (s *Service) handlePets(_ context.Context, sess *gateway.Session, _ *protocol.Request) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := l.With(func(r *record) { out = r.petsView() }); err != nil {
		return nil, asProtocolError(err)
	}
	return out, nil
}

// GivePet 是管理命令（stoneage-admin givepet 与协议用例的 admin 行）：按原版服务端
// ENEMY_createPetFromEnemyIndex 的规则生成一只宠物（随机资质），放进角色第一个空的宠物栏位，
// 在同一个事务里 NOTIFY，让正在运行的服务端重读并推送。返回宠物所在的栏位。
// rnd(n) 返回 [0, n) 的随机整数（命令行传真随机数，测试传固定序列）。
// 角色不在线时只写库，等下次进入世界时可见。
func GivePet(ctx context.Context, db *pgxpool.Pool, channel, character string, petID, level int, rnd func(n int) int) (int, error) {
	return GivePetWithLoyalty(ctx, db, channel, character, petID, level, 0, rnd)
}

// GivePetWithLoyalty 同 GivePet，并把宠物忠诚的可变部分设为 loyalty 点（100 即封顶，见 gamedata.AddVariableAI）。
// 给宠物一个确定的忠诚，用在协议用例里（casetest 的 give_pet 行的 loyalty 字段，STAGE-127）；同一个事务里 NOTIFY 一次。
func GivePetWithLoyalty(ctx context.Context, db *pgxpool.Pool, channel, character string, petID, level, loyalty int, rnd func(n int) int) (int, error) {
	tmpl, ok := gamedata.PetByID(petID)
	if !ok {
		return 0, fmt.Errorf("宠物编号 %d 不存在", petID)
	}
	if tmpl.PetFlag <= 0 {
		return 0, fmt.Errorf("编号 %d（%s）不能当宠物（PETFLG 为 0）", petID, tmpl.Name)
	}
	if level < 1 || level > MaxPetLevel {
		return 0, fmt.Errorf("等级要在 1–%d 之间", MaxPetLevel)
	}
	if tmpl.LimitLevel > 0 && level > tmpl.LimitLevel {
		return 0, fmt.Errorf("%s 的等级上限是 %d", tmpl.Name, tmpl.LimitLevel)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var charID int64
	err = tx.QueryRow(ctx, `SELECT id FROM characters WHERE name = $1`, character).Scan(&charID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("找不到角色 %s", character)
	}
	if err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT slot FROM character_pets WHERE character_id = $1`, charID)
	if err != nil {
		return 0, err
	}
	taken := [MaxPetSlots]bool{}
	for rows.Next() {
		var slot int
		if err := rows.Scan(&slot); err != nil {
			rows.Close()
			return 0, err
		}
		if slot >= 0 && slot < MaxPetSlots { // 寄放池的 slot（≥ PooledSlotBase）不占随身栏位
			taken[slot] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	free := -1
	for slot := range taken {
		if !taken[slot] {
			free = slot
			break
		}
	}
	if free < 0 {
		return 0, fmt.Errorf("%s 的宠物栏位已满（%d 只）", character, MaxPetSlots)
	}
	if err := insertPetRow(ctx, tx, charID, free, tmpl, level, rnd); err != nil {
		return 0, err
	}
	if loyalty != 0 {
		if _, err := tx.Exec(ctx, `UPDATE character_pets SET variable_ai = $3 WHERE character_id = $1 AND slot = $2`,
			charID, free, gamedata.AddVariableAI(0, loyalty*100)); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, strconv.FormatInt(charID, 10)); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	slog.Default().Info("管理命令给了宠物", "character", character, "pet", tmpl.Name, "level", level, "slot", free)
	return free, nil
}

// ClearPets 是测试用的管理命令（casetest 的 clear_pets 行）：删掉角色的所有宠物。
// 不涉及出生送宠物的用例用它恢复“角色起初没有宠物”的前提（出生送宠物见 STAGE-62）。
func ClearPets(ctx context.Context, db *pgxpool.Pool, character string) error {
	_, err := db.Exec(ctx, `DELETE FROM character_pets
		WHERE character_id = (SELECT id FROM characters WHERE name = $1)`, character)
	return err
}

// insertPetRow 往 character_pets 写一只由模板生成的宠物（在事务里用）：等级 level、资质随机
// （gamedata.NewPetGrowth，和捕捉、给予同一套规则）、HP 满、休息状态。出生送宠物（STAGE-62）
// 和 givepet 都走这里。
func insertPetRow(ctx context.Context, tx pgx.Tx, charID int64, slot int, tmpl gamedata.PetTemplate, level int, rnd func(n int) int) error {
	base, stats := gamedata.NewPetGrowth(tmpl, level, rnd)
	hp := gamedata.PetMaxHP(stats)
	skills := make([]int32, gamedata.PetSkillSlots)
	for i, id := range tmpl.Skills {
		skills[i] = int32(id)
	}
	// STAGE-215：起始值就是这只宠物生成时的等级和能力，进库后不再改动。
	oLevel, oVital, oStrength, oToughness, oDexterity := originArgs(&PetOrigin{Level: level, Stats: stats})
	_, err := tx.Exec(ctx, `INSERT INTO character_pets (`+petInsertColumns+`)
		VALUES ($1, $2, $3, $4, $5, 0, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)`,
		charID, slot, tmpl.ID, tmpl.Name, level,
		stats.Vital, stats.Strength, stats.Toughness, stats.Dexterity,
		base.Vital, base.Strength, base.Toughness, base.Dexterity,
		gamedata.PetRank(tmpl), 0, hp, petStateRest, skills,
		oLevel, oVital, oStrength, oToughness, oDexterity)
	return err
}

// petInsertColumns 是写一只完整宠物的列（GivePet、出生送宠、捕捉等共用；exp 固定为 0，同原版服务端 CHAR_EXP）。
// STAGE-215：末尾是起始值五列，只在插入时写，之后的 UPDATE 都不碰它们。
const petInsertColumns = `character_id, slot, template_id, name, level, exp,
	vital, strength, toughness, dexterity, base_vital, base_strength, base_toughness, base_dexterity,
	rank, variable_ai, hp, state, skills, ` + petOriginColumns

// reloadPets 重读角色的宠物并推送 pets_changed。角色不在世界中时没有可通知的连接，跳过。
func (s *Service) reloadPets(ctx context.Context, payload string) {
	charID, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		s.opts.Logger.Warn("宠物通知的 payload 不是角色编号", "payload", payload)
		return
	}
	p := s.presenceOf(charID)
	if p == nil {
		return
	}
	pets, err := s.loadPets(ctx, charID)
	if err != nil {
		s.opts.Logger.Warn("重读角色宠物失败", "character", charID, "err", err)
		return
	}
	p.mu.Lock()
	if !p.leaving {
		p.rec.Pets = pets
		p.pushAll("pets_changed", p.rec.petsView())
	}
	p.mu.Unlock()
}
