package character

// 真人组队（STAGE-274，docs/social-players.md 第二节，docs/protocol.md 5.67）：真人队的状态、锁和视图。
//
// 真人队是独立的共享对象 realParty，成员的 Presence 只持有指向它的指针（atomic）。全服的真人队和请求表
// 共用 partyHub.mu 这一把锁（World 索引层）。锁顺序：
//
//	Presence.mu → partyHub.mu
//
// partyHub.mu 持有期间不拿任何 Presence.mu。一次只锁一个角色，唯一的例外是跟随（party_players_follow.go）：
// 队长持自己的 mu 时按 charID 升序锁队员，且只有这一处会同时持有两个角色的锁。
// 请求处理里的组队操作都在释放自己的角色锁之后再进入 partyHub，所以不会和跟随的嵌套反过来。

import (
	"sync"
	"time"

	"github.com/athlan20/stoneage/server/gamedata"
	"github.com/athlan20/stoneage/server/gateway"
)

// partySettingKey 是真人组队的全服开关（game_settings 的一行，值 1 为开，缺省为关；STAGE-274）。
const partySettingKey = "party.players_enabled"

const (
	partyMaxPlayers  = 5                // 真人队一队最多 5 个真人
	partyRequestTTL  = 30 * time.Second // 请求有效期（expires_in 固定 30）
	partyRateWindow  = 10 * time.Second // 同一发起人对同一对象的发送间隔
	partyMaxIncoming = 5                // 一个玩家同时待处理的请求上限
)

// partyHub 是全服的真人组队状态：队伍之间、请求表都归它的锁管。
type partyHub struct {
	s        *Service
	mu       sync.Mutex
	seq      int
	requests map[string]*partyRequest // 待处理的请求，id → 请求
	sent     map[pairKey]time.Time    // 上次发出请求的时间（频率限制，成功发出才记）
	timer    *time.Timer              // 到期扫描的单个定时器，有待处理请求时才存在
	joins    int                      // 入队序号的计数（STAGE-277：离队后按入队先后重新编号）
}

// pairKey 是 (发起人, 对象) 的名字对。角色名全服唯一，名字不变，所以用名字做键。
type pairKey struct{ from, to string }

// realParty 是一支真人队。members 只在 partyHub.mu 下读写；members[0] 是队长，其余按入队先后。
type realParty struct {
	members []*partyMember
}

// partyMember 是队里的一个真人。name 在入队时记下（角色名不变），读它不需要角色锁。
type partyMember struct {
	p      *Presence
	name   string
	joined int // 入队序号（越小越早）；转让队长会打乱 members 的顺序，离队重新编号时按它排（协议 5.67）
}

// nextJoin 分配一个入队序号。调用时持有 partyHub.mu。
func (h *partyHub) nextJoin() int {
	h.joins++
	return h.joins
}

// partyRequest 是一条待处理的邀请（invite）或申请（apply）。
type partyRequest struct {
	id       string
	kind     string // invite 或 apply
	from, to *Presence
	fromName string
	toName   string
	expires  time.Time
}

func newPartyHub(s *Service) *partyHub {
	return &partyHub{s: s, requests: map[string]*partyRequest{}, sent: map[pairKey]time.Time{}}
}

// indexOf 返回 p 在队里的位置（队长是 0），不在队里返回 -1。调用时持有 partyHub.mu。
func (pt *realParty) indexOf(p *Presence) int {
	for i, m := range pt.members {
		if m.p == p {
			return i
		}
	}
	return -1
}

// snapshot 返回 p 所在真人队成员的副本（没有真人队返回 nil）。持有 partyHub.mu 读取，调用时不能持有任何角色锁以外的锁。
func (h *partyHub) snapshot(p *Presence) []*partyMember {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.membersLocked(p)
}

// partySummary 是 players 列表与请求里的队伍摘要：{"leader", "size", "max"}，没有真人队时为 nil。
// 对外只计可见队员（STAGE-276，协议 5.69）：size 不算隐身的队员，队长隐身时 leader 为 nil。
func partySummary(ms []*partyMember) any {
	if len(ms) == 0 {
		return nil
	}
	size := 0
	for _, m := range ms {
		if !m.p.hidden.Load() {
			size++
		}
	}
	var leader any
	if !ms[0].p.hidden.Load() {
		leader = ms[0].name
	}
	return map[string]any{"leader": leader, "size": size, "max": partyMaxPlayers}
}

// playersMemberView 是真人队里的一名成员（协议 5.67）：只给组队界面要的血蓝和位置，不给坐标和属性。
type playersMemberView struct {
	Index    int                 `json:"index"`
	Name     string              `json:"name"`
	Leader   bool                `json:"leader"`
	Clone    bool                `json:"clone"`
	Level    int                 `json:"level"`
	HP       int                 `json:"hp"`
	MaxHP    int                 `json:"max_hp"`
	MP       int                 `json:"mp"`
	MaxMP    int                 `json:"max_mp"`
	Pet      *playersPetView     `json:"pet"`
	Online   bool                `json:"online"`
	Position playersPositionView `json:"position"`
}

