// Package config 读取服务端配置。都来自环境变量：
// 优先级是真实环境变量 > .env 文件 > 代码里的默认值（docs/server.md）。
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/athlan20/stoneage/server/protocol"
)

const (
	EnvListen      = "STONEAGE_LISTEN"
	EnvDatabaseURL = "STONEAGE_DATABASE_URL"
	EnvAdminListen = "STONEAGE_ADMIN_LISTEN"
	// EnvEncounterInterval 是遇敌开关两场战斗之间的间隔（毫秒，STAGE-40）。
	EnvEncounterInterval = "STONEAGE_ENCOUNTER_INTERVAL_MS"
	// EnvRideLevelGap 是骑宠时人物等级加上它不能低于宠物等级（STAGE-123，docs/ride.md §3 第 8 条），默认 5。
	EnvRideLevelGap = "STONEAGE_RIDE_LEVEL_GAP"
	// EnvWSListen 是 WebSocket 入口的监听地址（STAGE-101）；空字符串表示不开。
	EnvWSListen = "STONEAGE_WS_LISTEN"
	// EnvTrustedProxies 是可信代理的地址段（逗号分隔的 IP 或 CIDR，如 Cloudflare 的网段）；
	// 只有直连地址在其中时，WebSocket 才采信 CF-Connecting-IP 作为玩家的真实 IP（STAGE-101）。
	EnvTrustedProxies = "STONEAGE_TRUSTED_PROXIES"
	// EnvMinClientVersion / EnvLatestClientVersion 是客户端版本要求（STAGE-100，x.y.z，空表示不检查）。
	EnvMinClientVersion    = "STONEAGE_MIN_CLIENT_VERSION"
	EnvLatestClientVersion = "STONEAGE_LATEST_CLIENT_VERSION"
	// EnvAgentMaxOnlineMinutes 是 agent 连续在线的上限（分钟，STAGE-118）；0 或不写表示不限制。
	EnvAgentMaxOnlineMinutes = "STONEAGE_AGENT_MAX_ONLINE_MINUTES"
	// EnvAutosaveSeconds 是在线角色定时存盘的间隔（秒，STAGE-290）；不写取默认 180，0 表示关闭。
	EnvAutosaveSeconds = "STONEAGE_AUTOSAVE_SECONDS"
	// EnvWikiCDN 是石器百科图片和地图的来源（STAGE-118），和 cmd/stoneage-wiki 的 -cdn 一样。
	EnvWikiCDN = "STONEAGE_WIKI_CDN"
	// EnvNpcEventOpen 是任务脚本的放开开关（STAGE-214）：空、all、或逗号分隔的脚本文件名，格式在 gamedata.SetNpcEventOpen 里解析。
	EnvNpcEventOpen = "STONEAGE_NPC_EVENT_OPEN"
	// EnvChatFilterFile 是聊天屏蔽词表文件的路径（STAGE-287）：UTF-8，一行一个词；空表示只用内置的少量词。
	// 文件不提交仓库，由运维放在服务器上；改完用 stoneage-admin chatfilter reload 让运行中的服务端重读。
	EnvChatFilterFile = "STONEAGE_CHAT_FILTER_FILE"
	// EnvInlayOverlayFile 是精工覆盖表的路径（STAGE-297，docs/inlay.md 2.4）：UTF-8，每行 物品编号、类别码、初始镶嵌码（TAB 分隔）。
	// 空表示用随程序发布的覆盖表（只有表头，不改任何物品）；编号不存在或文件读不出来，服务端启动失败。
	EnvInlayOverlayFile = "STONEAGE_INLAY_OVERLAY_FILE"
	// DefaultWikiCDN 为空表示不加载外部图片；部署时用 STONEAGE_WIKI_CDN 指向自己的图片源。
	DefaultWikiCDN = ""

	DefaultListen = "127.0.0.1:7065"
)

