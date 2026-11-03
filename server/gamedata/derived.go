package gamedata

// 角色页的衍生数值（STAGE-23，docs/ui.md “角色页”）。出处：gmsv/char/char.c 的 CHAR_initcharWorkInt。
// 原版服务端把能力按“点数×100”存，系数里的 0.01 在这里已经抵消，所以下面直接用点数；
// 结果按原版服务端的做法截成整数（小数部分舍去）。整数运算避免浮点误差：x×0.1 写成 x/10。

// Attack 是攻击力：腕力 + 耐久力×0.1 + 体力×0.1 + 速度×0.05。
func Attack(vital, strength, toughness, dexterity int) int {
	return strength + (toughness*10+vital*10+dexterity*5)/100
}

// Defense 是防御力：耐久力 + 腕力×0.1 + 体力×0.1 + 速度×0.05。
func Defense(vital, strength, toughness, dexterity int) int {
	return toughness + (strength*10+vital*10+dexterity*5)/100
}

// Agility 是敏捷：等于速度（原版服务端 CHAR_WORKQUICK 取 CHAR_WORKFIXDEX，即速度×0.01）。
func Agility(dexterity int) int {
	return dexterity
}
