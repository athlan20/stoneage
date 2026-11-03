// 精工的实例覆盖（STAGE-298，docs/inlay.md 2.4）：镶过东西的物品，自己记一份镶嵌状态，存在
// character_items.inlay（jsonb）里。没镶过的物品 Inlay 为 nil，所有读取都回到物品模板。
// effectiveTemplate 把模板和实例覆盖合成一份“实际”模板：装备加成、装备精灵、背包显示、使用物品都用它。
// 怎么镶、台词在 inlay_rules.go，写库在 item_inlay.go。

package character

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
)

// inlayGearMark 是可镶装备的类别码标记：原版 ITEM_TYPECODE 含子串 INSLAY 的是武器或防具（docs/inlay.md 1.2）。
const inlayGearMark = "INSLAY"

// inlayCodes 是一件装备的三个镶嵌格，nil 表示空格。
type inlayCodes [3]*string

// inlayState 是镶嵌实例覆盖。Bonus 存的是材料累加量，不是总值；最终值 = 模板 + Bonus。
type inlayState struct {
	Codes    inlayCodes  `json:"codes"`
	Bonus    inlayBonus  `json:"bonus"`
	Magic    *inlayMagic `json:"magic,omitempty"`    // 只在材料带魔法时有，覆盖模板的魔法
	UseFunc  *string     `json:"use_func,omitempty"` // 被材料覆盖后的使用函数与参数，只在发生过覆盖后有
	Argument *string     `json:"argument,omitempty"`
	Effect   string      `json:"effect,omitempty"` // 镶过之后重写的效果文字
}

// inlayBonus 是材料累加的数值（攻、防、敏、HP、MP、运气、伤、吸）。
type inlayBonus struct {
	Attack      int `json:"attack"`
	Defence     int `json:"defence"`
	Agility     int `json:"agility"`
	HP          int `json:"hp"`
	MP          int `json:"mp"`
	Luck        int `json:"luck"`
	OtherDamage int `json:"other_damage"`
	OtherDefc   int `json:"other_defc"`
}

// inlayMagic 是覆盖后的精灵编号与精灵 MP。
type inlayMagic struct {
	ID int `json:"id"`
	MP int `json:"mp"`
}

// inlayView 是背包格子里的 inlay 对象（docs/protocol.md 5.17）：可镶装备带 free 与 codes，材料只有 role。
type inlayView struct {
	Role  string    `json:"role"`
	Free  *int      `json:"free,omitempty"`
	Codes []*string `json:"codes,omitempty"`
}

// isInlayGear 判断模板是不是可镶装备（类别码含 INSLAY）。
func isInlayGear(def gamedata.ItemTemplate) bool {
	return strings.Contains(def.InlayCode, inlayGearMark)
}

// inlaySuitable 判断物品能不能精工（当装备或当材料）：类别码非空且不是 NULL（原版 PETSKILL_ITEM_inslay 的第一条检查）。
func inlaySuitable(def gamedata.ItemTemplate) bool {
	return def.InlayCode != "" && def.InlayCode != "NULL"
}

// parseInlayCodes 解析物品模板的初始镶嵌码（itemset.txt 第 5 列，`码1|码2|码3`，空位写 NULL）。
func parseInlayCodes(s string) (out inlayCodes) {
	parts := strings.Split(s, "|")
	for i := 0; i < len(out) && i < len(parts); i++ {
		if c := strings.TrimSpace(parts[i]); c != "" && c != "NULL" {
			out[i] = &c
		}
	}
	return out
}

// codesOf 返回物品现在的三个镶嵌格：镶过的看实例，没镶过的看模板的初始镶嵌码。
func codesOf(it itemRec, def gamedata.ItemTemplate) inlayCodes {
	if it.Inlay != nil {
		return it.Inlay.Codes
	}
	return parseInlayCodes(def.InlayInit)
}

// freeInlaySlots 数空的镶嵌格。
func freeInlaySlots(codes inlayCodes) int {
	n := 0
	for _, c := range codes {
		if c == nil {
			n++
		}
	}
	return n
}

// effectiveTemplate 把模板和物品的实例覆盖合成实际的模板；没有覆盖时原样返回。
func effectiveTemplate(def gamedata.ItemTemplate, st *inlayState) gamedata.ItemTemplate {
	if st == nil {
		return def
	}
	b := st.Bonus
	def.Attack += b.Attack
	def.Defence += b.Defence
	def.Agility += b.Agility
	def.Luck += b.Luck
	def.HP += b.HP
	def.MP += b.MP
	def.OtherDamage += b.OtherDamage
	def.OtherDefc += b.OtherDefc
	if st.Magic != nil {
		def.MagicID, def.MagicMP = st.Magic.ID, st.Magic.MP
	}
	if st.UseFunc != nil {
		def.UseFunc = *st.UseFunc
	}
	if st.Argument != nil {
		def.Argument = *st.Argument
	}
	if st.Effect != "" {
		def.Effect = st.Effect
	}
	return def
}

// inlayEffectText 按实际模板的总值重写效果文字（docs/inlay.md 1.4 第 7 条）：
// 攻、防、敏、HP、MP、伤、吸，非负带 +；有精灵时接在后面的是魔法名。运气不显示。
func inlayEffectText(def gamedata.ItemTemplate) string {
	s := fmt.Sprintf("攻%+d 防%+d 敏%+d HP%+d MP%+d 伤%+d 吸%+d", def.Attack, def.Defence, def.Agility, def.HP, def.MP, def.OtherDamage, def.OtherDefc)
	if def.MagicID >= 0 {
		if t, ok := gamedata.MagicByID(def.MagicID); ok {
			s += " " + t.Name
		}
	}
	return s
}

// itemInlayView 是背包格子的 inlay 对象；类别码为空的物品返回 nil（不带 inlay）。
func itemInlayView(it itemRec, def gamedata.ItemTemplate) *inlayView {
	if !inlaySuitable(def) {
		return nil
	}
	if !isInlayGear(def) {
		return &inlayView{Role: "material"}
	}
	codes := codesOf(it, def)
	free := freeInlaySlots(codes)
	return &inlayView{Role: "gear", Free: &free, Codes: codes[:]}
}

// inlayJSON 把镶嵌状态编成 jsonb 参数；没镶过（nil）是 NULL。
func inlayJSON(st *inlayState) any {
	if st == nil {
		return nil
	}
	b, _ := json.Marshal(st) // 结构里只有普通字段和指针，不会失败
	return string(b)
}

// decodeInlay 解析 character_items.inlay；NULL 返回 nil。
func decodeInlay(raw []byte) (*inlayState, error) {
	if raw == nil {
		return nil, nil
	}
	var st inlayState
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// sameInlay 比较两件物品的镶嵌状态（改动检测用，见 changedSlots）。
func sameInlay(a, b *inlayState) bool {
	return reflect.DeepEqual(a, b)
}
