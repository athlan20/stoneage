package protocol

import (
	"fmt"
	"strconv"
	"strings"
)

// 客户端版本（STAGE-100，docs/protocol.md 5.1）。hello 的 client 形如「名字/x.y.z」，
// 如 stoneage-cli/0.1.0。版本号只认 x.y.z 三段纯数字；取不到或格式不对时不做检查，
// 放行，避免误伤开发中的客户端。

// ClientVer 是解析后的 x.y.z（与协议版本号 Version 不是一回事）。
type ClientVer [3]int

// ParseClientVer 解析 x.y.z 三段纯数字版本号（如 0.2.0），其他格式返回 false。
func ParseClientVer(s string) (ClientVer, bool) {
	var v ClientVer
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return v, false
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// String 输出 x.y.z。
func (v ClientVer) String() string {
	return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])
}

// ClientVersion 从 hello 的 client（「名字/x.y.z」）里取版本号，取不到返回 false。
func ClientVersion(client string) (ClientVer, bool) {
	i := strings.LastIndex(client, "/")
	if i < 0 {
		return ClientVer{}, false
	}
	return ParseClientVer(client[i+1:])
}

// ClientTooOld 报告 hello 的 client 版本是否低于最低版本 minVersion。
// 客户端版本取不到、最低版本为空或格式不对时都不算过旧（放行）。
func ClientTooOld(client, minVersion string) bool {
	cur, ok := ClientVersion(client)
	if !ok {
		return false
	}
	least, ok := ParseClientVer(minVersion)
	if !ok {
		return false
	}
	for i := range cur {
		if cur[i] != least[i] {
			return cur[i] < least[i]
		}
	}
	return false
}
