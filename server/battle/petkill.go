// 宠物击杀数（STAGE-150，原版 CHAR_KILLPETCOUNT，battle.c 胜利结算）：出过手的宠物，每击败一只怪 +1；
// 胜利时只结算战斗结束时还活着的宠物（被击败、逃跑、没出手的不加）。骑着的宠物随人物出手一起记（合体，ride.go）。

package battle

// markActed 记下宠物出过手（同一只只记一次）。人物、怪物不记；骑着宠物的人物出手时，骑着的宠物算出过手
// （原版骑宠是人宠合体，人出手就是宠出手，battle.c 的骑宠段）。
func (b *Battle) markActed(u *Unit) {
	if u.Kind == KindPlayer && u.ridden() {
		b.markActed(u.mount)
		return
	}
	if u.Kind != KindPet || u.Acted || u.Auto { // 分身的宠物不计击杀（STAGE-162）
		return
	}
	u.Acted = true
	b.petsActed = append(b.petsActed, u)
}

// creditKill 一只怪倒下时，本场出过手的宠物各 +1 击杀数。
func (b *Battle) creditKill() {
	for _, p := range b.petsActed {
		p.Kills++
	}
}

// petKills 汇总胜利时要写回的宠物击杀数：只算出过手、战斗结束时还活着且有击杀的宠物。
func (b *Battle) petKills() []PetKill {
	var out []PetKill
	for _, p := range b.petsActed {
		if p.Alive() && p.Kills > 0 {
			out = append(out, PetKill{Slot: p.PetSlot, Kills: p.Kills})
		}
	}
	return out
}
