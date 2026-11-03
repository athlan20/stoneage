package character

// 真人同场战斗的胜利结算（STAGE-275 第三轮，路径 3、5）：经验只给出手击杀的人，金钱按人写回，
// 每人收到的 battle_end 只含本人份额；队员的宠物血量按人写回自己的宠物；战后两人都不再在战斗中。

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"
)

// 两人同场打赢：人物与宠物都攻击，每人只拿到自己的 battle_end 份额；队员宠物的血量写回他自己的宠物。
func TestPartyWinRewardsAndPetWriteBackPerMember(t *testing.T) {
	rnd := rand.New(rand.NewPCG(21, 43)).IntN
	pre := func(e *env) {
		if _, err := GivePet(context.Background(), e.chars.db, e.chars.opts.PetNotifyChannel, "小红", 1, 1,
			func(n int) int { return n / 2 }); err != nil {
			t.Fatal(err)
		}
	}
	e, a, b, _ := realPartyEnvWith(t, rnd, time.Hour, pre)
	a.turnOnEncounter(t)
	b.drainTo(t, "battle_start")
	b.drainTo(t, "battle_turn")
	goldA, goldB := recOf(t, e, "勇者小明").Gold, recOf(t, e, "小红").Gold
	petHP := recOf(t, e, "小红").Pets[0].HP

	endA, endB := fightTogether(t, a, b, 60, 1, pickAttack, pickAttack)
	if endA["result"] != "win" || endB["result"] != "win" {
		t.Fatalf("两人都应赢（结果 %v / %v）", endA["result"], endB["result"])
	}
	ra, rb := endA["rewards"].(map[string]any), endB["rewards"].(map[string]any)

	// 经验：拿到正的人物经验的都是出手击杀的人，名字对应本人。STAGE-291 批 B：怪物计划改为回合开始定下后，
	// 固定种子下两只怪各由一人打倒（两人各得一份），所以不再假定只有一人拿到。
	killers := map[string]bool{}
	for who, r := range map[string]map[string]any{"勇者小明": ra, "小红": rb} {
		list, _ := r["exp"].([]any) // 没有经验的一项不带 exp（omitempty）
		for _, x := range list {
			ex := x.(map[string]any)
			if ex["who"] == "player" && ex["gain"].(float64) > 0 {
				if ex["name"] != who {
					t.Fatalf("%s 的人物经验记在了 %v 名下", who, ex["name"])
				}
				killers[who] = true
			}
		}
	}
	if len(killers) == 0 {
		t.Fatalf("两只怪都打倒了，至少应有一人拿到人物经验")
	}

	// 金钱：两人都在场且存活时份额相同，并且写回了各自的记录。
	if num(ra["gold"]) != num(rb["gold"]) { // 没有金钱的一项不带 gold
		t.Fatalf("金钱份额应相同: 队长 %v 队员 %v", ra["gold"], rb["gold"])
	}
	gainA := recOf(t, e, "勇者小明").Gold - goldA
	gainB := recOf(t, e, "小红").Gold - goldB
	if int64(num(ra["gold"])) != gainA || int64(num(rb["gold"])) != gainB {
		t.Fatalf("金钱应按份额写回各自的人物: 份额 %v/%v，实际增加 %d/%d", ra["gold"], rb["gold"], gainA, gainB)
	}

	// 宠物：队员的宠物被打伤，血量写回他自己的宠物（库里与内存一致）；队长没有宠物。
	b.roundtrip(t, "pets", `{}`, "") // 等服务端写回完成（持有 b 的锁，写回在 finish 里）
	if hp := recOf(t, e, "小红").Pets[0].HP; hp >= petHP {
		t.Fatalf("队员的宠物应被打伤（血量 %d -> %d）", petHP, hp)
	}
	if dbHP := dbPets(t, e, "小红")[0].HP; dbHP != recOf(t, e, "小红").Pets[0].HP {
		t.Fatalf("队员宠物的血量应写库: 库里 %d，内存 %d", dbHP, recOf(t, e, "小红").Pets[0].HP)
	}
	if n := len(recOf(t, e, "勇者小明").Pets); n != 0 {
		t.Fatalf("队长不该多出宠物，实际 %d 只", n)
	}

	// 战后：两人都已离开战斗（战斗标记清掉），这里的遇敌间隔很长，不会自动开下一场。
	if partyOfNameOrNil(e, "勇者小明") != nil || partyOfNameOrNil(e, "小红") != nil {
		t.Fatal("战斗结束后两人都不应还在战斗中")
	}
}

// num 取数字字段（缺省为 0）。
func num(v any) float64 {
	f, _ := v.(float64)
	return f
}