// Config 是服务端的全部配置。
type Config struct {
	Listen      string
	DatabaseURL string
	// AdminListen 是本地管理口的地址（协议用例的 start_battle 用，server/app/admin.go）；
	// 空字符串表示不开。
	AdminListen string
	// EncounterIntervalMs 是遇敌开关两场战斗之间的间隔（毫秒，STAGE-40）；0 取默认 500。
	EncounterIntervalMs int
	// WSListen 是 WebSocket 入口的监听地址，空字符串表示不开（STAGE-101）。
	WSListen string
	// TrustedProxies 是可信代理的地址段，见 EnvTrustedProxies。
	TrustedProxies []netip.Prefix
	// MinClientVersion 是最低客户端版本 x.y.z，低于它的客户端握手时被拒绝（STAGE-100）；空不检查。
	MinClientVersion string
	// LatestClientVersion 是最新客户端版本 x.y.z，握手响应里返回，客户端据此提示升级（STAGE-100）；空不返回。
	LatestClientVersion string
	// AgentMaxOnlineMinutes 是 agent 连续在线的上限（分钟），0 表示不限制（STAGE-118）。
	AgentMaxOnlineMinutes int
	// AutosaveSeconds 是在线角色定时存盘的间隔（秒，STAGE-290），0 表示关闭。
	AutosaveSeconds int
	// WikiCDN 是石器百科图片的来源，见 EnvWikiCDN。
	WikiCDN string
	// ChatFilterFile 是聊天屏蔽词表文件的路径，见 EnvChatFilterFile。
	ChatFilterFile string
	// InlayOverlayFile 是精工覆盖表的路径，见 EnvInlayOverlayFile。
	InlayOverlayFile string
	// RideLevelGap 是骑宠的等级差上限，见 EnvRideLevelGap。
	RideLevelGap int
	// NpcEventOpen 是任务脚本的放开开关，原样保留，见 EnvNpcEventOpen（STAGE-214）；空字符串是默认。
	NpcEventOpen string
}

// Load 加载 envFile（不存在则忽略），再用真实环境变量覆盖。
func Load(envFile string) (Config, error) {
	file := map[string]string{}
	if envFile != "" {
		m, err := ParseEnvFile(envFile)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Config{}, err
		}
		file = m
	}
	return resolve(file, os.LookupEnv)
}

func resolve(file map[string]string, lookup func(string) (string, bool)) (Config, error) {
	get := func(key, def string) string {
		if v, ok := lookup(key); ok && v != "" {
			return v
		}
		if v := file[key]; v != "" {
			return v
		}
		return def
	}
	getInt := func(key string, def int) int {
		if n, err := strconv.Atoi(get(key, "")); err == nil && n > 0 {
			return n
		}
		return def
	}
	proxies, err := ParseTrustedProxies(get(EnvTrustedProxies, ""))
	if err != nil {
		return Config{}, err
	}
	agentMax, err := strconv.Atoi(get(EnvAgentMaxOnlineMinutes, "0"))
	if err != nil || agentMax < 0 {
		return Config{}, fmt.Errorf("%s 必须是不小于 0 的整数（分钟，0 不限制），当前是 %q", EnvAgentMaxOnlineMinutes, get(EnvAgentMaxOnlineMinutes, ""))
	}
	autosave, err := strconv.Atoi(get(EnvAutosaveSeconds, "180"))
	if err != nil || autosave < 0 {
		return Config{}, fmt.Errorf("%s 必须是不小于 0 的整数（秒，0 关闭），当前是 %q", EnvAutosaveSeconds, get(EnvAutosaveSeconds, ""))
	}
	c := Config{
		Listen:                get(EnvListen, DefaultListen),
		DatabaseURL:           get(EnvDatabaseURL, ""),
		AdminListen:           get(EnvAdminListen, ""),
		EncounterIntervalMs:   getInt(EnvEncounterInterval, 500),
		RideLevelGap:          getInt(EnvRideLevelGap, 5),
		WSListen:              get(EnvWSListen, ""),
		TrustedProxies:        proxies,
		MinClientVersion:      get(EnvMinClientVersion, ""),
		LatestClientVersion:   get(EnvLatestClientVersion, ""),
		AgentMaxOnlineMinutes: agentMax,
		AutosaveSeconds:       autosave,
		WikiCDN:               get(EnvWikiCDN, DefaultWikiCDN),
		NpcEventOpen:          get(EnvNpcEventOpen, ""),
		ChatFilterFile:        get(EnvChatFilterFile, ""),
		InlayOverlayFile:      get(EnvInlayOverlayFile, ""),
	}
	return c, c.Validate()
}

