package character

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
)

// record 是 characters 表的一行（只含服务端要用的列）。
type record struct {
	ID         int64
	AccountID  int64
	Slot       int
	Name       string
	Appearance Appearance
	Stats      Stats
	Elements   Elements
	Hometown   int

	Level, Transmigration int
	SkillPoints           int // 未分配的能力点（STAGE-24，docs/protocol.md 5.21）
	Savepoint             int // 记录的存档点 NPC 编号，0 是没有（STAGE-72，战败送回它的落点）
	RideLevel             int // 骑乘等级 0/40/80/120/200（STAGE-122，骑乘训练师的课程，docs/ride.md §2）
	Exp, Gold             int64
	HP, MP                int

	// TransQuestTotal、TransLevelTotal 是转生累计任务数和累计等级（STAGE-266，tensei_calc.go），转生时累加；老数据为 0。
	TransQuestTotal, TransLevelTotal int

	// SocialHidden 是隐身（characters.social_visible 取反，STAGE-276，docs/protocol.md 5.69）：true 时别人看不见这个角色。
	// SocialNoInvite 是拒绝组队邀请（characters.social_invite 取反）。存反值是为了零值就是默认的“都开”。
	SocialHidden, SocialNoInvite bool
	// SocialNoTrade 是拒绝交易邀请（characters.social_trade 取反，STAGE-294，docs/trade.md 2.8）。
	SocialNoTrade bool
	// SocialDuel 是接受决斗（社交开关 duel，STAGE-306）：只在内存里，不存库；读取时为 false，所以重新进入世界时是关的。
	SocialDuel bool
	// DuelPoint 是决斗点数（characters.duel_point，STAGE-306，docs/pk.md 2.2）。
	DuelPoint int

	// Charm 是魅力的基础值（0–100，战斗中增减，STAGE-61）；Luck 是当日运气（1–5，0 表示还没掷过）；
	// LuckRolledAt 是上次掷运气的时间，nil 表示还没掷过。运气在进入世界时按游戏日掷一次（fortune.go）。
	Charm        int
	Luck         int
	LuckRolledAt *time.Time
	// Overkill 是打飞的累计溢出值（STAGE-63）：只在内存里，随在线会话保留、重新进入世界清零，
	// 同原版 CHAR_WORKULTIMATE 工作变量；不写库。
	Overkill int

	Floor, X, Y, Direction int

	// Magics 是人物的精灵栏位（STAGE-48，characters.magics）：0–4 栏存精灵编号，
	// -1 表示空。进入世界时读一次；变化靠管理命令的通知重读。
	Magics [5]int

	// 人物的魔法熟练度与抗性（STAGE-226，characters.magic_*，下标依次是地、水、火、风）：MagicExp 是攻击时用的
	// 熟练度（原版 CHAR_EARTH_EXP），MagicResist 是防守时的抗性（不含抗魔装备，装备加成在战斗快照里合算），
	// MagicAttExp、MagicDefExp 是熟练度的经验（原版 CHAR_EARTH_ATTMAGIC_EXP、CHAR_EARTH_DEFMAGIC_EXP）。
	MagicExp, MagicResist, MagicAttExp, MagicDefExp [4]int

	// Items 是 character_items 表里这个角色的物品，进入世界时读一次
	// （STAGE-28）；变化只来自管理命令（写库 + 通知重读），离开世界不回写。
	Items []itemRec

	// Pets 是 character_pets 表里这个角色的宠物（STAGE-31），进入世界时读一次，之后靠通知重读。
	Pets []petRec

	// Quests 是 character_quests 表里这个角色的任务进度（STAGE-86），进入世界时读一次，之后靠通知重读。
	Quests []questRec

	// EventFlags 是 character_event_flags 表里这个角色的事件进度（STAGE-94，No 是事件号），只有不对应任务表的
	// 事件号才有行；进入世界时读一次，之后只由任务脚本的交易改（没有通知）。
	EventFlags []questRec

	// metamo 是变幻的精灵的变身状态（STAGE-203，metamo.go）：只在内存里，不写库；nil 表示没有变身。
	metamo *metamoState
}

