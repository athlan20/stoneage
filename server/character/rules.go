package character

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/protocol"
	"github.com/athlan20/stoneage/server/wordfilter"
)

// MaxSlots 是每个账号最多能有几个角色。
const MaxSlots = 2

const (
	nameMaxChars  = 16
	statsMax      = 20 // 四项能力点每项的上限，也是总和的上限（_NEW_PLAYER_CF：总和不超过 20 即可）
	elementsMax   = 10 // 四项属性点每项的上限
	elementsTotal = 10 // 属性点总和必须正好是 10
)

// forbiddenInName 是名字里不能出现的词，比较前先折叠大小写和全角半角。
// 出处：gmsv/callfromcli.c 的 lssproto_CreateNewChar_recv。
var forbiddenInName = []string{"华义", "神秘人物", "gm"}

// foldName 把名字折叠成比较用的形式：全角 ASCII 转半角，英文转小写。
// 名字的全服唯一性和敏感词检查都用折叠后的名字，防止用全角或大小写绕过。
func foldName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 0xFF01 && r <= 0xFF5E: // 全角 ！～
			r = r - 0xFF01 + 0x21
		case r == 0x3000:
			r = ' '
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// normalizeName 去掉首尾空白，并检查名字规则。返回保存用的名字，或 INVALID_NAME 错误（message 写明原因）。
func normalizeName(raw string) (string, *protocol.Error) {
	name := strings.TrimFunc(raw, unicode.IsSpace)
	if name == "" {
		return "", protocol.Errorf(protocol.CodeInvalidName, "角色名不能为空")
	}
	if n := utf8.RuneCountInString(name); n > nameMaxChars {
		return "", protocol.Errorf(protocol.CodeInvalidName, "角色名最多 %d 个字符，现在有 %d 个", nameMaxChars, n)
	}
	for _, r := range name {
		if unicode.IsSpace(r) {
			return "", protocol.Errorf(protocol.CodeInvalidName, "角色名不能包含空白")
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Co, r) || r == utf8.RuneError {
			return "", protocol.Errorf(protocol.CodeInvalidName, "角色名不能包含控制字符或不可见字符")
		}
	}
	folded := foldName(name)
	for _, word := range forbiddenInName {
		if strings.Contains(folded, word) {
			return "", protocol.Errorf(protocol.CodeInvalidName, "角色名不能包含「%s」", displayWord(word))
		}
	}
	// STAGE-287：和聊天共用同一份屏蔽词表；名字命中就拒绝，提示里不回显是哪个词（避免被拿来试词）。
	if wordfilter.Current().Contains(name) {
		return "", protocol.Errorf(protocol.CodeInvalidName, "角色名含有不允许的内容")
	}
	return name, nil
}

func displayWord(w string) string {
	if w == "gm" {
		return "GM"
	}
	return w
}

// Appearance 是外形，全部用序号表示（docs/login.md）。
type Appearance struct {
	Body  int `json:"body"`
	Color int `json:"color"`
	Eye   int `json:"eye"`
	Mouth int `json:"mouth"`
}

// Stats 是四项能力点。
type Stats struct {
	Vital     int `json:"vital"`
	Strength  int `json:"strength"`
	Toughness int `json:"toughness"`
	Dexterity int `json:"dexterity"`
}

// Elements 是四项属性点。
type Elements struct {
	Earth int `json:"earth"`
	Water int `json:"water"`
	Fire  int `json:"fire"`
	Wind  int `json:"wind"`
}

func inRange(v, lo, hi int) bool { return v >= lo && v <= hi }

func (a Appearance) validate() *protocol.Error {
	switch {
	case !inRange(a.Body, 0, gamedata.BodyCount-1):
		return protocol.Errorf(protocol.CodeInvalidAppearance, "外形序号要在 0–%d 之间", gamedata.BodyCount-1)
	case !inRange(a.Color, 0, gamedata.ColorCount-1):
		return protocol.Errorf(protocol.CodeInvalidAppearance, "颜色序号要在 0–%d 之间", gamedata.ColorCount-1)
	case !inRange(a.Eye, 0, gamedata.EyeCount-1):
		return protocol.Errorf(protocol.CodeInvalidAppearance, "眼型序号要在 0–%d 之间", gamedata.EyeCount-1)
	case !inRange(a.Mouth, 0, gamedata.MouthCount-1):
		return protocol.Errorf(protocol.CodeInvalidAppearance, "嘴型序号要在 0–%d 之间", gamedata.MouthCount-1)
	}
	return nil
}

