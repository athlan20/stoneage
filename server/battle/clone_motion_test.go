package battle

import (
	"testing"

	"github.com/athlan20/stoneage/server/gamedata"
)

// STAGE-167 / STAGE-162：共享人物初始化后，队长和分身事件仍带真实武器动画身份。
func TestCloneAttackKeepsWeaponMotion(t *testing.T) {
	for itemType, want := range map[int]string{
		gamedata.ItemTypeBow:       motionBow,
		gamedata.ItemTypeBoomerang: motionBoomerang,
		gamedata.ItemTypeSpear:     motionSpear,
	} {
		t.Run(want, func(t *testing.T) {
			leader := testSide()
			leader.WeaponType = itemType
			b := New("clone_motion", "野外", leader, monsters(5), rollRnd(9999))
			clone := cloneSide("分身")
			clone.WeaponType = itemType
			b.AddClones([]*SideInput{clone})
			if got := b.units[0].WeaponMotion; got != want {
				t.Fatalf("队长武器 = %q, want %q", got, want)
			}
			b.planAuto()
			events := b.autoAct(b.units[1])
			if len(events) == 0 || events[0].Weapon != want {
				t.Fatalf("分身事件丢失真实武器 %q: %+v", want, events)
			}
		})
	}
}
