// Package casetest 读取并重放 docs/protocol/cases/ 里的协议用例（格式见 docs/protocol.md 第 6 节），
// 把服务端的回复和用例里的期望逐字段比对。服务端的 STAGE-2、3、4 都用它验收。
package casetest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/athlan20/stoneage/server/gamedata"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Meta 是用例首行的说明。
type Meta struct {
	Desc  string `json:"desc"`
	Stage string `json:"stage"` // 这个用例由哪个需求负责实现，如 STAGE-2
	// ServerConfig 是跑这个用例时服务端需要的配置，如 {"idle_timeout_seconds": 2}。
	ServerConfig map[string]any `json:"server_config"`
	// Manual 为 true 表示自动化测试跳过（需要人工触发），只给模拟服务端和阅读用。
	Manual bool `json:"manual"`
}

// Step 是用例里的一行。
type Step struct {
	Line    int
	Conn    string // 连接名，缺省为 player
	Dir     string // c2s、s2c、close 或 admin
	Msg     json.RawMessage
	Raw     string // c2s：不经 JSON 编码，原样发送的一行
	RawLen  int    // c2s：发送一行 N 个字节的数据（用来测超长行）
	Closed  bool   // s2c：期望服务端关闭这条连接
	Z       string // s2c：期望线上这一行是压缩行（STAGE-246，如 "gzip"）；空表示必须是原样 JSON
	DelayMs int    // c2s：发送前等待；s2c：等待服务端发来的时限延长这么久
	// Do 和 Account 用于 dir 为 admin 的行：以管理员身份直接操作服务端（如 disable_account），
	// 不经过协议。模拟服务端只处理 player 连接，会跳过这类行。
	// Do 为 give_item 时用 Character（角色名）、ItemID、Count（物品编号与数量）；
	// 为 give_pet 时用 Character、PetID、Level（宠物编号与等级，缺省为 1）、Loyalty（忠诚的可变部分，单位是点，100 即封顶；缺省 0，STAGE-127）；
	// 为 give_points 时用 Character、Points（发的未分配点数，docs/protocol.md 5.21）；
	// 为 faint_pet 时用 Character、Slot（把栏位上宠物的血量设为 0，见 character.FaintPet）；
	// 为 start_battle 时用 Character、Scenario（让角色进入一场预设剧本的战斗，docs/protocol.md 10.13）；
	// 为 set_setting 时用 Key、Value（改全服设置，如 exp_rate 2，docs/server.md 管理命令）；
	// 为 move_character 时用 Character、Floor、X、Y（把角色挪到指定位置，室内 NPC 用例用）。
	// 为 set_pet_transmigration 时用 Character、Slot、Trans（把栏位上宠物的转生次数设为 trans，STAGE-196）。
	// 为 set_transmigration 时用 Character、Trans（把人物的转生次数设为 trans，STAGE-266）。
	// 为 give_quest 时用 Character、QuestNo、State（设任务进度，doing/done/clear，STAGE-86）。
	Do         string
	Account    string
	Character  string
	ItemID     int
	Count      int
	PetID      int
	Level      int
	Loyalty    int // give_pet 用：宠物忠诚的可变部分（点），缺省 0
	Trans      int // set_pet_transmigration 用：宠物的转生次数（STAGE-196）
	Seconds    int // chat_mute 用：禁言秒数（STAGE-278）
	Points     int
	Slot       int
	Scenario   string
	MagicID    int         // give_magic 用：精灵编号
	Key        string      // set_setting 用：设置名
	Value      json.Number // set_setting 用：设置值（数字或字符串都收，按原文校验）
	Floor      int         // move_character 用：目标地图编号
	X, Y       int         // move_character 用：目标坐标
	QuestNo    int         // give_quest 用：任务编号
	QuestState string      // give_quest 用：doing、done 或 clear
	HP         int         // set_hp 用：血量（STAGE-306，决斗用例造一击倒下的场面）
	DuelPoint  int         // set_duel_point 用：决斗点数（STAGE-306）
	Gold       int         // set_gold 用：石币（STAGE-328，0–持有上限）
}

// GivePetLevel 是 give_pet 行的等级；行里没写时取 1（与 stoneage-admin givepet 不写等级时的默认一致）。
func (s Step) GivePetLevel() int {
	if s.Level == 0 {
		return gamedata.NewPetLevel
	}
	return s.Level
}

// Case 是一个场景。
type Case struct {
	Name  string
	Path  string
	Meta  Meta
	Steps []Step
}