func (s Stats) validate() *protocol.Error {
	for _, v := range []int{s.Vital, s.Strength, s.Toughness, s.Dexterity} {
		if !inRange(v, 0, statsMax) {
			return protocol.Errorf(protocol.CodeInvalidStats, "每项能力点要在 0–%d 之间", statsMax)
		}
	}
	if sum := s.Vital + s.Strength + s.Toughness + s.Dexterity; sum > statsMax {
		return protocol.Errorf(protocol.CodeInvalidStats, "能力点总和不能超过 %d，现在是 %d", statsMax, sum)
	}
	return nil
}

func (e Elements) validate() *protocol.Error {
	all := []int{e.Earth, e.Water, e.Fire, e.Wind}
	sum, nonZero := 0, 0
	for _, v := range all {
		if !inRange(v, 0, elementsMax) {
			return protocol.Errorf(protocol.CodeInvalidElements, "每项属性点要在 0–%d 之间", elementsMax)
		}
		sum += v
		if v > 0 {
			nonZero++
		}
	}
	switch {
	case sum != elementsTotal:
		return protocol.Errorf(protocol.CodeInvalidElements, "属性点总和必须正好是 %d，现在是 %d", elementsTotal, sum)
	case nonZero > 2:
		return protocol.Errorf(protocol.CodeInvalidElements, "属性点最多分给 2 个属性")
	case e.Earth > 0 && e.Fire > 0:
		return protocol.Errorf(protocol.CodeInvalidElements, "地和火不能同时有属性点")
	case e.Water > 0 && e.Wind > 0:
		return protocol.Errorf(protocol.CodeInvalidElements, "水和风不能同时有属性点")
	}
	return nil
}

// createRequest 是 create_character 的 data。所有字段都必填：缺字段或类型不对由解码得到 nil，回复 BAD_REQUEST。
type createRequest struct {
	Slot       *int        `json:"slot"`
	Name       *string     `json:"name"`
	Appearance *Appearance `json:"appearance"`
	Stats      *Stats      `json:"stats"`
	Elements   *Elements   `json:"elements"`
	Hometown   *int        `json:"hometown"`
}

func (r *createRequest) complete() bool {
	return r.Slot != nil && r.Name != nil && r.Appearance != nil && r.Stats != nil && r.Elements != nil && r.Hometown != nil
}

// newCharacter 是通过校验、可以入库的新角色。
type newCharacter struct {
	Slot       int
	Name       string
	NameKey    string
	Appearance Appearance
	Stats      Stats
	Elements   Elements
	Hometown   int
}

// validateCreate 按固定顺序检查：栏位 → 名字 → 外形 → 能力点 → 属性点 → 出生地。
// “栏位已被占用”和“名字已被使用”要查数据库，由调用方在之后处理。
func validateCreate(r *createRequest) (*newCharacter, *protocol.Error) {
	if *r.Slot < 0 || *r.Slot >= MaxSlots {
		return nil, protocol.Errorf(protocol.CodeInvalidSlot, "栏位要在 0–%d 之间", MaxSlots-1)
	}
	name, perr := normalizeName(*r.Name)
	if perr != nil {
		return nil, perr
	}
	if perr := r.Appearance.validate(); perr != nil {
		return nil, perr
	}
	if perr := r.Stats.validate(); perr != nil {
		return nil, perr
	}
	if perr := r.Elements.validate(); perr != nil {
		return nil, perr
	}
	if *r.Hometown < 0 || *r.Hometown >= len(gamedata.Hometowns) {
		return nil, protocol.Errorf(protocol.CodeInvalidHometown, "出生地序号要在 0–%d 之间", len(gamedata.Hometowns)-1)
	}
	return &newCharacter{
		Slot: *r.Slot, Name: name, NameKey: foldName(name),
		Appearance: *r.Appearance, Stats: *r.Stats, Elements: *r.Elements, Hometown: *r.Hometown,
	}, nil
}
