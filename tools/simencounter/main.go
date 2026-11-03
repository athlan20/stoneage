// STAGE-40 起用例校准的离线模拟（docs/server.md“遇敌用例”）：按用例固定种子重放野外战斗，
// 打印每条推送的确切内容。用法：go run ./tools/simencounter <用例名> [额外腕力] [场数]。
// 只在开发时手工使用，不进版本库（build/ 已 gitignore）。
package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"

	"github.com/athlan20/stoneage/server/battle"
	"github.com/athlan20/stoneage/server/casetest"
	"github.com/athlan20/stoneage/server/gamedata"
)

func dump(label string, v any) {
	b, _ := json.Marshal(v)
	fmt.Println(label, string(b))
}

func play(name string, addStrength int, battles int) {
	fmt.Printf("== %s ==\n", name)
	rnd := rand.New(rand.NewPCG(casetest.SeedForCase(name), 43)).IntN
	hp, mp := 44, 100 // 血量在战斗间延续；最大血量按加点后的能力算
	for bi := 1; bi <= battles; bi++ {
		area := gamedata.EncounterAreaAt(100, 650, 500)
		side := &battle.SideInput{
			Name: "勇者小明", Level: 1, HP: hp, MaxHP: gamedata.MaxHP(8, 4+addStrength, 4, 4), MP: mp, MaxMP: 100,
			Attack:  gamedata.Attack(8, 4+addStrength, 4, 4),
			Defense: gamedata.Defense(8, 4+addStrength, 4, 4),
			Agility: gamedata.Agility(4),
			Vital:   8, Strength: 4 + addStrength, Toughness: 4, Dexterity: 4,
			Elem: battle.Elements{Earth: 60, Water: 40, Fire: 0, Wind: 0}, // 建角 6 地 4 水
		}
		b, ok := battle.BuildWild(fmt.Sprintf("b_%d", bi), "萨伊那斯", side, &battle.WildSpec{Area: area}, rnd)
		if !ok {
			fmt.Println("no battle")
			return
		}
		dump("battle_start", b.StartView())
		for turn := 1; ; turn++ {
			dump("battle_turn", b.TurnView(0, 0))
			// 找还站着的怪物里号最小的一只
			target := -1
			for no := 15; no < 20; no++ {
				if u := b.Unit(no); u != nil && u.Alive() {
					target = no
					break
				}
			}
			if target == -1 {
				fmt.Println("no target but battle not over?!")
				return
			}
			next, perr := b.Submit(turn, "player", "attack", battle.Cmd{Target: target})
			if perr != nil {
				fmt.Println("submit error:", perr)
				return
			}
			fmt.Println("battle_command ok next:", next)
			dump("battle_progress", b.ProgressDone())
			round, end := b.Settle()
			dump("battle_round", round)
			if end != nil {
				dump("battle_end", end)
				if u := b.PlayerUnit(); u != nil { // 战斗后的血量带到下一场（applyResult 的行为）
					hp, mp = u.HP, u.MP
					if hp < 1 {
						hp = 1 // lose 复活血 1 并回村；这里只关心数值
					}
				}
				break
			}
		}
	}
}

func main() {
	name := os.Args[1]
	strength, _ := strconv.Atoi(os.Args[2])
	battles := 1
	if len(os.Args) > 3 {
		battles, _ = strconv.Atoi(os.Args[3])
	}
	play(name, strength, battles)
}
