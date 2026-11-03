// 真人同场战斗的数据模型（STAGE-275）：座位编号、宠物号、各自的 needs 与指令归属。
package battle

import "testing"

// partyFixture 开一场 n 个真人（含队长）的战斗，怪物用 win 剧本的两只，固定种子。
func partyFixture(t *testing.T, n int) *Battle {
	t.Helper()
	sc, err := Lookup("win")
	if err != nil {
		t.Fatal(err)
	}
	rnd := sc.NewRand()
	monsters, err := sc.makeMonsters(rnd)
	if err != nil {
		t.Fatal(err)
	}
	var mates []PartyMate
	for i := 1; i < n; i++ {
		mates = append(mates, PartyMate{No: i, Side: testSide()})
	}
	b, err := NewParty("p_test", "野外", testSide(), mates, monsters, rnd)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPartyTwoHumansSeats(t *testing.T) {
	b := partyFixture(t, 2)
	if !b.IsParty() || b.Kind != "party" || b.You != 0 {
		t.Fatalf("kind=%s party=%v you=%d", b.Kind, b.IsParty(), b.You)
	}
	for _, no := range []int{0, 1, 5, 6} {
		if b.Unit(no) == nil {
			t.Fatalf("单位 %d 应该在场", no)
		}
	}
	if b.Unit(1).Kind != KindPlayer || b.Unit(6).Kind != KindPet {
		t.Fatalf("队员 1 应为人物、宠物 6 应为宠物，实际 %s / %s", b.Unit(1).Kind, b.Unit(6).Kind)
	}
	if !b.isMateUnit(b.Unit(1)) || !b.isMateUnit(b.Unit(6)) || b.isMateUnit(b.Unit(0)) || b.isMateUnit(b.Unit(5)) {
		t.Fatal("只有队员的人物和宠物是队员单位")
	}
	if got := b.needsOf(1); len(got) != 2 || got[0] != "player" || got[1] != "pet" {
		t.Fatalf("队员 needs = %v", got)
	}
	if got := b.Needs(); len(got) != 2 {
		t.Fatalf("队长 needs = %v", got)
	}
}

func TestPartyFiveHumansSeats(t *testing.T) {
	b := partyFixture(t, 5)
	for no := 0; no < 5; no++ {
		if b.Unit(no) == nil || b.Unit(no+5) == nil {
			t.Fatalf("座位 %d 或宠物 %d 缺失", no, no+5)
		}
		if no > 0 && b.Unit(no).Side() != 0 {
			t.Fatalf("队员 %d 不在己方", no)
		}
	}
	if got := b.inBattleHumans(); len(got) != 5 {
		t.Fatalf("在场真人 = %v", got)
	}
	// 第 6 个真人没有座位：号码超出 1–4 的队员不接受。
	sc, _ := Lookup("win")
	monsters, _ := sc.makeMonsters(sc.NewRand())
	if _, err := NewParty("p_bad", "野外", testSide(), []PartyMate{{No: 5, Side: testSide()}}, monsters, sc.NewRand()); err == nil {
		t.Fatal("队员号 5 应该被拒绝")
	}
}

func TestPartyCommandOwnership(t *testing.T) {
	b := partyFixture(t, 2)
	mateCmd := &Cmd{Actor: "player", Action: "guard"}
	b.mateCmds[1] = mateCmd
	if b.commandOf(b.Unit(1)) != mateCmd {
		t.Fatal("队员人物的指令应该从 mateCmds 取")
	}
	leaderCmd := &Cmd{Actor: "player", Action: "guard"}
	b.cmds["player"] = leaderCmd
	if b.commandOf(b.Unit(0)) != leaderCmd {
		t.Fatal("队长的指令仍从 cmds 取")
	}
	if b.matesSubmitted() {
		t.Fatal("队员的宠物还没交，不能算交齐")
	}
	b.mateCmds[6] = &Cmd{Actor: "pet", Action: "attack"}
	if !b.matesSubmitted() {
		t.Fatal("队员的人物和宠物都交了，应该算交齐")
	}
}

func TestSingleBattleHasNoParty(t *testing.T) {
	b, err := Build("single", testSide(), "win")
	if err != nil {
		t.Fatal(err)
	}
	if b.IsParty() || b.mates[1] != nil {
		t.Fatal("单人战斗不应该带真人同场的结构")
	}
}
