package gamedata

// STAGE-234：宠物对话（玛蕾等，原版 data/npc/pettalk/*.arg，pet_event.c 的 PET_Talkfunc）的数据与选块。
// server/gamedata/pettalk.json 由 tools/pettalk.py 生成（不要手改），这里只读它、按原版规则挑出一句台词。
// 个体等级上限的读写和对话的执行在 server/character/pettalk.go。

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed pettalk.json
var pettalkJSON []byte

// PetTalkCond 是宠物的一个等级条件（PET:LV 的 = < >，比的是宠物自己的等级）。
type PetTalkCond struct {
	Cmp   string `json:"cmp"`
	Level int    `json:"level"`
}

// PetTalkFloor 是玩家所在楼层条件（FLOOR:n；Not 为真时是 FLOOR:!n）。
type PetTalkFloor struct {
	ID  int  `json:"id"`
	Not bool `json:"not"`
}

// PetTalkBlock 是一个对话块。Tag 是 TALKRUN、EVENTRUN 或 BOTH；Limit 为 0 表示不改上限（LimitLevel 动作）；
// Items 是 AddItem 动作要给的道具编号（每个一件）。Pet 里的条件全部要满足（&）。
type PetTalkBlock struct {
	Name  string        `json:"name"`
	Tag   string        `json:"tag"`
	Pet   []PetTalkCond `json:"pet"`
	Floor *PetTalkFloor `json:"floor"`
	Limit int           `json:"limit"`
	Items []int         `json:"items"`
	Text  string        `json:"text"`
}

// PetTalkTable 是一只宠物模板的对话：Other 是没有块命中时随机说的台词（原版 OTHER 的 TalkMsg，逗号分句）。
type PetTalkTable struct {
	Source   string         `json:"source"`
	Template int            `json:"template"`
	Other    []string       `json:"other"`
	Blocks   []PetTalkBlock `json:"blocks"`
}

var pettalkTables = mustLoadPetTalk()

func mustLoadPetTalk() map[int]PetTalkTable {
	var list []PetTalkTable
	if err := json.Unmarshal(pettalkJSON, &list); err != nil {
		panic(fmt.Sprintf("pettalk.json 读不出来：%v", err))
	}
	out := make(map[int]PetTalkTable, len(list))
	for _, t := range list {
		out[t.Template] = t
	}
	return out
}

// PetTalkOf 返回宠物模板的对话表；没有对话的宠物返回 false。
func PetTalkOf(template int) (PetTalkTable, bool) {
	t, ok := pettalkTables[template]
	return t, ok
}

// PetTalkPick 是原版 PET_Talkfunc 的选块：pet 是宠物等级，limit 是它的个体等级上限（有效值，见 character 层），
// floor 是玩家所在楼层。等级 >= 上限时只看 EVENTRUN 块，否则只看 TALKRUN 块，BOTH 两种都看（原版 Type 的判断）。
// 满足条件的块里随机挑一个；没有就从 Other 里随机挑一句（此时 ok 为真、Block 为 nil）。
// 挑出的块返回值在 Block，台词在 Text。rnd(n) 返回 [0, n) 的整数。
func PetTalkPick(t PetTalkTable, pet, limit, floor int, rnd func(n int) int) (block *PetTalkBlock, text string, ok bool) {
	want := "TALKRUN"
	if pet >= limit {
		want = "EVENTRUN"
	}
	var cands []int
	for i := range t.Blocks {
		b := &t.Blocks[i]
		if b.Tag != want && b.Tag != "BOTH" {
			continue
		}
		if b.Floor != nil && (floor == b.Floor.ID) == b.Floor.Not {
			continue
		}
		if !petTalkPetMatch(b.Pet, pet) {
			continue
		}
		cands = append(cands, i)
	}
	if len(cands) > 0 {
		b := &t.Blocks[cands[rnd(len(cands))]]
		return b, b.Text, true
	}
	if len(t.Other) == 0 {
		return nil, "", false
	}
	return nil, t.Other[rnd(len(t.Other))], true
}

// petTalkPetMatch 检查宠物等级条件（PET:LV 的 & 组合全部成立）。
func petTalkPetMatch(conds []PetTalkCond, pet int) bool {
	for _, c := range conds {
		if !cmpInt(pet, c.Cmp, c.Level) {
			return false
		}
	}
	return true
}
