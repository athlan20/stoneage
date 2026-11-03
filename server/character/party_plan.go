// 分身队的自动战斗方案（STAGE-195，docs/protocol.md 5.57）：组队时 party_clone 可以带上客户端「保存并应用」的方案，
// 服务端只认战斗用到的字段（其余的出招延迟、逃跑、停止保护、换宠、播放设置一律忽略），收下后挂在队伍上，
// 之后每场战斗里分身按它出招。方案版本不是 1、或字段的值不认识，不报错，只回复 ignored，分身走默认策略。

package character

import (
	"encoding/json"

	"github.com/athlan20/stoneage/server/battle"
)

// 响应里 auto_profile 的取值（协议 5.57）。
const (
	autoProfileApplied = "applied"
	autoProfileIgnored = "ignored"
	autoProfileVersion = 1 // 客户端 AutoProfile 的版本（client/src/app/auto_battle/mod.rs 的 VERSION）
)

// wireProfile 是客户端方案里服务端用得上的那部分，字段名逐字对照客户端的 serde 写法。
// 解析前先用 defaultWireProfile 填上客户端 AutoProfile::default() 的值，缺的字段就取默认。
type wireProfile struct {
	Version int    `json:"version"`
	Target  string `json:"target"`
	Player  struct {
		Act       string `json:"act"`
		MagicSlot int    `json:"magic_slot"`
		MagicName string `json:"magic_name"`
	} `json:"player"`
	Pet struct {
		Act       string `json:"act"`
		SkillSlot int    `json:"skill_slot"`
		SkillName string `json:"skill_name"`
	} `json:"pet"`
	First struct {
		Enabled bool `json:"enabled"`
		Player  struct {
			Act       string `json:"act"`
			MagicSlot int    `json:"magic_slot"`
			MagicName string `json:"magic_name"`
		} `json:"player"`
		Pet struct {
			Act       string `json:"act"`
			SkillSlot int    `json:"skill_slot"`
			SkillName string `json:"skill_name"`
		} `json:"pet"`
	} `json:"first"`
	Recover struct {
		PlayerHeal    bool `json:"player_heal"`
		PlayerHealPct int  `json:"player_heal_pct"`
		PetHeal       bool `json:"pet_heal"`
		PetHealPct    int  `json:"pet_heal_pct"`
		PlayerMP      bool `json:"player_mp"`
		PlayerMPPct   int  `json:"player_mp_pct"`
	} `json:"recover"`
	Capture struct {
		Enabled  bool            `json:"enabled"`
		LevelMin int             `json:"level_min"`
		LevelMax int             `json:"level_max"`
		Names    []string        `json:"names"`
		Keep     json.RawMessage `json:"keep"` // STAGE-264：资质筛选，类型或取值不对整个方案按 ignored 处理
	} `json:"capture"`
}

// defaultWireProfile 是客户端 AutoProfile::default() 里服务端用得上的值。
func defaultWireProfile() wireProfile {
	var w wireProfile
	w.Target = "combo"
	w.Player.Act, w.Pet.Act, w.Pet.SkillSlot = "attack", "attack", 1
	w.First.Player.Act, w.First.Pet.Act, w.First.Pet.SkillSlot = "attack", "attack", 1
	w.Recover.PlayerHeal, w.Recover.PlayerHealPct = true, 30
	w.Recover.PetHeal, w.Recover.PetHealPct = true, 30
	w.Recover.PlayerMPPct = 20
	w.Capture.LevelMin = 1
	return w
}

// parseAutoPlan 解析 party_clone 带来的方案，第二个返回值是响应里的 applied / ignored。
// 版本不是 1、JSON 解析失败、类型不对，或者动作、目标取了不认识的值，都是 ignored（方案为 nil）。
func parseAutoPlan(raw json.RawMessage) (*battle.AutoPlan, string) {
	w := defaultWireProfile()
	if err := json.Unmarshal(raw, &w); err != nil || w.Version != autoProfileVersion {
		return nil, autoProfileIgnored
	}
	plan, ok := w.plan()
	if !ok {
		return nil, autoProfileIgnored
	}
	return plan, autoProfileApplied
}

// plan 把解析好的方案换成战斗用的结构；取值不认识的返回 false。
func (w wireProfile) plan() (*battle.AutoPlan, bool) {
	if w.Target != "combo" && w.Target != "random" {
		return nil, false
	}
	player, ok1 := playerAct(w.Player.Act, w.Player.MagicSlot, w.Player.MagicName)
	firstPlayer, ok2 := playerAct(w.First.Player.Act, w.First.Player.MagicSlot, w.First.Player.MagicName)
	pet, ok3 := petAct(w.Pet.Act, w.Pet.SkillSlot, w.Pet.SkillName)
	firstPet, ok4 := petAct(w.First.Pet.Act, w.First.Pet.SkillSlot, w.First.Pet.SkillName)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return nil, false
	}
	keep, kerr := battle.ParseCaptureKeep(w.Capture.Keep)
	if kerr != nil {
		return nil, false
	}
	return &battle.AutoPlan{
		Target: w.Target,
		Player: player,
		Pet:    pet,
		First: battle.PlanFirst{
			Enabled: w.First.Enabled,
			Player:  firstPlayer,
			Pet:     firstPet,
		},
		Recover: battle.PlanRecover{
			PlayerHeal:    w.Recover.PlayerHeal,
			PlayerHealPct: w.Recover.PlayerHealPct,
			PetHeal:       w.Recover.PetHeal,
			PetHealPct:    w.Recover.PetHealPct,
			PlayerMP:      w.Recover.PlayerMP,
			PlayerMPPct:   w.Recover.PlayerMPPct,
		},
		Capture: battle.PlanCapture{
			Enabled:  w.Capture.Enabled,
			LevelMin: w.Capture.LevelMin,
			LevelMax: w.Capture.LevelMax,
			Names:    w.Capture.Names,
			Keep:     keep,
		},
	}, true
}

// playerAct 是人物的常规动作（attack / magic / guard），magic 的栏位是 0–4。
func playerAct(act string, slot int, name string) (battle.PlanAct, bool) {
	switch act {
	case "attack", "magic", "guard":
		return battle.PlanAct{Act: act, Slot: slot, Name: name}, true
	}
	return battle.PlanAct{}, false
}

// petAct 是战宠的常规动作（attack / skill / guard），skill 的栏位是 1–7。
func petAct(act string, slot int, name string) (battle.PlanAct, bool) {
	switch act {
	case "attack", "skill", "guard":
		return battle.PlanAct{Act: act, Slot: slot, Name: name}, true
	}
	return battle.PlanAct{}, false
}
