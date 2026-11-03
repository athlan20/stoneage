package gamedata

import (
	"fmt"
	"time"
)

// ExitCell preserves every original trigger -> destination pair, including conflicts.
type ExitCell struct{ X, Y, ToX, ToY int }
type MapExit struct {
	ID                         string
	ToFloor                    int
	Kind, TimeGate, Parameters string
	Cells                      []ExitCell
	Conflict, OneWay           bool
}
type TriggerCell struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type ExitView struct {
	ID                string        `json:"id"`
	ToFloor           int           `json:"to_floor"`
	ToName            string        `json:"to_name"`
	TriggerCells      []TriggerCell `json:"trigger_cells"`
	TimeGate          string        `json:"time_gate"`
	Available         bool          `json:"available"`
	UnavailableReason *string       `json:"unavailable_reason"`
	OneWay            bool          `json:"one_way"`
}

// ExitsOf returns independent copies in target-floor / representative x,y order.
func ExitsOf(floor int) []MapExit {
	result := make([]MapExit, len(mapExits[floor]))
	copy(result, mapExits[floor])
	for i := range result {
		result[i].Cells = append([]ExitCell(nil), result[i].Cells...)
	}
	return result
}
func exitTimeActive(gate string, hour int) bool {
	switch gate {
	case "all":
		return true
	case "morning":
		return hour > 700 && hour <= 930
	case "noon":
		return hour > 930 || hour <= 200
	case "night":
		return hour > 300 && hour <= 700
	}
	return false
}
func (e MapExit) UnavailableAt(now time.Time) string {
	if e.Conflict {
		return "出口数据冲突，暂不可用"
	}
	if !exitTimeActive(e.TimeGate, lsHour(now)) {
		return map[string]string{"morning": "早晨开放", "noon": "白天开放", "night": "夜晚开放"}[e.TimeGate] + "，当前游戏时段不可用"
	}
	return ""
}
func (e MapExit) View(now time.Time) ExitView {
	name, _ := MapName(e.ToFloor)
	if name == "" {
		name = fmt.Sprintf("地图 #%d", e.ToFloor)
	}
	v := ExitView{ID: e.ID, ToFloor: e.ToFloor, ToName: name, TimeGate: e.TimeGate, OneWay: e.OneWay, TriggerCells: []TriggerCell{}}
	for _, c := range e.Cells {
		p := TriggerCell{c.X, c.Y}
		if len(v.TriggerCells) == 0 || v.TriggerCells[len(v.TriggerCells)-1] != p {
			v.TriggerCells = append(v.TriggerCells, p)
		}
	}
	reason := e.UnavailableAt(now)
	v.Available = reason == ""
	if reason != "" {
		v.UnavailableReason = &reason
	}
	return v
}

// NextExitChange returns the next actual availability change, using the same whole-second clock as lsHour.
func NextExitChange(exits []MapExit, now time.Time) *int64 {
	timed := false
	for _, e := range exits {
		if !e.Conflict && e.TimeGate != "all" {
			timed = true
			break
		}
	}
	if !timed {
		return nil
	}
	for seconds := int64(1); seconds <= 5400; seconds++ {
		candidate := time.Unix(now.Unix()+seconds, 0)
		for _, e := range exits {
			if e.Conflict || e.TimeGate == "all" {
				continue
			}
			if exitTimeActive(e.TimeGate, lsHour(now)) != exitTimeActive(e.TimeGate, lsHour(candidate)) {
				ms := candidate.UnixMilli()
				return &ms
			}
		}
	}
	return nil
}