// playersPetView 是队员的出战宠物（协议 5.67 只给这四项）。
type playersPetView struct {
	Name  string `json:"name"`
	Level int    `json:"level"`
	HP    int    `json:"hp"`
	MaxHP int    `json:"max_hp"`
}

type playersPositionView struct {
	Floor   int    `json:"floor"`
	MapName string `json:"map_name"`
}

// playersPartyView 是真人队的队伍对象（协议 5.67）：沿用 5.57 的外层结构，kind 为 players。
type playersPartyView struct {
	Kind    string              `json:"kind"`
	Leader  string              `json:"leader"`
	Members []playersMemberView `json:"members"`
}

// playersRowOf 取一名队员的视图行。调用时持有它的 mu（或者调用方已经持有所有队员的锁，见 partyViewOf）。
func playersRowOf(r *record, index int) playersMemberView {
	row := playersMemberView{
		Index: index, Name: r.Name, Leader: index == 0, Clone: false,
		Level: r.Level, HP: r.HP, MaxHP: r.maxHP(), MP: r.MP, MaxMP: gamedata.NewPlayerMaxMP,
		Online:   true,
		Position: playersPositionView{Floor: r.Floor, MapName: mapName(r.Floor)},
	}
	if lp := leaderPet(r); lp != nil {
		row.Pet = &playersPetView{Name: lp.Name, Level: lp.Level, HP: lp.HP, MaxHP: gamedata.PetMaxHP(lp.Stats)}
	}
	return row
}

// partyViewOf 把队伍（成员按顺序，队长在前）做成视图。locked 为 false 时逐个短暂地锁成员（不嵌套）；
// 为 true 时调用方已经持有所有成员的锁（跟随路径），直接读。
func partyViewOf(ms []*partyMember, locked bool) *playersPartyView {
	if len(ms) == 0 {
		return nil
	}
	v := &playersPartyView{Kind: "players", Leader: ms[0].name, Members: make([]playersMemberView, 0, len(ms))}
	for i, m := range ms {
		if !locked {
			m.p.mu.Lock()
		}
		v.Members = append(v.Members, playersRowOf(&m.p.rec, i))
		if !locked {
			m.p.mu.Unlock()
		}
	}
	return v
}

// peerState 是另一名角色在某一刻的状态（读完就放锁，之后只用它做判断，不持有它的锁）。
type peerState struct {
	p        *Presence
	name     string
	floor    int
	dead     bool
	battle   bool
	clone    bool // 有分身队
	real     bool // 有真人队
	leaving  bool
	noInvite bool // 拒绝组队邀请（STAGE-276，协议 5.69）
	trade    bool // 正在交易中（STAGE-294，协议 5.72）
}

// peerOf 读一名角色的组队相关状态。调用时不能持有任何角色的锁。
func peerOf(p *Presence) peerState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return peerState{
		p: p, name: p.rec.Name, floor: p.rec.Floor, dead: p.rec.HP <= 0,
		battle: p.battle != nil, clone: p.party != nil, real: p.realParty.Load() != nil, leaving: p.leaving,
		noInvite: p.rec.SocialNoInvite, trade: p.trade != nil,
	}
}

// partyActor 是发起组队请求的一方（控制权已经确认，角色锁已经放开）。
type partyActor struct {
	l     *Live
	p     *Presence
	name  string
	floor int
}

// partyActorOf 检查控制权并读出发起方的名字和地图。调用时不持有角色锁。
func (s *Service) partyActorOf(sess *gateway.Session) (*partyActor, error) {
	l, err := s.liveOrErr(sess)
	if err != nil {
		return nil, err
	}
	a := &partyActor{l: l, p: l.p}
	if err := l.Operate(func(r *record) {
		a.name, a.floor = r.Name, r.Floor
	}); err != nil {
		return nil, asProtocolError(err)
	}
	return a, nil
}

// partyBattling 报告队伍里是否有人在战斗中（逐个读，不嵌套）。
func partyBattling(ms []*partyMember) bool {
	for _, m := range ms {
		if peerOf(m.p).battle {
			return true
		}
	}
	return false
}

// findOnline 按角色名找在世界中的角色（players 索引是叶子锁，可以在任何时候调用）。
func (s *Service) findOnline(name string) *Presence {
	return s.players.byName(name)
}

// partyRealField 是 enter_world / status / party 的 party 字段里真人队的部分，调用时不能持有角色锁。
func (s *Service) partyRealField(p *Presence) any {
	if v := partyViewOf(p.parties.snapshot(p), false); v != nil {
		return v
	}
	return nil
}
