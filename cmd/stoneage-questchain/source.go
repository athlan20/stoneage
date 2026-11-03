package main

// 正式运行的数据来源：gamedata 里的任务脚本、任务表、怪物和宠物、道具表，以及白名单和整本支持的判断。

import "github.com/athlan20/stoneage/server/gamedata"

func realEnv() Env {
	return Env{
		EnemyTemplate: func(id int) (int, bool) {
			e, ok := gamedata.EnemyByID(id)
			if !ok {
				return 0, false
			}
			return e.Template, true
		},
		PetCatchable: func(template int) bool {
			p, ok := gamedata.PetByID(template)
			return ok && p.PetFlag > 0
		},
		ItemName: func(id int) string {
			if it, ok := gamedata.ItemByID(id); ok {
				return it.Name
			}
			return ""
		},
		PetName: func(template int) string {
			if p, ok := gamedata.PetByID(template); ok {
				return p.Name
			}
			return ""
		},
	}
}

// realModel 用全部任务脚本和任务表建模型（不论白名单）。
func realModel() *Model {
	return NewModel(gamedata.NpcEventScripts(), gamedata.Quests(), realEnv())
}

// realRules 是正式的放开规则：白名单用 gamedata.NpcEventEnabled（不设开关时就是白名单），整本支持用 NpcEventFileFull。
func realRules() Rules {
	full := gamedata.NpcEventFileFull()
	return Rules{
		Enabled: gamedata.NpcEventEnabled,
		Full:    func(file string) bool { return full[file] },
	}
}
