// STAGE-241：批量发放的“发什么”和“怎么发一个人”。规则全部复用 server/character 里的单发函数
// （GiveItem / GivePet / GiveMagic / GivePoints），这里只做参数校验、说明文字和逐人调用。
package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/athlan20/stoneage/server/character"
	"github.com/athlan20/stoneage/server/gamedata"
)

// grantSpec 是一次批量发放要发的东西。Qty 的含义随类型不同：物品是个数，宠物是等级，
// 能力点是点数，精灵固定为 1。能力点没有编号，ID 为 0。
type grantSpec struct {
	Kind string
	ID   int
	Qty  int
}

// describe 是预演和汇总里的中文说明，如“物品 小斧头（编号 0）×3”。
func (s grantSpec) describe() string {
	switch s.Kind {
	case "item":
		def, _ := gamedata.ItemByID(s.ID)
		return fmt.Sprintf("物品 %s（编号 %d）×%d", def.Name, s.ID, s.Qty)
	case "pet":
		def, _ := gamedata.PetByID(s.ID)
		return fmt.Sprintf("宠物 %s（编号 %d，Lv %d）", def.Name, s.ID, s.Qty)
	case "magic":
		def, _ := gamedata.MagicByID(s.ID)
		return fmt.Sprintf("精灵 %s（编号 %d）", def.Name, s.ID)
	case "gold":
		return fmt.Sprintf("石币 %d（超过持有上限的部分不发）", s.Qty)
	default:
		return fmt.Sprintf("能力点 %d 点（未分配，玩家自己用 addpoint 加到能力上）", s.Qty)
	}
}

// validateGrantSpec 在碰数据库之前检查编号和数量，错误一次报完，不让每个角色都报同一个错。
// 规则和 GiveItem / GivePet / GiveMagic / GivePoints 里的检查一致。
func validateGrantSpec(s grantSpec) error {
	switch s.Kind {
	case "item":
		if _, ok := gamedata.ItemByID(s.ID); !ok {
			return fmt.Errorf("物品编号 %d 不存在", s.ID)
		}
		if s.Qty < 1 {
			return errors.New("数量至少是 1")
		}
	case "pet":
		tmpl, ok := gamedata.PetByID(s.ID)
		if !ok {
			return fmt.Errorf("宠物编号 %d 不存在", s.ID)
		}
		if tmpl.PetFlag <= 0 {
			return fmt.Errorf("编号 %d（%s）不能当宠物（PETFLG 为 0）", s.ID, tmpl.Name)
		}
		if s.Qty < 1 || s.Qty > character.MaxPetLevel {
			return fmt.Errorf("宠物等级要在 1–%d 之间", character.MaxPetLevel)
		}
		if tmpl.LimitLevel > 0 && s.Qty > tmpl.LimitLevel {
			return fmt.Errorf("%s 的等级上限是 %d", tmpl.Name, tmpl.LimitLevel)
		}
	case "magic":
		if _, ok := gamedata.MagicByID(s.ID); !ok {
			return fmt.Errorf("精灵编号 %d 不在魔法表里", s.ID)
		}
	case "point":
		if s.Qty < 1 {
			return errors.New("点数至少是 1")
		}
	case "gold":
		if s.Qty < 1 {
			return errors.New("石币数量至少是 1")
		}
	default:
		return fmt.Errorf("不认识的物资类型 %q", s.Kind)
	}
	return nil
}

// applyGrant 给一个角色发一次。每个单发函数自己开事务、写库并 NOTIFY，角色不在线时只写库。
// rnd 只给宠物的随机资质用。返回的说明只有石币封顶时才有（写进发放记录）。
func applyGrant(ctx context.Context, pool *pgxpool.Pool, s grantSpec, name string, rnd func(n int) int) (string, error) {
	switch s.Kind {
	case "item":
		return "", character.GiveItem(ctx, pool, character.DefaultItemNotifyChannel, name, s.ID, s.Qty)
	case "pet":
		_, err := character.GivePet(ctx, pool, character.DefaultPetNotifyChannel, name, s.ID, s.Qty, rnd)
		return "", err
	case "magic":
		return "", character.GiveMagic(ctx, pool, character.DefaultMagicNotifyChannel, name, s.ID)
	case "gold":
		g, err := character.GiveGold(ctx, pool, character.DefaultGoldNotifyChannel, name, s.Qty)
		if err != nil || !g.Capped() {
			return "", err
		}
		return fmt.Sprintf("石币封顶：应发 %d，实发 %d（持有上限 %d）", g.Requested, g.Granted, g.Cap), nil
	default:
		return "", character.GivePoints(ctx, pool, character.DefaultPointNotifyChannel, name, s.Qty)
	}
}

// newGrantRand 返回宠物资质用的随机数，每次批量发放一个，不用全局随机源。
func newGrantRand(seed int64) func(n int) int {
	rng := rand.New(rand.NewPCG(uint64(seed), 0))
	return rng.IntN
}
