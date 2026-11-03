package character

// 真人同场战斗的捕捉（STAGE-275 第三轮，路径 4）：队员捕捉成功，宠物进队员自己的宠物栏位，不进队长的。
// 捕捉成功与否由种子决定：这里挑一个在前几个种子里捉得到的。

import (
	"math/rand/v2"
	"testing"
	"time"
)

func TestPartyMateCaptureGoesToMateNotLeader(t *testing.T) {
	// STAGE-291 批 B：怪物计划改为回合开始定下后，原来的 31–34 走向变了，这几个种子能打到队员捕获。
	for _, seed := range []uint64{38, 40, 44} {
		e, a, b, _ := realPartyEnvWith(t, rand.New(rand.NewPCG(seed, seed*2+1)).IntN, time.Hour, nil)
		a.turnOnEncounter(t)
		b.drainTo(t, "battle_start")
		b.drainTo(t, "battle_turn")

		captured := func() bool { return len(recOf(t, e, "小红").Pets) > 0 }
		pickA := func(string, []int) (string, int) { return "guard", -1 } // 队长防御，只看队员捉
		rounds := 0
		pickB := func(actor string, mons []int) (string, int) {
			if actor == "player" {
				rounds++
			}
			if actor == "player" && rounds > 2 && !captured() && len(mons) > 1 { // 先打两回合压血，捉得到的概率才高
				return "capture", mons[0] // 还有别的怪物时捉，保证战斗不提前结束
			}
			return "attack", mons[0]
		}
		fightTogether(t, a, b, 60, 1, pickA, pickB)
		if !captured() {
			continue // 这个种子没捉到，试下一个
		}

		b.roundtrip(t, "pets", `{}`, "") // 服务端写回在结束时完成，这里同步一下
		if n := len(recOf(t, e, "小红").Pets); n != 1 {
			t.Fatalf("队员应多一只捕到的宠物，实际 %d 只", n)
		}
		if n := len(recOf(t, e, "勇者小明").Pets); n != 0 {
			t.Fatalf("捕到的宠物不该进队长的宠物栏，队长实际 %d 只", n)
		}
		if n := len(dbPets(t, e, "小红")); n != 1 {
			t.Fatalf("队员的宠物应写库，库里 %d 只", n)
		}
		if n := len(dbPets(t, e, "勇者小明")); n != 0 {
			t.Fatalf("队长的宠物库里应为 0 只，实际 %d 只", n)
		}
		return
	}
	t.Fatal("测试种子里队员一次都没捉到，换一组种子")
}
