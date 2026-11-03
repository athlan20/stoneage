package character

import (
	"context"
	"strings"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
	"github.com/athlan20/stoneage/server/protocol"
)

// mapsPartMaps / mapsPartNodes 是 maps 请求 parts 数组里认的两个取值。
const (
	mapsPartMaps  = "maps"
	mapsPartNodes = "nodes"
)

// handleMaps 返回快捷传送目录（docs/protocol.md 5.42，查看类，STAGE-161）。
//
// 一次发三样东西：
//   - `catalog_version`：目录版本号，层级或落点变了它才变，始终返回；
//   - `maps`：平面表，每张正式地图一条，按 floor 升序，用来按名字搜索和补全 warp 参数；
//   - `nodes`：包含树，客户端按 parent_id 逐级选择（地图节点也可以有子节点）。
//
// 三者来自同一份审核过的正式目录，所以「能传到」的 floor 集合和树里的地图节点集合完全相等。
//
// STAGE-258 起可选参数（不传则与原来一致，全部返回）：
//   - `parts`：字符串数组，取 "maps" / "nodes"，只返回选中的部分（没选的键不出现）；缺省或空数组=全部；
//   - `keyword`：对 maps 平面表按名字做子串过滤（不区分大小写，去掉首尾空白后为空则不过滤），不影响 nodes；
//     只带 keyword 不带 parts 时，按 parts=["maps"] 处理（否则整棵节点树照发，不压缩的连接仍然超限）。
func (s *Service) handleMaps(_ context.Context, sess *gateway.Session, req *protocol.Request) (any, error) {
	var in struct {
		Parts   []string `json:"parts"`
		Keyword string   `json:"keyword"`
	}
	if perr := req.Decode(&in); perr != nil {
		return nil, perr
	}
	wantMaps, wantNodes := true, true
	if len(in.Parts) > 0 {
		wantMaps, wantNodes = false, false
		for _, p := range in.Parts {
			switch p {
			case mapsPartMaps:
				wantMaps = true
			case mapsPartNodes:
				wantNodes = true
			default:
				return nil, protocol.Errorf(protocol.CodeBadRequest, "parts 只能是 %q 或 %q，收到 %q", mapsPartMaps, mapsPartNodes, p)
			}
		}
	}
	if len(in.Parts) == 0 && strings.TrimSpace(in.Keyword) != "" {
		wantNodes = false
	}
	if _, err := s.liveOrErr(sess); err != nil {
		return nil, err
	}
	out := map[string]any{"catalog_version": gamedata.TravelCatalogVersion()}
	if wantMaps {
		out["maps"] = filterWarpMaps(gamedata.WarpableMaps(), in.Keyword)
	}
	if wantNodes {
		out["nodes"] = gamedata.TravelCatalog()
	}
	return out, nil
}

// filterWarpMaps 按名字子串过滤平面表；keyword 去掉首尾空白后为空则原样返回。结果总是非 nil，JSON 里是 []。
func filterWarpMaps(all []gamedata.WarpMap, keyword string) []gamedata.WarpMap {
	kw := strings.ToLower(strings.TrimSpace(keyword))
	out := make([]gamedata.WarpMap, 0, len(all))
	for _, m := range all {
		if kw == "" || strings.Contains(strings.ToLower(m.Name), kw) {
			out = append(out, m)
		}
	}
	return out
}
