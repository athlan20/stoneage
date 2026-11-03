package battle

import "testing"

// STAGE-291 批 B：剧本名可以带 "@种子"，替换剧本自带的种子，其余不变。
func TestLookupSeedOverride(t *testing.T) {
	base, err := Lookup("beaten")
	if err != nil {
		t.Fatal(err)
	}
	sc, err := Lookup("beaten@40")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Seed != 40 || base.Seed == 40 || sc.FleeTurn != base.FleeTurn || len(sc.Monsters) != len(base.Monsters) {
		t.Fatalf("种子 %d（原 %d），其余应与 beaten 相同：%+v", sc.Seed, base.Seed, sc)
	}
	again, _ := Lookup("beaten")
	if again.Seed != base.Seed {
		t.Fatal("带种子的查找不能改掉剧本表里的原种子")
	}
	for _, bad := range []string{"beaten@x", "nope@3", "@3"} {
		if _, err := Lookup(bad); err == nil {
			t.Fatalf("%q 应当报错", bad)
		}
	}
}
