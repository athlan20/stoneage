// 加点（STAGE-24，docs/protocol.md 5.21，docs/ui.md 角色页 B）：addpoint 操作类请求，
// 管理命令 stoneage-admin givepoint（与协议用例的 admin 行）发未分配点数，以及 character_points_changed
// 通知后的重读推送（监听在 notify.go）。
//
// 能力点和未分配点数直接写库，不等离开世界才保存：加点的扣点与加能力在同一条 UPDATE 里完成，
// 管理命令也写同一行，两者不会互相覆盖（如果只改内存，管理命令发的点数会被旧的内存值冲掉）。

package character

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// DefaultPointNotifyChannel 是“角色未分配点数变了”的 PostgreSQL 通知频道：stoneage-admin givepoint
// 在同一个事务里 NOTIFY，运行中的服务端收到后重读该角色的能力点并推送 status_changed。
const DefaultPointNotifyChannel = "character_points_changed"

// maxStat 是单项能力的上限：characters 表的能力列是 smallint（0003），超过就存不下。
const maxStat = 32767

// statField 返回协议能力键（与 status.stats 相同）对应的字段；不认识的键返回 nil。
func statField(s *Stats, stat string) *int {
	switch stat {
	case "vital":
		return &s.Vital
	case "strength":
		return &s.Strength
	case "toughness":
		return &s.Toughness
	case "dexterity":
		return &s.Dexterity
	}
	return nil
}

// pointsState 是加点相关的数据：四项能力点和未分配点数。
type pointsState struct {
	Stats       Stats
	SkillPoints int
}

// readPoints 从数据库读角色的能力点和未分配点数；角色不存在时返回 pgx.ErrNoRows。
func (s *Service) readPoints(ctx context.Context, id int64) (pointsState, error) {
	var p pointsState
	err := s.db.QueryRow(ctx,
		`SELECT vital, strength, toughness, dexterity, skill_points FROM characters WHERE id = $1`, id,
	).Scan(&p.Stats.Vital, &p.Stats.Strength, &p.Stats.Toughness, &p.Stats.Dexterity, &p.SkillPoints)
	return p, err
}

// saveSkillPoints 把未分配点数写回数据库。战斗结算的升级奖励只改内存时，allocate 按库里的点数扣会失败，
// 所以 finishBattle 在结算后立即调用，不等离开世界（STAGE-151）。调用方持有 p.mu，同步写完再放锁，同 savePets。
func (s *Service) saveSkillPoints(charID int64, points int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx, `UPDATE characters SET skill_points = $2 WHERE id = $1`, charID, points); err != nil {
		s.opts.Logger.Warn("保存未分配点数失败", "character", charID, "err", err)
	}
}

type addPointRequest struct {
	Stat   *string `json:"stat"`
	Points *int    `json:"points"`
}

