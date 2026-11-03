package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseRequest(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantCode string
		fatal    bool
		wantID   uint64
		wantType string
	}{
		{"ok", `{"id":3,"type":"login","data":{"account":"a"}}`, "", false, 3, "login"},
		{"no data", `{"id":1,"type":"ping"}`, "", false, 1, "ping"},
		{"null data", `{"id":1,"type":"ping","data":null}`, "", false, 1, "ping"},
		{"unknown fields ignored", `{"id":1,"type":"ping","data":{},"extra":1}`, "", false, 1, "ping"},
		{"garbage", `this is not json`, CodeBadRequest, true, 0, UnknownType},
		{"truncated", `{"id":1,"type":`, CodeBadRequest, true, 0, UnknownType},
		{"array", `[1,2]`, CodeBadRequest, false, 0, UnknownType},
		{"missing id", `{"type":"ping"}`, CodeBadRequest, false, 0, "ping"},
		{"string id", `{"id":"1","type":"ping"}`, CodeBadRequest, false, 0, "ping"},
		{"zero id", `{"id":0,"type":"ping"}`, CodeBadRequest, false, 0, "ping"},
		{"fraction id", `{"id":1.5,"type":"ping"}`, CodeBadRequest, false, 0, "ping"},
		{"missing type", `{"id":7}`, CodeBadRequest, false, 7, UnknownType},
		{"type not string", `{"id":7,"type":5}`, CodeBadRequest, false, 7, UnknownType},
		{"data not object", `{"id":7,"type":"ping","data":[1]}`, CodeBadRequest, false, 7, "ping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, perr, fatal := ParseRequest([]byte(tt.line))
			if (perr == nil) != (tt.wantCode == "") || (perr != nil && perr.Code != tt.wantCode) {
				t.Fatalf("err = %v, want %q", perr, tt.wantCode)
			}
			if fatal != tt.fatal {
				t.Errorf("fatal = %v, want %v", fatal, tt.fatal)
			}
			if req.ID != tt.wantID || req.Type != tt.wantType {
				t.Errorf("id/type = %d/%q, want %d/%q", req.ID, req.Type, tt.wantID, tt.wantType)
			}
		})
	}
}

func TestEncode(t *testing.T) {
	got := string(EncodeOK(9, "ping", nil))
	if got != `{"id":9,"type":"ping","ok":true,"data":{}}`+"\n" {
		t.Errorf("EncodeOK = %q", got)
	}
	e := Errorf(CodeRateLimited, "太频繁").WithData(map[string]any{"retry_after_seconds": 30})
	var m map[string]any
	if err := json.Unmarshal(EncodeError(2, "login", e), &m); err != nil {
		t.Fatal(err)
	}
	if m["ok"] != false || m["error"].(map[string]any)["code"] != CodeRateLimited {
		t.Errorf("EncodeError = %v", m)
	}
	if strings.Contains(string(EncodeError(2, "login", e.Closing())), "CloseAfter") {
		t.Error("CloseAfter 不应出现在协议里")
	}
	if got := string(EncodePush("notice", map[string]string{"level": "info"})); got != `{"type":"notice","data":{"level":"info"}}`+"\n" {
		t.Errorf("EncodePush = %q", got)
	}
}

func TestRequestTable(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range Requests {
		if seen[m.Type] {
			t.Errorf("重复登记 %s", m.Type)
		}
		seen[m.Type] = true
		if len(m.States) == 0 || len(m.Roles) == 0 {
			t.Errorf("%s 缺少状态或身份", m.Type)
		}
	}
	// 账号类请求只允许玩家连接。
	for _, m := range Requests {
		if m.Category == CategoryAccount && m.AllowsRole(RoleAgent) && m.Type != "agent_login" {
			t.Errorf("%s 是账号类请求，不应允许 agent", m.Type)
		}
	}
	login, _ := Lookup("login")
	if login.AllowsRole(RoleAgent) || !login.AllowsState(StateHandshaken) || login.AllowsState(StateLoggedIn) {
		t.Errorf("login 元数据不对: %+v", login)
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("不应查到不存在的请求")
	}
}

// 第 9 节的玩家专用请求只允许玩家连接，agent 登录只允许 agent 连接；
// “操作”类请求里除了 enter_world / leave_world 以外都要在世界中才能发。
func TestAgentRequestRoles(t *testing.T) {
	for _, typ := range []string{"create_agent_token", "agent_tokens", "revoke_agent_token", "take_control", "release_control", "kick_agent", "create_character", "delete_character"} {
		m, ok := Lookup(typ)
		if !ok || m.AllowsRole(RoleAgent) || !m.AllowsRole(RolePlayer) {
			t.Errorf("%s 应只允许玩家连接: %+v", typ, m)
		}
	}
	login, _ := Lookup("agent_login")
	if login.AllowsRole(RolePlayer) || !login.AllowsRole(RoleAgent) || !login.AllowsState(StateHandshaken) {
		t.Errorf("agent_login 元数据不对: %+v", login)
	}
	say, _ := Lookup("agent_say")
	if say.Category != CategoryChat || !say.AllowsRole(RoleAgent) || !say.AllowsRole(RolePlayer) || !say.AllowsState(StateLoggedIn) {
		t.Errorf("agent_say 元数据不对: %+v", say)
	}
	for _, m := range Requests {
		if m.Category == CategoryOperation && m.Type != "enter_world" && m.Type != "leave_world" && m.AllowsState(StateLoggedIn) {
			t.Errorf("操作类请求 %s 不应在“已登录”状态下允许", m.Type)
		}
	}
}
