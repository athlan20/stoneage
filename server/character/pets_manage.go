// 宠物管理（STAGE-32，docs/protocol.md 5.22–5.24，docs/ui.md 宠物页 B）：pet_state 设出战、待命、休息，
// pet_rename 改名，pet_drop 放生。三者都是操作类，走同一条路径：加行锁读出宠物，检查（规则在 pets_rules.go），
// 在一个事务里写库，提交后才更新内存并推送 pets_changed，所以失败时数据库和内存都不变。

package character

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

type petStateRequest struct {
	Slot  *int    `json:"slot"`
	State *string `json:"state"`
}

type petRenameRequest struct {
	Slot *int    `json:"slot"`
	Name *string `json:"name"`
}

type petDropRequest struct {
	Slot    *int    `json:"slot"`
	Confirm *string `json:"confirm"`
}

// petChange 是一次宠物改动。事务里读出的宠物（已加行锁）和角色 ID 交进来，检查并写库，返回改后的完整列表。
// 不需要改（如已经是那个状态）时返回 changed 为 false，调用方不提交、不推送。
type petChange func(tx pgx.Tx, charID int64, pets []petRec) (next []petRec, changed bool, err error)

// handlePetState 是 pet_state（docs/protocol.md 5.22）：设为出战、待命或休息。
func (s *Service) handlePetState(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in petStateRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	slot, err := petSlotArg(in.Slot)
	if err != nil {
		return nil, err
	}
	state, ok := petStateByName(in.State)
	if !ok {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "state 只能是 battle、standby、rest 之一")
	}
	return s.operatePet(ctx, sess, func(tx pgx.Tx, charID int64, pets []petRec) ([]petRec, bool, error) {
		changes, err := planPetState(pets, slot, state)
		if err != nil || len(changes) == 0 {
			return nil, false, err
		}
		next := append([]petRec(nil), pets...)
		for i := range next {
			if st, ok := changes[next[i].Slot]; ok {
				next[i].State = st
			}
		}
		for sl, st := range changes {
			if _, err := tx.Exec(ctx, `UPDATE character_pets SET state = $3 WHERE character_id = $1 AND slot = $2`, charID, sl, st); err != nil {
				return nil, false, err
			}
		}
		return next, true, nil
	})
}

// handlePetRename 是 pet_rename（docs/protocol.md 5.23）：改名，名字为空就恢复种族名。
func (s *Service) handlePetRename(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in petRenameRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	slot, err := petSlotArg(in.Slot)
	if err != nil {
		return nil, err
	}
	raw := ""
	if in.Name != nil {
		raw = *in.Name
	}
	name, err := normalizePetName(raw)
	if err != nil {
		return nil, err
	}
	return s.operatePet(ctx, sess, func(tx pgx.Tx, charID int64, pets []petRec) ([]petRec, bool, error) {
		target := findPet(pets, slot)
		if target == nil {
			return nil, false, protocol.Errorf(protocol.CodeNoPet, "%d 号栏位没有宠物", slot)
		}
		tmpl, ok := gamedata.PetByID(target.TemplateID)
		if !ok {
			return nil, false, fmt.Errorf("宠物模板 %d 不存在", target.TemplateID)
		}
		newName := petName(name, tmpl.Name)
		if newName == target.Name {
			return nil, false, nil
		}
		if _, err := tx.Exec(ctx, `UPDATE character_pets SET name = $3 WHERE character_id = $1 AND slot = $2`, charID, slot, newName); err != nil {
			return nil, false, err
		}
		next := append([]petRec(nil), pets...)
		findPet(next, slot).Name = newName
		return next, true, nil
	})
}

// handlePetDrop 是 pet_drop（docs/protocol.md 5.24）：放生，宠物从栏位删除，不可恢复。
func (s *Service) handlePetDrop(ctx context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in petDropRequest
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	slot, err := petSlotArg(in.Slot)
	if err != nil {
		return nil, err
	}
	if in.Confirm == nil || *in.Confirm != "yes" {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "放生要确认：confirm 必须是 \"yes\"")
	}
	return s.operatePet(ctx, sess, func(tx pgx.Tx, charID int64, pets []petRec) ([]petRec, bool, error) {
		target := findPet(pets, slot)
		if target == nil {
			return nil, false, protocol.Errorf(protocol.CodeNoPet, "%d 号栏位没有宠物", slot)
		}
		if target.State == petStateRide {
			return nil, false, protocol.Errorf(protocol.CodePetRiding, "骑乘中的宠物不能放生，先下骑")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM character_pets WHERE character_id = $1 AND slot = $2`, charID, slot); err != nil {
			return nil, false, err
		}
		next := make([]petRec, 0, len(pets))
		for _, p := range pets {
			if p.Slot != slot {
				next = append(next, p)
			}
		}
		return next, true, nil
	})
}

// operatePet 是三个宠物操作共用的入口：确认连接在世界中且是操控方（Live.Operate），然后执行 change。
// 成功返回空对象；改动被规则拒绝时返回对应的协议错误，数据不变。
func (s *Service) operatePet(ctx context.Context, sess *gateway.Session, change petChange) (any, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	var changeErr error
	if err := l.Operate(func(r *record) {
		changeErr = s.changePets(ctx, l.p, r, change)
	}); err != nil {
		return nil, asProtocolError(err)
	}
	if changeErr != nil {
		return nil, changeErr
	}
	return map[string]any{}, nil
}

// changePets 在一个事务里改角色的宠物：加行锁读出宠物，交给 change；提交成功后才更新内存并推送 pets_changed，
// 给世界中的每一条连接（玩家和 agent）都推。调用时要持有角色的锁（在 Live.Operate 里）。
func (s *Service) changePets(ctx context.Context, p *Presence, r *record, change petChange) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	pets, err := readPets(ctx, tx, r.ID, true)
	if err != nil {
		return err
	}
	next, changed, err := change(tx, r.ID, pets)
	if err != nil || !changed {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	r.Pets = next
	p.pushAll("pets_changed", r.petsView())
	return nil
}

// FaintPet 是测试用的管理命令（casetest 的 faint_pet 行）：把角色栏位上宠物的血量设为 0，相当于战斗中倒下。
// 战斗还没做，只能这样造出倒下的宠物，用来验收 PET_FAINTED；和 givepet 一样 NOTIFY，让运行中的服务端重读并推送。
func FaintPet(ctx context.Context, db *pgxpool.Pool, channel, character string, slot int) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var charID int64
	err = tx.QueryRow(ctx, `UPDATE character_pets SET hp = 0 FROM characters
		WHERE characters.id = character_pets.character_id AND characters.name = $1 AND character_pets.slot = $2
		RETURNING character_pets.character_id`, character, slot).Scan(&charID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s 的 %d 号栏位没有宠物", character, slot)
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, strconv.FormatInt(charID, 10)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
