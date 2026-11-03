// STAGE-340：龙王的守护（18544，ITEM_useNoenemy）在世界中使用：扣 1 个、推提示、一段时间内不遇敌，到期恢复。

package character

import (
	"context"
	"math/rand/v2"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

const noEnemyItem = 18544

// noEnemyEnv 起一个可以推进时钟的环境：角色已进世界并站在野外（place 5，遇敌区域内），
// 随机源固定，遇敌间隔很短。clock 是服务端的当前时间（Unix 秒）。
func noEnemyEnv(t *testing.T, clock *atomic.Int64) (*env, *client) {
	t.Helper()
	e := newEnv(t, func(s *Service) {
		s.opts.Now = func() time.Time { return time.Unix(clock.Load(), 0) }
		s.opts.EncounterInterval = 40 * time.Millisecond
		s.opts.WildRnd = rand.New(rand.NewPCG(23, 47)).IntN
	})
	c := e.loggedIn("alice")
	c.ok("create_character", createJSON(0, "勇者小明", 0))
	clearStartPets(t, e, "勇者小明")
	c.ok("enter_world", `{"slot":0}`)
	c.warpTo(t, 5)
	return e, c
}

// giveNoEnemyItems 在背包 slot 格放 count 个龙王的守护（直接写库和内存，同其他道具用例）。
func giveNoEnemyItems(t *testing.T, e *env, slot, count int) {
	t.Helper()
	withRec(t, e, "alice", func(r *record) {
		if _, err := e.chars.db.Exec(context.Background(),
			`INSERT INTO character_items (character_id, slot, item_id, count) VALUES ($1, $2, $3, $4)`,
			r.ID, slot, noEnemyItem, count); err != nil {
			t.Fatal(err)
		}
		r.Items = []itemRec{{Slot: slot, ItemID: noEnemyItem, Count: count}}
	})
}

// noEnemyCount 返回背包 slot 格里龙王的守护的数量，没有则为 0。
func noEnemyCount(t *testing.T, e *env, slot int) int {
	t.Helper()
	n := 0
	withRec(t, e, "alice", func(r *record) {
		if it := findItem(r.Items, slot); it != nil {
			n = it.Count
		}
	})
	return n
}

// useNoEnemyItem 在世界中使用 slot 格的龙王的守护，返回响应和随之而来的推送。
func useNoEnemyItem(c *client, slot int) (map[string]any, []map[string]any) {
	return c.callp("item_use", `{"slot":`+strconv.Itoa(slot)+`}`)
}

// hasNotice 报告推送里是否有“你感受到周边的杀气消失了。”这条提示。
func hasNotice(pushes []map[string]any, text string) bool {
	for _, p := range pushes {
		if p["type"] != "notice" {
			continue
		}
		if d, _ := p["data"].(map[string]any); d["text"] == text {
			return true
		}
	}
	return false
}

// TestNoEnemyItemStopsEncounter 是主路径：使用后扣 1 个、推提示，遇敌开关开不起来（NO_ENEMY_HERE），
// 时间到了之后恢复遇敌。
func TestNoEnemyItemStopsEncounter(t *testing.T) {
	var clock atomic.Int64
	base := time.Now().Unix()
	clock.Store(base)
	e, c := noEnemyEnv(t, &clock)

	const slot = 5
	giveNoEnemyItems(t, e, slot, 2)
	m, pushes := useNoEnemyItem(c, slot)
	if m["ok"] != true {
		t.Fatalf("使用龙王的守护应成功: %v", m)
	}
	if !hasNotice(pushes, noEnemyText) {
		t.Fatalf("使用后应推送提示“%s”，推送：%v", noEnemyText, pushes)
	}
	if n := noEnemyCount(t, e, slot); n != 1 {
		t.Fatalf("使用后数量应为 1，实际 %d", n)
	}

	// 守护期间打不开遇敌开关，什么都不推，开关保持关闭。
	c.expectError(t, "encounter", `{"set":"on"}`, "NO_ENEMY_HERE")
	c.expectError(t, "encounter_start", `{"source_floor":100,"area_id":"`+area21Key+`","mode":"once"}`, "NO_ENEMY_HERE")

	// 守护到期后恢复遇敌：开关能打开，马上进战斗。
	clock.Store(base + int64((noEnemyDuration + time.Second).Seconds()))
	c.turnOnEncounter(t)
}

// TestNoEnemyItemRenewsDuration 检查持续时间：到期前仍不遇敌；重复使用是重新计时，不叠加。
func TestNoEnemyItemRenewsDuration(t *testing.T) {
	var clock atomic.Int64
	base := time.Now().Unix()
	clock.Store(base)
	e, c := noEnemyEnv(t, &clock)

	const slot = 5
	giveNoEnemyItems(t, e, slot, 2)
	useNoEnemyItem(c, slot)

	// 过了 4 分钟再用一个：期限从这时起算 5 分钟，原来那一次的剩余时间不叠加。
	clock.Store(base + int64((4 * time.Minute).Seconds()))
	if m, _ := useNoEnemyItem(c, slot); m["ok"] != true {
		t.Fatalf("第二次使用应成功: %v", m)
	}
	// 原定的到期时刻（5 分钟）已过，但重新计时后还在守护中。
	clock.Store(base + int64((noEnemyDuration + time.Second).Seconds()))
	c.expectError(t, "encounter", `{"set":"on"}`, "NO_ENEMY_HERE")
	// 从第二次使用起满 5 分钟后恢复。
	clock.Store(base + int64((4*time.Minute + noEnemyDuration + time.Second).Seconds()))
	c.turnOnEncounter(t)
}

// TestNoEnemyItemBlockedInBattle 战斗中不能用：回 IN_BATTLE，道具不扣。
func TestNoEnemyItemBlockedInBattle(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().Unix())
	e, c := noEnemyEnv(t, &clock)

	const slot = 5
	giveNoEnemyItems(t, e, slot, 1)
	if err := e.chars.StartBattle("勇者小明", "win"); err != nil {
		t.Fatal(err)
	}
	c.drainTo(t, "battle_turn")
	c.wantErr(t, "item_use", `{"slot":`+strconv.Itoa(slot)+`}`, "IN_BATTLE")
	if n := noEnemyCount(t, e, slot); n != 1 {
		t.Fatalf("战斗中用不掉，数量应仍为 1，实际 %d", n)
	}
}

// TestNoEnemyItemRejects 数量为 0 回 EMPTY_SLOT；选别的目标回 INVALID_TARGET，道具不扣。
func TestNoEnemyItemRejects(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Now().Unix())
	e, c := noEnemyEnv(t, &clock)

	const slot = 5
	c.wantErr(t, "item_use", `{"slot":`+strconv.Itoa(slot)+`}`, protocol.CodeEmptySlot)

	giveNoEnemyItems(t, e, slot, 1)
	c.wantErr(t, "item_use", `{"slot":`+strconv.Itoa(slot)+`,"target":1}`, protocol.CodeInvalidTarget)
	if n := noEnemyCount(t, e, slot); n != 1 {
		t.Fatalf("选错目标不应扣道具，数量应为 1，实际 %d", n)
	}
	useNoEnemyItem(c, slot)
	c.wantErr(t, "item_use", `{"slot":`+strconv.Itoa(slot)+`}`, protocol.CodeEmptySlot)
}
