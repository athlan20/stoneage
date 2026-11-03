package gamedata

// 任务 NPC 的事件脚本（STAGE-90，docs/quest.md）：npc-event.json 由 tools/gen-npcs.py（tools/npcevent.py）
// 从原版服务端 data/npc 的 changeevent 脚本转换，对照 gmsv/npc/npc_exchangeman.c。这里只提供数据和白名单，
// 选块、条件和交易在 server/character（npcevent*.go）里做。

import (
	_ "embed"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

//go:embed npc-event.json
var npcEventJSON []byte

// EventScript 是一个任务 NPC 的脚本。File 是脚本文件相对 data/npc 的路径，白名单按它判断。
type EventScript struct {
	ID     int          `json:"id"`
	File   string       `json:"file"`
	Main   []string     `json:"main"`   // NomalMainMsg：没有块命中时随机说的一句（闲聊）
	Blocks []EventBlock `json:"blocks"` // 文件顺序：原版按顺序选第一个命中的块
}

// EventBlock 是脚本里的一个事件块。Unsupported 非空的块运行时不放行：遇到它（而且它的事件没完成）时
// 对话按“还不能对话”处理。Keyword 非空的块是暗号块：只有玩家说的话和它逐字相同时才会考虑，普通对话跳过。
type EventBlock struct {
	Type        string        `json:"type"` // MESSAGE / ACCEPT / REQUEST / CLEAN
	Event       int           `json:"event"`
	Cond        [][]EventTerm `json:"cond"` // 外层是 OR（逗号），内层是 AND（&）
	Msg         EventMsg      `json:"msg"`
	GetItem     []ItemCount   `json:"get_item"`
	DelItem     []ItemCount   `json:"del_item"` // N 为 0 表示删掉背包里所有这个编号的道具
	GetRand     []int         `json:"get_rand"` // 随机给其中一件
	GetStone    int           `json:"get_stone"`
	DelStone    *StoneCost    `json:"del_stone"`
	Charm       int           `json:"charm"`
	EndSet      []int         `json:"end_set"`     // EndSetFlg：清进行中、设完成
	Clean       []int         `json:"clean"`       // CleanFlg：清完成和进行中
	Warp        []WarpPoint   `json:"warp"`        // NpcWarp：这一步成功后 NPC 移到下一个位置（STAGE-93，见 npcboard.go）
	DelPet      []PetAsk      `json:"del_pet"`     // DelPet：直接写的收宠物项（Count 0 表示全部收走）
	DelPetEV    [][]PetAsk    `json:"del_pet_ev"`  // DelPet:EVDEL：每个候选组一份，取命中的那一组（npcevent_pet.go）
	DelItemEV   [][]ItemCount `json:"del_item_ev"` // DelItem:EVDEL：每个候选组一份，N 为 0 表示收走全部（NotDel 已排除）
	GetPet      []int         `json:"get_pet"`     // GetPet：怪物编号（ENEMY_ID），逗号分的，运行时随机选一个
	Keyword     string        `json:"keyword"`     // 暗号原文（逐字，区分大小写），空串表示不是暗号块
	Unsupported string        `json:"unsupported"`
	// MissingItem 是 STAGE-324：GetItem 引用了道具表里没有的编号（原版 itemset.txt 也没有，如 2745）。
	// 原版 ITEM_makeItemAndRegist 返回 -1，整个块的交易失败、什么都不改；运行时照此处理（npcevent_deal.go）。
	MissingItem bool `json:"missing_item"`
}

// WarpPoint 是 NpcWarp 的一个位置：地图编号和格子坐标。
type WarpPoint struct {
	Floor int `json:"floor"`
	X     int `json:"x"`
	Y     int `json:"y"`
}

// EventTerm 是一个条件。ITEM 时 ID 是道具编号、N 是数量（默认 1）；其他标记 N 是数值。
// Op 取 ITEM / LV / ENDEV / NOWEV / TIME，Cmp 取 = / != / < / >（对照 NPC_EventFreeIfCheck）。
type EventTerm struct {
	Op  string  `json:"op"`
	Cmp string  `json:"cmp"`
	ID  int     `json:"id"`
	N   int     `json:"n"`
	Pet *PetAsk `json:"pet,omitempty"` // Op 为 PET 时的宠物项（npcevent_pet.go）
}

// ItemCount 是一条物品动作：道具编号和数量（每件占一格）。
type ItemCount struct {
	ID int `json:"id"`
	N  int `json:"n"`
}

// StoneCost 是 DelStone 的金币：固定值，或等级乘以系数（原版 NPC_EventGetCost 的 LV*N）。
type StoneCost struct {
	Fixed int `json:"fixed"`
	PerLv int `json:"per_lv"`
}

// EventMsg 是一个块的台词（字段名对照 tools/npcevent.py 的 MSG_FIELDS，逐字保留，换行已转成 \n）。
type EventMsg struct {
	Nomal       string `json:"nomal"`
	NomalWindow string `json:"nomal_window"`
	Accept      string `json:"accept"`
	Thanks      string `json:"thanks"`
	ItemFull    string `json:"item_full"`
	Request     string `json:"request"`
	Stop        string `json:"stop"`
	EndStop     string `json:"end_stop"`
	NoStop      string `json:"no_stop"`
	StoneLess   string `json:"stone_less"`
	StoneFull   string `json:"stone_full"`
	CleanMain   string `json:"clean_main"`
	CleanFlg    string `json:"clean_flg"`
	PetFull     string `json:"pet_full"` // PetFullMsg：宠物栏满时的台词（GetPet 必须有，STAGE-197）
	// Paged 是分页台词（STAGE-97）：键是 accept、thanks、request、nomal_window，值是全部页（按页序）。
	// 有分页的台词只写这里，不写上面的单页字段；用 Pages 取，不要直接读单页字段。
	Paged map[string][]string `json:"paged"`
}

// Pages 返回台词 kind（accept、thanks、request、nomal、nomal_window）的各页，按页序；没有台词返回 nil。
// 分页台词取 Paged，单页台词取对应字段的一页。其他台词（stop、clean_main 等）没有分页，直接读字段。
func (m EventMsg) Pages(kind string) []string {
	if pages, ok := m.Paged[kind]; ok {
		return pages
	}
	text := ""
	switch kind {
	case "accept":
		text = m.Accept
	case "thanks":
		text = m.Thanks
	case "request":
		text = m.Request
	case "nomal":
		text = m.Nomal
	case "nomal_window":
		text = m.NomalWindow
	}
	if text == "" {
		return nil
	}
	return []string{text}
}

// npcEventEnabled 是放开的任务脚本（按脚本文件）。数据是全量转换的，但只有这里列出的脚本会生效，
// 其余任务 NPC 对话时回“还不能对话”。第一批只放「成人仪式」；第二批（STAGE-93）放 NpcWarp 补齐后
// 全部块都支持的脚本（docs/quest.md 的分批清单）。
var npcEventEnabled = map[string]bool{
	"jaruga/event/event04_1":    true, // 仪式的审判（审判官）
	"jaruga/event/event04_2":    true, // 仪式审判的差使（使者）
	"jaruga/event/ruri03":       true, // 琉璃的洞窟·蒂雅特的事件（NpcWarp）
	"jaruga/event/ruri04":       true, // STAGE-222：琉璃的洞窟·大地的主人（只说闲话，不发奖励、不改状态）
	"giiru/event/giruev2_1":     true, // STAGE-322：暴龙之路（任务 14，帖尼姆团长）
	"giiru/event/event30_1":     true, // STAGE-322：黄金羚羊之路（任务 15，采石所管理人）
	"giiru/event/event30_2":     true, // STAGE-322：黄金羚羊之路（任务 16、17，精灵之石）
	"giiru/event/event30_6":     true, // STAGE-322：黄金羚羊之路（任务 16，精灵之木）
	"giiru/event/garunaev2":     true, // STAGE-323：小猪爱情故事加鲁那（任务 20，雪鲁：接取）
	"giiru/event/garunaev3":     true, // STAGE-323：小猪爱情故事加鲁那（任务 20，美鲁：完成；NOEEV 恒假）
	"giiru/event/garunaev5":     true, // STAGE-323：小猪爱情故事加特洛洞（任务 21，左恩巴：接取）
	"giiru/event/garunaev6":     true, // STAGE-323：小猪爱情故事加特洛洞（任务 21，加洛：完成）
	"giiru/event/sink/sink01":   true, // NpcWarp
	"giiru/event/sink/sink02":   true, // STAGE-316：深红洞窟（任务 37，伊拉交不死鸟的羽毛）
	"giiru/event/sink/sink07":   true, // STAGE-316：深红洞窟（任务 37，达那村真红接取）
	"giiru/event/sink/sink055":  true, // NpcWarp
	"giiru/event/sink/sink055a": true, // NpcWarp
	"giiru/event/ev28_0":        true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_1":        true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_2":        true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_3":        true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_4":        true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_5":        true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_6":        true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_7":        true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_8":        true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_9":        true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_10":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_11":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_12":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_13":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_14":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_15":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_16":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_17":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_18":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_19":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_20":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_21":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_22":       true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_end":      true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_ed1":      true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_ed2":      true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_ed3":      true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_ed4":      true, // STAGE-313：五兄弟之迷（任务 18、19）
	"giiru/event/ev28_ed5":      true, // STAGE-313：五兄弟之迷（任务 18、19）
	"poru/mary02":               true, // NpcWarp
	"poru/samugiru":             true, // NpcWarp
	"poru/girl":                 true, // STAGE-333：萨姆吉尔的勾玉（任务 111 兑换：五块齐换首饰和英雄的祝福）
	"poru/samugiru04":           true, // STAGE-333：萨姆吉尔的勾玉（任务 111 的提示）
	"poru/samugiru01":           true, // STAGE-339：迷路的小女孩（1313，只说话，按事件 64 换台词）
	"poru/girl01":               true, // STAGE-339：害怕的小女孩（1314，只说话）
	"poru/boy":                  true, // STAGE-339：小男孩（1218，只说话，恐龙的闲话）
	"poru/mary03":               true, // STAGE-339：泰坦队士兵（1221，只说话）
	"poru/mary01":               true, // STAGE-339：四圣石最终交付（任务 41 完成，萨姆吉尔的圣石交给金格萨贝鲁，NPC 1316）
	"overlay/gouyu_elder":       true, // STAGE-333：萨姆吉尔的勾玉（任务 111 接取：长老，5500，NPC 640）
	"jaruga/event/20501ev1":     true, // STAGE-94
	"jaruga/event/20501ev2":     true, // STAGE-94
	"jaruga/event/20501ev3":     true, // STAGE-94
	"jaruga/event/20501ev4":     true, // STAGE-94
	"jaruga/event/20501ev5":     true, // STAGE-94
	"jaruga/event/20700_1":      true, // STAGE-94
	"jaruga/event/bait_01":      true, // STAGE-94
	"jaruga/event/bait_04":      true, // STAGE-94
	"jaruga/event/event01_1":    true, // STAGE-94
	"jaruga/event/event06_1":    true, // STAGE-221：哥亚山洞窟（任务 23，乌力接任务、交贝壳领宠物）
	"jaruga/event/event06_2":    true, // STAGE-94
	"jaruga/event/event07_1":    true, // STAGE-94
	"jaruga/event/event07_10":   true, // STAGE-94
	"jaruga/event/event07_2":    true, // STAGE-94
	"jaruga/event/event07_3":    true, // STAGE-94
	"jaruga/event/event07_4":    true, // STAGE-94
	"jaruga/event/event07_5":    true, // STAGE-94
	"jaruga/event/event07_6":    true, // STAGE-94
	"jaruga/event/event07_7":    true, // STAGE-94
	"jaruga/event/event07_8":    true, // STAGE-94
	"jaruga/event/event07_9":    true, // STAGE-94
	"jaruga/event/event17_1":    true, // STAGE-94
	"jaruga/event/event17_2":    true, // STAGE-94
	"jaruga/event/event17_3":    true, // STAGE-94
	"jaruga/event/event17_4":    true, // STAGE-94
	"jaruga/event/event20_1":    true, // STAGE-94
	"jaruga/event/event20_2":    true, // STAGE-324：强恩洞窟（任务 27、28、29，吉德；块 4 引用原版也不存在的 2744/2745）
	"jaruga/event/event20_3":    true, // STAGE-94
	"jaruga/event/event20_4":    true, // STAGE-94
	"jaruga/event/event20_5":    true, // STAGE-94
	"jaruga/event/event20_6":    true, // STAGE-94
	"jaruga/event/hekisei":      true, // STAGE-94
	"jaruga/event/hekisei01":    true, // STAGE-314：碧青洞窟（任务 35，女神艾妮基：交碧青水晶换水的戒指）
	"jaruga/event/hekisei03":    true, // STAGE-94
	"jaruga/event/hekisei04":    true, // STAGE-94
	"jaruga/event/oev_0":        true, // STAGE-94
	"jaruga/event/oev_1":        true, // STAGE-94
	"jaruga/event/oev_10":       true, // STAGE-94
	"jaruga/event/oev_10b":      true, // STAGE-94
	"jaruga/event/oev_11":       true, // STAGE-94
	"jaruga/event/oev_11b":      true, // STAGE-94
	"jaruga/event/oev_12":       true, // STAGE-94
	"jaruga/event/oev_12b":      true, // STAGE-94
	"jaruga/event/oev_1b":       true, // STAGE-94
	"jaruga/event/oev_2":        true, // STAGE-94
	"jaruga/event/oev_2b":       true, // STAGE-94
	"jaruga/event/oev_3":        true, // STAGE-94
	"jaruga/event/oev_3b":       true, // STAGE-94
	"jaruga/event/oev_4":        true, // STAGE-94
	"jaruga/event/oev_4b":       true, // STAGE-94
	"jaruga/event/oev_5":        true, // STAGE-94
	"jaruga/event/oev_5b":       true, // STAGE-94
	"jaruga/event/oev_6":        true, // STAGE-94
	"jaruga/event/oev_6b":       true, // STAGE-94
	"jaruga/event/oev_7":        true, // STAGE-94
	"jaruga/event/oev_7b":       true, // STAGE-94
	"jaruga/event/oev_8":        true, // STAGE-94
	"jaruga/event/oev_8b":       true, // STAGE-94
	"jaruga/event/oev_9":        true, // STAGE-94
	"jaruga/event/oev_9b":       true, // STAGE-94
	"king/event69_1":            true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_3":            true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_4":            true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_6":            true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_7":            true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_11":           true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_12":           true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_14":           true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_16":           true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_18":           true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_20":           true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_22":           true, // STAGE-220：精灵王的传说（king 目录）
	"king/event69_24":           true, // STAGE-220：精灵王的传说（king 目录）
	"jaruga/event/event01_2":    true, // STAGE-222：塔姆塔姆村的药草（任务 30，咍罗山的医生）
	"jaruga/event/ruri01":       true, // STAGE-222：琉璃洞窟（任务 36，女神蒂亚特）
	"jaruga/event/ruri02":       true, // STAGE-222：琉璃洞窟（任务 36，拉鲁）
	"extra/event/hougyoku":      true, // STAGE-319：四宝玉之迷（任务 31，奇努伊村村子的学者）
	"quiz/equiz_01":             true, // STAGE-318：猜谜3兄弟（任务 2、3，魁司：接任务 2）
	"quiz/equiz_02":             true, // STAGE-318：猜谜3兄弟（任务 2、3，伊那多：交入门证给金布伊、练到 77 级再给入门证）
	"quiz/equiz_04":             true, // STAGE-318：猜谜3兄弟（任务 3，大师的弟子：交谜之箱给左迪洛斯）
	"seimu/event/event01_1":     true, // STAGE-320：梦幻洞窟（任务 32、33、34，奇努伊村的老人：派先见之光）
	"seimu/event/event01_2":     true, // STAGE-320：梦幻洞窟（任务 32、33、34，梦见的仙人：交先见之光换竖琴/帽子/衣服）
	"seimu/event/event02_1":     true, // STAGE-320：卡坦 找4只鲨鱼（任务 4，卡坦：收 4 只 25 级恐龙）
	"sainasu/event/event15_1":   true, // STAGE-315：恐龙博士（任务 6、7，哈鲁：收鲁尼帖斯、贝鲁卡）
	"sainasu/event/event13_1":   true, // STAGE-315：强盗洞窟（任务 10，老人萨顿：收三件传家宝）
	"sainasu/event/event09":     true, // STAGE-315：梦德洞窟（任务 8，柯尔克村姑：接取和交付）
	"sainasu/event/event10":     true, // STAGE-315：梦德洞窟（任务 8，霍特尔村姑：收霍特尔的保护石）
	"sainasu/event/event11":     true, // STAGE-315：梦德洞窟（任务 8，卡坦的年轻人：收卡坦的保护石）
	"sainasu/event/event12":     true, // STAGE-315：梦德洞窟（任务 8，梦德族长：给两块保护石；任务 38 给风之竖琴）
	"sainasu/event/event03_1":   true, // STAGE-315：亚姆亚姆的斧头（任务 12、13，马提：收斧头、写信）
	"sainasu/event/event03_2":   true, // STAGE-315：亚姆亚姆的斧头（任务 12、13，亚姆亚姆：给斧头、收信换改良斧头）
	"sainasu/event/genou_1":     true, // STAGE-315：玄黄洞窟-1/2（任务 38、39，威尔：接任务 38、收风之竖琴换风之锁）
	"sainasu/event/genou_3":     true, // STAGE-315：玄黄洞窟-2（任务 39，姆哈萨：接任务 39）
	"sainasu/event/nevent01_1":  true, // STAGE-338：钓竿专家（任务 47/48 链上的材料，钓竿换鱼和壳，NPC 1256）
	"sainasu/event/nevent01_2":  true, // STAGE-338：钓竿专家（曼模竿+钓鱼线做钓竿 2432~2434，NPC 1260、1299；暗号块 楍 已删，见 tools/npc_event_overlay.py）
	"poru/boy02":                true, // STAGE-317：红暴任务（任务 40，走失孙女的老爷爷：接取）
	"poru/boy04":                true, // STAGE-317：红暴任务（任务 40，龙王：给龙王的守护）
	"poru/mary":                 true, // STAGE-317：四圣石任务（任务 41 接取；任务 42、44 的前置）
	"poru/mary04":               true, // STAGE-317：四圣石任务（任务 44，小队长：给鲁乌）
	"poru/team01":               true, // STAGE-317：四圣石任务（任务 42 接取、任务 44 接取；泰坦队长）
	"poru/titan":                true, // STAGE-317：四圣石任务（任务 42，泰坦队队员：收公文换通行证）
	"poru/mary05":               true, // STAGE-338：四圣石任务（任务 48，加美训练师 1322：给加美 1482）
	"poru/team03":               true, // STAGE-338：四圣石任务（任务 47 交付：仙人钓竿 2432 + 33 级加美，给岚之圣石 18559）
	"poru/mary15":               true, // STAGE-338：四圣石任务（加美闲聊，NPC 1324）
	"poru/mary06":               true, // STAGE-337：涛之圣石（任务 46 迪欧训练师：交付后给迪欧 719，NPC 1327）
	"poru/team02":               true, // STAGE-337：涛之圣石（任务 46 接取 REQUEST 67，任务 45 交付：采伐许可证 2478 + 30 级迪欧给涛之圣石 18557，NPC 1326）
	"poru/mary16":               true, // STAGE-337：涛之圣石（迪欧闲聊，只说话、不给东西，NPC 1325）
	"poru/mary13":               true, // STAGE-336：炎之圣石（任务 41 进行中，埋宝箱的：龙王的守护换挖掘物 18561/18562）
	"poru/mary12":               true, // STAGE-336：炎之圣石（考古队员：挖掘物 18561/18562 换炎之圣石 18558，不看任务进度）
	"sainasu/event/nevent03_1":  true, // STAGE-335：柯尔克坑道采石鉴定（一级矿石鉴定家：2469 换 2472）
	"sainasu/event/nevent03_2":  true, // STAGE-335：柯尔克坑道采石鉴定（特级矿石鉴定家：2470 换 2473）
	"sainasu/event/nevent03_3":  true, // STAGE-335：柯尔克坑道采石鉴定（最高级矿石鉴定家：2471 换 2474，岳之圣石的材料）
	"sainasu/event/nevent03_4":  true, // STAGE-335：柯尔克坑道崩塌受困（3 层矿工：无矿石时给 1 个 2471）
	"sainasu/event/nevent03_5":  true, // STAGE-335：柯尔克坑道管理处（办事处负责人：只说明，不给东西）
	"sainasu/event/nevent03_6":  true, // STAGE-335：柯尔克坑道管理处（职员：只说明，不给东西）
	"extra/event/marju":         true, // STAGE-321：马祖任务（任务 5 接取、任务 9 交付）
	"extra/event/marju01":       true, // STAGE-321：水田迷宫（任务 5 的卡伊特，NOWEV!= 按原版恒真）
}

// NpcEventEnabled 报告脚本文件是否放开（生效）：白名单里的，或者配置开关 STONEAGE_NPC_EVENT_OPEN 放开的（STAGE-214，npcevent_open.go）。
func NpcEventEnabled(file string) bool { return npcEventEnabled[file] || npcEventOpenFor(file) }

var (
	npcEventOnce sync.Once
	npcEventByID map[int]*EventScript
)

// NpcEventOf 按 NPC 编号取任务脚本；不是任务 NPC 或没有脚本时返回 false。
func NpcEventOf(npcID int) (*EventScript, bool) {
	npcEventOnce.Do(func() {
		var list []EventScript
		if err := json.Unmarshal(npcEventJSON, &list); err != nil {
			panic("npc-event.json 解析失败: " + err.Error())
		}
		markPetGifts(list) // GetPet 的怪物和宠物模板要查得到（STAGE-197，npcevent_pet.go）
		npcEventByID = make(map[int]*EventScript, len(list))
		for i := range list {
			npcEventByID[list[i].ID] = &list[i]
		}
	})
	s, ok := npcEventByID[npcID]
	return s, ok
}

// NpcEventScripts 返回全部任务脚本（每个任务 NPC 一份，按 NPC 编号升序）。给分析工具用（cmd/stoneage-questchain，STAGE-213），
// 不论白名单和开关，全部返回。返回的是内部数据，调用方不要修改。
func NpcEventScripts() []*EventScript {
	NpcEventOf(0) // 先解析 npc-event.json（只做一次）
	out := make([]*EventScript, 0, len(npcEventByID))
	for _, s := range npcEventByID {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// EventTimeSection 是游戏时段（0 夜、1 晨、2 午、3 晚，对照 handletime.c 的 getLSTime），
// 条件 TIME!=n 用它。时段的边界与报时 NPC 用的是同一套时间（lsHour）。
func EventTimeSection(now time.Time) int {
	switch hour := lsHour(now); {
	case 700 < hour && hour <= 930:
		return 1
	case 200 < hour && hour <= 300:
		return 3
	case 300 < hour && hour <= 700:
		return 0
	default:
		return 2
	}
}
