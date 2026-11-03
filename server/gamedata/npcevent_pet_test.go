package gamedata

// 任务脚本宠物写法的数据（STAGE-197）：PetAsk 的匹配、GetPet 编号的查表（怪物和宠物模板）。

import (
	"strings"
	"testing"
)

// 宠物项按模板和等级匹配：编号要相同，等级用比较符比（比的是宠物等级）。
func TestPetAskMatch(t *testing.T) {
	gt := PetAsk{Cmp: ">", Level: 0, Template: 560, Count: 4}
	if !gt.Match(560, 1) || gt.Match(561, 1) {
		t.Fatalf("PET>0-560 应该只匹配 560 号、等级大于 0 的宠物")
	}
	lt := PetAsk{Cmp: "<", Level: 30, Template: 720}
	if !lt.Match(720, 29) || lt.Match(720, 30) {
		t.Fatalf("PET<30-720 应该是等级小于 30")
	}
	eq := PetAsk{Cmp: "=", Level: 77, Template: 12}
	if !eq.Match(12, 77) || eq.Match(12, 76) {
		t.Fatalf("PET=77-12 应该是等级等于 77")
	}
}

// GetPet 的怪物要在敌人表里，它的宠物模板也要在宠物表里；都查得到时返回空串。
func TestCheckPetGifts(t *testing.T) {
	if why := checkPetGifts([]int{44, 45}); why != "" {
		t.Fatalf("44、45 号怪物应该都能查到，实际 %s", why)
	}
	if why := checkPetGifts([]int{44, 999999}); !strings.Contains(why, "不在敌人表里") {
		t.Fatalf("查不到的怪物应该说不在敌人表里，实际 %q", why)
	}
}

// 加载时的检查只标记带 GetPet 的块：查不到的标 Unsupported，原来已经不支持的块保持原因，其余块不动。
func TestMarkPetGifts(t *testing.T) {
	list := []EventScript{{ID: 1, Blocks: []EventBlock{
		{Type: "ACCEPT", GetPet: []int{999999}},
		{Type: "ACCEPT", GetPet: []int{999998}, Unsupported: "原来的原因"},
		{Type: "ACCEPT", GetPet: []int{44}},
		{Type: "MESSAGE"},
	}}}
	markPetGifts(list)
	b := list[0].Blocks
	if !strings.Contains(b[0].Unsupported, "不在敌人表里") {
		t.Fatalf("查不到的怪物应标不支持，实际 %q", b[0].Unsupported)
	}
	if b[1].Unsupported != "原来的原因" {
		t.Fatalf("已经不支持的块原因不应被覆盖，实际 %q", b[1].Unsupported)
	}
	if b[2].Unsupported != "" || b[3].Unsupported != "" {
		t.Fatalf("查得到的 GetPet 和没有 GetPet 的块应该保持支持，实际 %q %q", b[2].Unsupported, b[3].Unsupported)
	}
}

// 真实数据里的 GetPet（seimu/event/event02_1 的 GetPet:93、GetPet:341）加载后仍然支持。
func TestRealGetPetBlockSupported(t *testing.T) {
	for id := 1; id <= 5000; id++ {
		s, ok := NpcEventOf(id)
		if !ok || s.File != "seimu/event/event02_1" {
			continue
		}
		for _, b := range s.Blocks {
			if len(b.GetPet) > 0 && b.Unsupported != "" {
				t.Fatalf("seimu/event02_1 的 GetPet 应该支持，实际 %s", b.Unsupported)
			}
		}
		return
	}
	t.Fatal("找不到 seimu/event/event02_1")
}
