// 捕捉的资质筛选（STAGE-264 第 4 条，docs/protocol.md 10.5 的 capture.keep）。
// 抓成功之后，服务端在入栏位之前按四项资质的档位（tier，0 为满，正常 −4…0，
// 与宠物页 5.19 的 quality 同一口径）判断：每项设了阈值的都要 ≥ 阈值（AND），
// 没设的项不看，四项都没设就完全不筛（包括档位低于 −4 的怪）。

package battle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/athlan20/stoneage/server/protocol"
)

// CaptureKeep 是抓宠的资质阈值，每项是“最差不低于”的档位（−4…0）；nil 表示这项不限。
type CaptureKeep struct {
	Vital     *int `json:"vital,omitempty"`
	Strength  *int `json:"strength,omitempty"`
	Toughness *int `json:"toughness,omitempty"`
	Dexterity *int `json:"dexterity,omitempty"`
}

// Quality 是宠物的一项资质：素质点 points 与档位 tier（points − (模板资质 + 2)）。
type Quality struct {
	Stat   string `json:"stat"`
	Points int    `json:"points"`
	Tier   int    `json:"tier"`
}

// Discarded 是被资质筛掉的宠物（捕捉事件 10.7 的 discarded）：没有入库，也不占栏位。
type Discarded struct {
	Name    string    `json:"name"`
	Level   int       `json:"level"`
	Quality []Quality `json:"quality"`
	Reason  string    `json:"reason"`
}

// keepLabels 是四项资质的显示名，顺序与 Quality 的顺序一致。
var keepLabels = [4]string{"体力", "腕力", "耐久力", "速度"}

// ParseCaptureKeep 解析 keep 对象（JSON 原文）。null 或没有带等同于不限；键只认四个，
// 值必须是 −4…0 的整数，否则回复 BAD_REQUEST。
func ParseCaptureKeep(raw json.RawMessage) (*CaptureKeep, *protocol.Error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var k CaptureKeep
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&k); err != nil {
		return nil, protocol.Errorf(protocol.CodeBadRequest, "keep 只认 vital、strength、toughness、dexterity 四项，值是 -4 到 0 的整数")
	}
	for _, v := range k.values() {
		if v != nil && (*v < -4 || *v > 0) {
			return nil, protocol.Errorf(protocol.CodeBadRequest, "keep 的档位要在 -4 到 0 之间")
		}
	}
	return &k, nil
}

// values 按 Quality 的顺序返回四个阈值指针。
func (k *CaptureKeep) values() [4]*int {
	return [4]*int{k.Vital, k.Strength, k.Toughness, k.Dexterity}
}

// Judge 判断四项资质是否合格：q 按 vital、strength、toughness、dexterity 的顺序给出。
// 不合格时 reason 说明第一个不达标的项，如“体力 −3 < −2”。nil 接收者或四项都不限时合格。
func (k *CaptureKeep) Judge(q []Quality) (reason string, ok bool) {
	if k == nil || len(q) != 4 {
		return "", true
	}
	for i, lo := range k.values() {
		if lo == nil || q[i].Tier >= *lo {
			continue
		}
		return fmt.Sprintf("%s %s < %s", keepLabels[i], signed(q[i].Tier), signed(*lo)), false
	}
	return "", true
}

// signed 按协议文本的写法输出档位：负数用 “−”（U+2212），0 直接写 0。
func signed(n int) string {
	return strings.ReplaceAll(fmt.Sprint(n), "-", "−")
}
