package character

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// 不连数据库：变幻的精灵的变身状态（STAGE-203）。Presence 没有连接，推送都是空操作。
func newMetamoPresence() *Presence {
	return &Presence{members: map[protocol.Role]*Live{}, done: make(chan struct{})}
}

// 变身状态只在 status 里出现：没有变身时省略 metamo，有变身时带宠物名、图像编号和毫秒级的到期时间。
func TestMetamoStatusView(t *testing.T) {
	r := &record{Name: "测试"}
	if r.status().Metamo != nil {
		t.Fatal("没有变身时 status.metamo 应为 nil")
	}
	if b, _ := json.Marshal(r.status()); strings.Contains(string(b), `"metamo"`) {
		t.Fatal("没有变身时 status 里不该有 metamo 字段")
	}
	p := newMetamoPresence()
	now := time.Unix(1000, 0)
	p.startMetamo(&p.rec, "乌力", 100250, now, time.Minute)
	defer stopMetamo(&p.rec)
	v := p.rec.status().Metamo
	if v == nil || v.Pet != "乌力" || v.Image != 100250 || v.Until != now.Add(time.Minute).UnixMilli() {
		t.Fatalf("变身中的 status.metamo = %+v", v)
	}
	if b, _ := json.Marshal(p.rec.status()); !strings.Contains(string(b), `"metamo":{"pet":"乌力","image":100250,"until":1060000}`) {
		t.Fatalf("变身中的 status 没有带 metamo：%s", b)
	}
}

// 到期定时器清掉变身（不用等真正的一分钟：时长只要很短就行）。
func TestMetamoExpiresByTimer(t *testing.T) {
	p := newMetamoPresence()
	p.mu.Lock()
	p.startMetamo(&p.rec, "乌力", 100250, time.Now(), 20*time.Millisecond)
	p.mu.Unlock()
	deadline := time.Now().Add(2 * time.Second)
	for {
		p.mu.Lock()
		gone := p.rec.metamo == nil
		p.mu.Unlock()
		if gone {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("到期后变身应该清掉")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// 旧的定时器不能清掉新的变身：到期回调只认自己那一次的 metamoState。
func TestMetamoOldTimerCannotClearNew(t *testing.T) {
	p := newMetamoPresence()
	p.mu.Lock()
	p.startMetamo(&p.rec, "乌力", 100250, time.Now(), time.Hour)
	old := p.rec.metamo
	p.startMetamo(&p.rec, "布比", 100251, time.Now(), time.Hour)
	p.mu.Unlock()
	p.expireMetamo(old)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rec.metamo == nil || p.rec.metamo.Pet != "布比" {
		t.Fatalf("旧定时器清掉了新的变身：%+v", p.rec.metamo)
	}
	stopMetamo(&p.rec)
}

// 身体装备（装备格 1）被卸下或换掉才算变身失效，其他格子的变化不算（原版 CHAR_BODY）。
func TestBodyArmourChanged(t *testing.T) {
	armour := []itemRec{{Slot: 1, ItemID: 1000, Count: 1}}
	cases := []struct {
		name          string
		before, after []itemRec
		want          bool
	}{
		{"卸下铠甲", armour, nil, true},
		{"换成别的铠甲", armour, []itemRec{{Slot: 1, ItemID: 1001, Count: 1}}, true},
		{"铠甲不变", armour, armour, false},
		{"身上本来没有铠甲", nil, []itemRec{{Slot: 1, ItemID: 1000, Count: 1}}, false},
		{"卸下的是手上的斧头", []itemRec{{Slot: 2, ItemID: 0, Count: 1}, armour[0]}, armour, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bodyArmourChanged(c.before, c.after); got != c.want {
				t.Errorf("bodyArmourChanged = %v，应为 %v", got, c.want)
			}
		})
	}
}

// 远距离武器：弓、回旋标、投掷斧、投掷石；斧头、铠甲不算。
func TestIsRangedWeapon(t *testing.T) {
	for _, id := range []int{400, 500, 600, 700} {
		if !isRangedWeapon(id) {
			t.Errorf("物品 %d 应是远距离武器", id)
		}
	}
	for _, id := range []int{0, 1000} {
		if isRangedWeapon(id) {
			t.Errorf("物品 %d 不是远距离武器", id)
		}
	}
}

// 变身中穿远距离武器被拒；没有变身、穿的是近战武器、或手上本来就是它，都放行。
func TestCheckMetamoWear(t *testing.T) {
	bow := []itemRec{{Slot: 2, ItemID: 400, Count: 1}}
	axe := []itemRec{{Slot: 2, ItemID: 0, Count: 1}}
	cases := []struct {
		name    string
		metamo  bool
		before  []itemRec
		after   []itemRec
		wantErr bool
	}{
		{"变身中穿回旋标", true, nil, []itemRec{{Slot: 2, ItemID: 500, Count: 1}}, true},
		{"变身中换上弓", true, axe, bow, true},
		{"变身中穿斧头", true, nil, axe, false},
		{"没有变身穿弓", false, nil, bow, false},
		{"手上本来就是弓", true, bow, bow, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &record{}
			if c.metamo {
				r.metamo = &metamoState{}
			}
			perr := checkMetamoWear(r, c.before, c.after)
			if (perr != nil) != c.wantErr {
				t.Fatalf("checkMetamoWear = %v，wantErr=%v", perr, c.wantErr)
			}
			if perr != nil && perr.Code != protocol.CodeRequirementNotMet {
				t.Errorf("错误码 %s，应为 REQUIREMENT_NOT_MET", perr.Code)
			}
		})
	}
}

func TestHasRidingPet(t *testing.T) {
	if hasRidingPet([]petRec{{Slot: 0, State: petStateStandby}}) {
		t.Error("待命的宠物不是骑乘")
	}
	if !hasRidingPet([]petRec{{Slot: 0, State: petStateStandby}, {Slot: 1, State: petStateRide}}) {
		t.Error("骑着的宠物应该算骑乘中")
	}
}
