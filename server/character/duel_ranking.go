package character

// STAGE-308：决斗强者表（docs/pk.md 3.5、5.3，协议 5.75）。全服角色（含离线）按决斗点数从高到低排，
// 同分先达到的排前（duel_point_at），再按名字。duel_ranking 请求和强者表 NPC（npc_duelranking.go）共用这里。

import (
	"context"
	"fmt"

	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// duelRankPageSize 是强者表每页的人数（原版 NPC_DUELRANKING_WINDOWLINENUM）。
const duelRankPageSize = 10

// duelRankRow 是排行里的一个人。Self 为 true 的是请求者自己。
type duelRankRow struct {
	Rank      int    `json:"rank"`
	Name      string `json:"name"`
	Level     int    `json:"level"`
	DuelPoint int    `json:"duel_point"`
	Self      bool   `json:"self"`
}

// duelRanking 是一页排行（协议 5.75 的 data，强者表窗口的 ranking 字段也是它）。
type duelRanking struct {
	Top   []duelRankRow `json:"top"`
	Page  int           `json:"page"`
	Pages int           `json:"pages"`
	Me    duelRankMe    `json:"me"`
}

// duelRankMe 是请求者自己的名次和决斗点数。
type duelRankMe struct {
	Rank      int `json:"rank"`
	DuelPoint int `json:"duel_point"`
}

// duelRankPages 是总页数：至少 1 页，哪怕没有人（不会发生，角色至少有自己）。
func duelRankPages(total int) int {
	return max(1, (total+duelRankPageSize-1)/duelRankPageSize)
}

// duelRankPageOf 是名次所在的页（1 起）。
func duelRankPageOf(rank int) int {
	return max(1, (rank-1)/duelRankPageSize+1)
}

// duelRankLoad 读取一页排行和请求者自己的名次。page 已经检查过是 1 以上的整数。
// 排序和名次用同一套条件（决斗点数降序、达到时间升序、名字升序），保证 me.rank 与 top 里的名次一致。
func (s *Service) duelRankLoad(ctx context.Context, charID int64, page int) (*duelRanking, error) {
	var dp int
	if err := s.db.QueryRow(ctx, `SELECT duel_point FROM characters WHERE id = $1`, charID).Scan(&dp); err != nil {
		return nil, fmt.Errorf("读取自己的决斗点数: %w", err)
	}
	var total, ahead int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM characters`).Scan(&total); err != nil {
		return nil, fmt.Errorf("统计角色数: %w", err)
	}
	if err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM characters c, (SELECT duel_point, duel_point_at, name FROM characters WHERE id = $1) m
		 WHERE c.duel_point > m.duel_point
		    OR (c.duel_point = m.duel_point AND (c.duel_point_at < m.duel_point_at
		        OR (c.duel_point_at = m.duel_point_at AND c.name < m.name)))`,
		charID).Scan(&ahead); err != nil {
		return nil, fmt.Errorf("算自己的名次: %w", err)
	}
	out := &duelRanking{Top: []duelRankRow{}, Page: page, Pages: duelRankPages(total), Me: duelRankMe{Rank: ahead + 1, DuelPoint: dp}}
	if page > out.Pages {
		return out, nil // 超过最后一页：top 为空
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, name, level, duel_point FROM characters
		ORDER BY duel_point DESC, duel_point_at ASC, name ASC
		LIMIT $1 OFFSET $2`, duelRankPageSize, (page-1)*duelRankPageSize)
	if err != nil {
		return nil, fmt.Errorf("读取排行: %w", err)
	}
	defer rows.Close()
	for i := 0; rows.Next(); i++ {
		var id int64
		var row duelRankRow
		if err := rows.Scan(&id, &row.Name, &row.Level, &row.DuelPoint); err != nil {
			return nil, fmt.Errorf("读取排行: %w", err)
		}
		row.Rank = (page-1)*duelRankPageSize + i + 1
		row.Self = id == charID
		out.Top = append(out.Top, row)
	}
	return out, rows.Err()
}

// handleDuelRanking 是 duel_ranking（查看类，世界中，协议 5.75）：page 可省略（默认 1）。
func (s *Service) handleDuelRanking(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Page *int `json:"page"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	page := 1
	if in.Page != nil {
		if *in.Page < 1 {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "page 要是 1 以上的整数")
		}
		page = *in.Page
	}
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var charID int64
	if err := l.With(func(r *record) { charID = r.ID }); err != nil {
		return nil, asProtocolError(err)
	}
	out, err := s.duelRankLoad(ctx, charID, page)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeInternal, "读取强者表失败：%v", err)
	}
	return out, nil
}