const selectColumns = `id, account_id, slot, name, body, color, eye, mouth,
	vital, strength, toughness, dexterity, earth, water, fire, wind, hometown,
	level, transmigration, exp, gold, hp, mp, floor, x, y, direction, skill_points, magics,
	charm, luck, luck_rolled_at, savepoint, ride_level, magic_exp, magic_resist, magic_att_exp, magic_def_exp,
	trans_quest_total, trans_level_total, social_visible, social_invite, social_trade, duel_point`

func scanRecord(row pgx.Row) (*record, error) {
	var r record
	var magics, mExp, mResist, mAttExp, mDefExp []int32
	var socialVisible, socialInvite, socialTrade bool
	err := row.Scan(&r.ID, &r.AccountID, &r.Slot, &r.Name,
		&r.Appearance.Body, &r.Appearance.Color, &r.Appearance.Eye, &r.Appearance.Mouth,
		&r.Stats.Vital, &r.Stats.Strength, &r.Stats.Toughness, &r.Stats.Dexterity,
		&r.Elements.Earth, &r.Elements.Water, &r.Elements.Fire, &r.Elements.Wind, &r.Hometown,
		&r.Level, &r.Transmigration, &r.Exp, &r.Gold, &r.HP, &r.MP, &r.Floor, &r.X, &r.Y, &r.Direction, &r.SkillPoints, &magics,
		&r.Charm, &r.Luck, &r.LuckRolledAt, &r.Savepoint, &r.RideLevel,
		&mExp, &mResist, &mAttExp, &mDefExp, &r.TransQuestTotal, &r.TransLevelTotal, &socialVisible, &socialInvite, &socialTrade, &r.DuelPoint)
	r.SocialHidden, r.SocialNoInvite, r.SocialNoTrade = !socialVisible, !socialInvite, !socialTrade
	for i := range r.Magics {
		if i < len(magics) {
			r.Magics[i] = int(magics[i])
		}
	}
	r.MagicExp, r.MagicResist = int4s(mExp), int4s(mResist)
	r.MagicAttExp, r.MagicDefExp = int4s(mAttExp), int4s(mDefExp)
	return &r, err
}