// handleAddPoint 把未分配的能力点加到一项能力上（docs/protocol.md 5.21，操作类）。
func (s *Service) handleAddPoint(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in addPointRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	if in.Stat == nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "需要 stat 字段（vital、strength、toughness、dexterity 之一）")
	}
	points := 1
	if in.Points != nil {
		points = *in.Points
	}
	if points < 1 {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "points 要是正整数")
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var addErr error
	if err := l.Operate(func(r *record) {
		// STAGE-189：战斗中也能加点。点数本来就是战斗结算的产物，战斗单位用的是开战时的能力快照，
		// 战后只回写血量、MP，不会拿战斗单位覆盖 Stats，所以加点从下一场起生效，不影响当前这场的平衡。
		addErr = s.allocate(ctx, l.p, r, *in.Stat, points)
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if addErr != nil {
		return nil, addErr
	}
	return map[string]any{}, nil
}

// allocate 把 points 点未分配点数加到 stat 上。调用时要持有 p.mu（在 Live.Operate 里）。
// 先在数据库里原子地扣点、加能力（条件是点数够），成功后才把数据库里的值放进内存并推送 status_changed；
// 任何一步失败内存都不变，所以不会出现只加了一半的情况。
func (s *Service) allocate(ctx context.Context, p *Presence, r *record, stat string, points int) error {
	field := statField(&r.Stats, stat)
	if field == nil {
		return protocol.Errorf(protocol.CodeBadRequest, "stat 只能是 vital、strength、toughness、dexterity 之一")
	}
	if r.HP <= 0 {
		return protocol.Errorf(protocol.CodeCharacterDead, "角色已经死亡，不能加点")
	}
	if r.SkillPoints < points {
		return notEnoughPoints(r.SkillPoints)
	}
	if *field+points > maxStat {
		return protocol.Errorf(protocol.CodeBadRequest, "这项能力最高 %d 点", maxStat)
	}
	// stat 已经由 statField 确认是四个列名之一，拼进 SQL 不会注入。
	var got pointsState
	err := s.db.QueryRow(ctx, `
		UPDATE characters SET `+stat+` = `+stat+` + $2, skill_points = skill_points - $2
		WHERE id = $1 AND skill_points >= $2
		RETURNING vital, strength, toughness, dexterity, skill_points`,
		r.ID, points,
	).Scan(&got.Stats.Vital, &got.Stats.Strength, &got.Stats.Toughness, &got.Stats.Dexterity, &got.SkillPoints)
	if errors.Is(err, pgx.ErrNoRows) {
		// 库里的点数比内存少，只可能是有人绕过服务端直接改了库：先同步内存，再报点数不够。
		fresh, rerr := s.readPoints(ctx, r.ID)
		if errors.Is(rerr, pgx.ErrNoRows) {
			return protocol.Errorf(protocol.CodeCharacterNotFound, "这个栏位没有角色")
		}
		if rerr != nil {
			return rerr
		}
		r.Stats, r.SkillPoints = fresh.Stats, fresh.SkillPoints
		// 内存里的值被纠正了，推送权威状态，客户端不能继续显示已经失效的点数（STAGE-151）。
		p.pushAll("status_changed", map[string]any{"status": statusLive(p, r)})
		return notEnoughPoints(r.SkillPoints)
	}
	if err != nil {
		return err
	}
	r.Stats, r.SkillPoints = got.Stats, got.SkillPoints
	// 角色的变化同时推给玩家连接和 agent 连接，两边看到的一致（推送先于响应到达）。
	p.pushAll("status_changed", map[string]any{"status": statusLive(p, r)})
	return nil
}

// statusLive 是推送给客户端的 status：战斗中 r.HP、r.MP 要到战后才回写，这里取战斗单位的实时值，
// 免得战斗中加点推出的状态把血量显示回开战前（STAGE-189）。调用时要持有 p.mu。
func statusLive(p *Presence, r *record) statusView {
	st := r.status()
	if p.battle != nil {
		if u := p.battle.b.PlayerUnit(); u != nil {
			st.HP, st.MP = u.HP, u.MP
		}
	}
	return st
}

func notEnoughPoints(have int) *protocol.Error {
	return protocol.Errorf(protocol.CodeNotEnoughPoints, "未分配的点数不够，现在有 %d 点", have)
}

// GivePoints 是管理命令（stoneage-admin givepoint 与协议用例的 admin 行）：给角色加 points 点未分配点数，
// 在同一个事务里 NOTIFY，让正在运行的服务端重读并推送 status_changed。不经过协议，直接操作数据库；
// 角色不在线时只写库，等下次进入世界时可见。
func GivePoints(ctx context.Context, db *pgxpool.Pool, channel, character string, points int) error {
	if points < 1 {
		return errors.New("点数至少是 1")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var charID int64
	err = tx.QueryRow(ctx,
		`UPDATE characters SET skill_points = skill_points + $2 WHERE name = $1 RETURNING id`,
		character, points,
	).Scan(&charID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("找不到角色 %s", character)
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, strconv.FormatInt(charID, 10)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Default().Info("管理命令发了未分配点数", "character", character, "points", points)
	return nil
}

// reloadPoints 收到通知后重读角色的能力点，推送 status_changed。角色不在世界中时没有可通知的连接，跳过。
// 读库放在持有角色锁的时候做：加点也是在锁里写库，这样读到的值不会比加点的结果旧。
func (s *Service) reloadPoints(ctx context.Context, payload string) {
	charID, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		s.opts.Logger.Warn("未分配点数通知的 payload 不是角色编号", "payload", payload)
		return
	}
	p := s.presenceOf(charID)
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.leaving {
		return
	}
	fresh, err := s.readPoints(ctx, charID)
	if err != nil {
		s.opts.Logger.Warn("重读角色的能力点失败", "character", charID, "err", err)
		return
	}
	p.rec.Stats, p.rec.SkillPoints = fresh.Stats, fresh.SkillPoints
	p.pushAll("status_changed", map[string]any{"status": statusLive(p, &p.rec)})
}