// ParseTrustedProxies 解析逗号分隔的可信代理地址，每项可以是 IP 或 CIDR；空字符串返回 nil。
func ParseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		p, err := netip.ParsePrefix(item)
		if err != nil {
			addr, aerr := netip.ParseAddr(item)
			if aerr != nil {
				return nil, fmt.Errorf("%s 里的 %q 不是 IP 或 CIDR", EnvTrustedProxies, item)
			}
			p = netip.PrefixFrom(addr, addr.BitLen())
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// Validate 检查监听地址和连接串的格式。
func (c Config) Validate() error {
	if _, port, err := net.SplitHostPort(c.Listen); err != nil || port == "" {
		return fmt.Errorf("%s 必须是 host:port 形式，当前是 %q", EnvListen, c.Listen)
	}
	if c.WSListen != "" {
		if _, port, err := net.SplitHostPort(c.WSListen); err != nil || port == "" {
			return fmt.Errorf("%s 必须是 host:port 形式，当前是 %q", EnvWSListen, c.WSListen)
		}
	}
	for _, kv := range []struct{ env, val string }{
		{EnvMinClientVersion, c.MinClientVersion},
		{EnvLatestClientVersion, c.LatestClientVersion},
	} {
		if kv.val == "" {
			continue
		}
		if _, ok := protocol.ParseClientVer(kv.val); !ok {
			return fmt.Errorf("%s 必须是 x.y.z 形式（如 0.2.0），当前是 %q", kv.env, kv.val)
		}
	}
	return nil
}

// DatabaseParts 是连接串里和本地数据库容器相关的几项。
type DatabaseParts struct {
	User, Password, Name, Host, Port string
}

// ParseDatabaseURL 拆出连接串中的用户名、密码、库名、主机和端口（端口缺省为 5432）。
func ParseDatabaseURL(raw string) (DatabaseParts, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return DatabaseParts{}, fmt.Errorf("%s 必须是 postgres://用户:密码@主机:端口/库名 形式", EnvDatabaseURL)
	}
	p := DatabaseParts{Host: u.Hostname(), Port: u.Port(), Name: strings.TrimPrefix(u.Path, "/")}
	if p.Port == "" {
		p.Port = "5432"
	}
	if u.User != nil {
		p.User = u.User.Username()
		p.Password, _ = u.User.Password()
	}
	if p.User == "" || p.Name == "" || p.Host == "" {
		return DatabaseParts{}, fmt.Errorf("%s 缺少用户名、主机或库名", EnvDatabaseURL)
	}
	return p, nil
}

// IsLocalDatabase 报告连接串指向的是不是本机的数据库（localhost、127.0.0.1、::1 或本机 unix socket）。
// 清库之类的危险操作只允许对本机的库执行。连接串解析不了、或用 ?host= 指到别处时按“不是本机”处理。
func IsLocalDatabase(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if h := u.Query().Get("host"); h != "" {
		host = h
	}
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return strings.HasPrefix(host, "/") // unix socket 目录，只能是本机
}

// ParseEnvFile 读取 KEY=VALUE 形式的 .env 文件：忽略空行和 # 注释，支持 export 前缀和成对引号。
func ParseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("%s:%d: 需要 KEY=VALUE 形式", path, n)
		}
		out[key] = unquote(strings.TrimSpace(val))
	}
	return out, sc.Err()
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}