// listByAccount 按栏位从小到大列出账号的所有角色。
func (s *Service) listByAccount(ctx context.Context, accountID int64) ([]*record, error) {
	rows, err := s.db.Query(ctx, `SELECT `+selectColumns+` FROM characters WHERE account_id = $1 ORDER BY slot`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*record
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// findBySlot 取账号某个栏位的角色；没有时返回 nil。只会找到这个账号自己的角色。
func (s *Service) findBySlot(ctx context.Context, accountID int64, slot int) (*record, error) {
	r, err := scanRecord(s.db.QueryRow(ctx, `SELECT `+selectColumns+` FROM characters WHERE account_id = $1 AND slot = $2`, accountID, slot))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// insert 创建角色。栏位已被占用、名字已被使用分别返回对应的协议错误（唯一约束兜底，防并发）。
func (s *Service) insert(ctx context.Context, accountID int64, c *newCharacter, rnd func(n int) int) (*record, error) {
	h := gamedata.Hometowns[c.Hometown]
	maxHP := gamedata.MaxHP(c.Stats.Vital, c.Stats.Strength, c.Stats.Toughness, c.Stats.Dexterity)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	r, err := scanRecord(tx.QueryRow(ctx, `
		INSERT INTO characters (account_id, slot, name, name_key, body, color, eye, mouth, body_image, face_image,
			vital, strength, toughness, dexterity, earth, water, fire, wind, hometown,
			level, transmigration, exp, gold, hp, mp, floor, x, y, direction, charm)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, $11,$12,$13,$14,$15,$16,$17,$18,$19, $20,$21,0,$22,$23,$24,$25,$26,$27,$28,$29)
		RETURNING `+selectColumns,
		accountID, c.Slot, c.Name, c.NameKey,
		c.Appearance.Body, c.Appearance.Color, c.Appearance.Eye, c.Appearance.Mouth,
		gamedata.BodyImage(c.Appearance.Body, c.Appearance.Color),
		gamedata.FaceImage(c.Appearance.Body, c.Appearance.Color, c.Appearance.Eye, c.Appearance.Mouth),
		c.Stats.Vital, c.Stats.Strength, c.Stats.Toughness, c.Stats.Dexterity,
		c.Elements.Earth, c.Elements.Water, c.Elements.Fire, c.Elements.Wind, c.Hometown,
		gamedata.NewPlayerLevel, gamedata.NewPlayerTransmigration, gamedata.NewPlayerGold,
		maxHP, gamedata.NewPlayerMaxMP, h.Floor, h.X, h.Y, gamedata.DefaultDirection, gamedata.NewPlayerCharm))
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		switch pg.ConstraintName {
		case "characters_account_id_slot_key":
			return nil, protocol.Errorf(protocol.CodeSlotOccupied, "这个栏位已经有角色了")
		case "characters_name_key_key":
			return nil, protocol.Errorf(protocol.CodeNameTaken, "角色名已被使用")
		}
	}
	if err != nil {
		return nil, err
	}
	// 出生送宠物（STAGE-62）：和角色在同一个事务里写，进游戏后宠物页就能看到。
	for slot, petID := range gamedata.NewPlayerPetIDs(c.Hometown, c.Appearance.Body, c.Appearance.Color) {
		tmpl, ok := gamedata.PetByID(petID)
		if !ok {
			return nil, fmt.Errorf("出生宠物模板 %d 不存在", petID)
		}
		if err := insertPetRow(ctx, tx, r.ID, slot, tmpl, gamedata.NewPetLevel, rnd); err != nil {
			return nil, err
		}
	}
	// 出生礼包：物品放进背包前几格（背包是空的，格子一定够）。
	for i, g := range s.opts.NewPlayerGifts {
		if _, ok := gamedata.ItemByID(g.ItemID); !ok {
			return nil, fmt.Errorf("出生礼包物品 %d 不存在", g.ItemID)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO character_items (character_id, slot, item_id, count) VALUES ($1, $2, $3, $4)`,
			r.ID, firstBagSlot+i, g.ItemID, g.Count); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

// deleteByID 按角色 id 删除账号的角色（不按栏位删，免得栏位在中途换成另一个角色），返回是否删到了。
// 删除角色时，引用它的数据（排行、名片等）靠外键 ON DELETE CASCADE 一并清理。
func (s *Service) deleteByID(ctx context.Context, accountID, id int64) (bool, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM characters WHERE id = $1 AND account_id = $2`, id, accountID)
	return tag.RowsAffected() > 0, err
}

// errCharacterGone 表示要写的角色已经不在数据库里（被删除了）。
var errCharacterGone = errors.New("角色已不存在")

// save 把世界中的角色状态写回数据库。角色已被删除（更新 0 行）时返回 errCharacterGone，不能静默丢掉。
func (s *Service) save(ctx context.Context, r *record, now time.Time) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE characters SET level = $2, exp = $3, gold = $4, hp = $5, mp = $6,
			floor = $7, x = $8, y = $9, direction = $10, saved_at = $11, charm = $12, skill_points = $13,
			magic_exp = $14, magic_resist = $15, magic_att_exp = $16, magic_def_exp = $17,
			trans_quest_total = $18, trans_level_total = $19
		WHERE id = $1`,
		r.ID, r.Level, r.Exp, r.Gold, r.HP, r.MP, r.Floor, r.X, r.Y, r.Direction, now, r.Charm, r.SkillPoints,
		int32s4(r.MagicExp), int32s4(r.MagicResist), int32s4(r.MagicAttExp), int32s4(r.MagicDefExp),
		r.TransQuestTotal, r.TransLevelTotal)
	if err == nil && tag.RowsAffected() == 0 {
		return errCharacterGone
	}
	return err
}

// markEntered 记录最近一次进入世界的时间，连同进入时掷的运气（STAGE-61）；角色已被删除（更新 0 行）时返回 errCharacterGone。
func (s *Service) markEntered(ctx context.Context, r *record, now time.Time) error {
	tag, err := s.db.Exec(ctx, `UPDATE characters SET last_enter_at = $2, luck = $3, luck_rolled_at = $4 WHERE id = $1`,
		r.ID, now, r.Luck, r.LuckRolledAt)
	if err == nil && tag.RowsAffected() == 0 {
		return errCharacterGone
	}
	return err
}