// Save 把用例写回文件（Record 模式校准用例后调用，见 RunOptions.Record）。
func (c *Case) Save() error {
	var b strings.Builder
	meta, err := json.Marshal(c.Meta)
	if err != nil {
		return err
	}
	b.WriteString(`{"meta": `)
	b.Write(meta)
	b.WriteString("}\n")
	for _, s := range c.Steps {
		row := map[string]any{"conn": s.Conn, "dir": s.Dir}
		if len(s.Msg) > 0 {
			row["msg"] = s.Msg
		}
		if s.Raw != "" {
			row["raw"] = s.Raw
		}
		if s.RawLen > 0 {
			row["raw_len"] = s.RawLen
		}
		if s.Closed {
			row["closed"] = true
		}
		if s.Z != "" {
			row["z"] = s.Z
		}
		if s.DelayMs > 0 {
			row["delay_ms"] = s.DelayMs
		}
		if s.Dir == "admin" {
			row["do"] = s.Do
			if s.Account != "" {
				row["account"] = s.Account
			}
			if s.Character != "" {
				row["character"] = s.Character
			}
			if s.ItemID != 0 {
				row["item_id"] = s.ItemID
			}
			if s.Count != 0 {
				row["count"] = s.Count
			}
			if s.PetID != 0 {
				row["pet_id"] = s.PetID
			}
			if s.Level != 0 {
				row["level"] = s.Level
			}
			if s.Points != 0 {
				row["points"] = s.Points
			}
			if s.Loyalty != 0 {
				row["loyalty"] = s.Loyalty
			}
			if s.Trans != 0 {
				row["trans"] = s.Trans
			}
			if s.HP != 0 {
				row["hp"] = s.HP
			}
			if s.DuelPoint != 0 {
				row["duel_point"] = s.DuelPoint
			}
			if s.Gold != 0 {
				row["gold"] = s.Gold
			}
			if s.Scenario != "" {
				row["scenario"] = s.Scenario
			}
			if s.MagicID != 0 {
				row["magic_id"] = s.MagicID
			}
			if s.Key != "" {
				row["key"] = s.Key
			}
			if s.Value != "" {
				row["value"] = s.Value
			}
			if s.Floor != 0 {
				row["floor"] = s.Floor
				row["x"] = s.X
				row["y"] = s.Y
			}
		}
		line, err := json.Marshal(row)
		if err != nil {
			return err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return os.WriteFile(c.Path, []byte(b.String()), 0o644)
}

// LoadDir 读取目录下所有 .jsonl 用例，按名字排序。
func LoadDir(dir string) ([]*Case, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var cases []*Case
	for _, p := range paths {
		c, err := LoadFile(p)
		if err != nil {
			return nil, err
		}
		cases = append(cases, c)
	}
	return cases, nil
}

// LoadFile 读取一个用例文件。
func LoadFile(path string) (*Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	c := &Case{Name: strings.TrimSuffix(filepath.Base(path), ".jsonl"), Path: path}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var row struct {
			Meta      *Meta           `json:"meta"`
			Conn      string          `json:"conn"`
			Dir       string          `json:"dir"`
			Msg       json.RawMessage `json:"msg"`
			Raw       string          `json:"raw"`
			RawLen    int             `json:"raw_len"`
			Closed    bool            `json:"closed"`
			Z         string          `json:"z"`
			DelayMs   int             `json:"delay_ms"`
			Do        string          `json:"do"`
			Account   string          `json:"account"`
			Character string          `json:"character"`
			ItemID    int             `json:"item_id"`
			Count     int             `json:"count"`
			PetID     int             `json:"pet_id"`
			Level     int             `json:"level"`
			Points    int             `json:"points"`
			Loyalty   int             `json:"loyalty"`
			Trans     int             `json:"trans"`
			HP        int             `json:"hp"`
			DuelPoint int             `json:"duel_point"`
			Gold      int             `json:"gold"`
			Seconds   int             `json:"seconds"`
			Slot      int             `json:"slot"`
			Scenario  string          `json:"scenario"`
			MagicID   int             `json:"magic_id"`
			Key       string          `json:"key"`
			Value     json.Number     `json:"value"`
			Floor     int             `json:"floor"`
			X         int             `json:"x"`
			Y         int             `json:"y"`
			QuestNo   int             `json:"quest_no"`
			State     string          `json:"state"`
		}
		if err := json.Unmarshal([]byte(text), &row); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		if row.Meta != nil {
			c.Meta = *row.Meta
			continue
		}
		if row.Conn == "" {
			row.Conn = "player"
		}
		step := Step{Line: n, Conn: row.Conn, Dir: row.Dir, Msg: row.Msg, Raw: row.Raw, RawLen: row.RawLen, Closed: row.Closed, Z: row.Z, DelayMs: row.DelayMs, Do: row.Do, Account: row.Account, Character: row.Character, ItemID: row.ItemID, Count: row.Count, PetID: row.PetID, Level: row.Level, Loyalty: row.Loyalty, Trans: row.Trans, Seconds: row.Seconds, Points: row.Points, Slot: row.Slot, Scenario: row.Scenario, MagicID: row.MagicID, Key: row.Key, Value: row.Value, Floor: row.Floor, X: row.X, Y: row.Y, QuestNo: row.QuestNo, QuestState: row.State, HP: row.HP, DuelPoint: row.DuelPoint, Gold: row.Gold}
		switch row.Dir {
		case "c2s":
			if len(row.Msg) == 0 && row.Raw == "" && row.RawLen == 0 {
				return nil, fmt.Errorf("%s:%d: c2s 行需要 msg、raw 或 raw_len", path, n)
			}
		case "s2c":
			if len(row.Msg) == 0 && !row.Closed {
				return nil, fmt.Errorf("%s:%d: s2c 行需要 msg 或 closed", path, n)
			}
		case "admin":
			if row.Do == "" {
				return nil, fmt.Errorf("%s:%d: admin 行需要 do", path, n)
			}
		case "close":
			if len(row.Msg) > 0 {
				return nil, fmt.Errorf("%s:%d: close 行不需要 msg", path, n)
			}
		default:
			return nil, fmt.Errorf("%s:%d: dir 只能是 c2s、s2c、close 或 admin", path, n)
		}
		c.Steps = append(c.Steps, step)
	}
	return c, sc.Err()
}
