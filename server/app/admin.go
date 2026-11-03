// 本地管理端口（docs/server.md）：协议用例的 admin 行 start_battle 需要让**正在运行
// 的服务端**立刻开一场战斗，直接写库做不到，所以服务端可以另开一个只监听本机的
// JSON Lines 管理口。一条连接一行请求：{"do":"start_battle","character":"名字","scenario":"win"}，
// set_setting 改全服设置（STAGE-59）：{"do":"set_setting","key":"exp_rate","value":2}，
// 回 {"ok":true} 或 {"ok":false,"error":"说明"}。默认不开；冒烟脚本和协议测试客户端
// 按需打开（cmd/stoneage-server -admin-listen）。

package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"time"
)

// adminRequest 是管理口的一行请求。
type adminRequest struct {
	Do        string      `json:"do"`
	Character string      `json:"character"`
	Scenario  string      `json:"scenario"`
	Key       string      `json:"key"`   // set_setting 用：设置名
	Value     json.Number `json:"value"` // set_setting 用：设置值（数字或字符串都收，按原文校验）
}

// adminServer 是管理口的监听。
type adminServer struct {
	svcs *Services
	ln   net.Listener
	done chan struct{}

	mu    sync.Mutex
	conns map[net.Conn]bool
}

// StartAdmin 在 addr（只能监听本机回环地址，如 127.0.0.1:端口）上开管理口；addr 为空返回 nil。
// 管理口没有鉴权，配成对外可达的地址任何人都能强制开战，所以非回环地址直接拒绝（STAGE-55）。
func (s *Services) StartAdmin(addr string) (*adminServer, error) {
	if addr == "" {
		return nil, nil
	}
	if err := checkLoopbackAddr(addr); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	a := &adminServer{svcs: s, ln: ln, done: make(chan struct{}), conns: map[net.Conn]bool{}}
	go a.serve()
	slog.Info("管理口已启动", "listen", ln.Addr().String())
	return a, nil
}

// checkLoopbackAddr 检查监听地址的主机部分是回环地址（127.0.0.1、::1、localhost）。
// 没写主机（如 ":7066"、"0.0.0.0:7066"）等于监听所有网卡，同样拒绝。
func checkLoopbackAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("管理口地址 %q 不合法: %w（应形如 127.0.0.1:7066）", addr, err)
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	return fmt.Errorf("管理口没有鉴权，只能监听本机回环地址（127.0.0.1 / ::1 / localhost），%q 不允许；它只用于本地联调", addr)
}

// Close 关掉管理口。
func (a *adminServer) Close() {
	if a == nil {
		return
	}
	a.ln.Close()
	a.mu.Lock()
	for c := range a.conns {
		c.Close()
	}
	a.mu.Unlock()
	<-a.done
}

func (a *adminServer) serve() {
	defer close(a.done)
	for {
		c, err := a.ln.Accept()
		if err != nil {
			return
		}
		a.mu.Lock()
		a.conns[c] = true
		a.mu.Unlock()
		go a.handle(c)
	}
}

func (a *adminServer) handle(c net.Conn) {
	defer func() {
		a.mu.Lock()
		delete(a.conns, c)
		a.mu.Unlock()
		c.Close()
	}()
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64*1024), 64*1024)
	for sc.Scan() {
		var req adminRequest
		line := []byte("{\"ok\":true}")
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			line = []byte("{\"ok\":false,\"error\":\"请求不是 JSON\"}")
		} else if err := a.dispatch(req); err != nil {
			line = []byte("{\"ok\":false,\"error\":" + mustJSON(err.Error()) + "}")
		}
		if _, werr := c.Write(append(line, '\n')); werr != nil {
			return
		}
	}
}

func (a *adminServer) dispatch(req adminRequest) (err error) {
	// STAGE-159：管理命令里的 panic 只算这条命令失败，不能让整个服务端进程退出。
	defer func() {
		if r := recover(); r != nil {
			slog.Error("管理口命令 panic", "do", req.Do, "panic", r, "stack", string(debug.Stack()))
			err = fmt.Errorf("命令执行出错: %v", r)
		}
	}()
	switch req.Do {
	case "start_battle":
		return a.svcs.Characters.StartBattle(req.Character, req.Scenario)
	case "set_setting":
		// 写库、NOTIFY 并同步重读，回复 ok 时倍数已经在运行中的服务端里生效
		//（协议用例 battle_exp_rate 用，黑盒重放没有通知到达的竞态）。
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		v, err := a.svcs.Characters.SetGameSetting(ctx, req.Key, req.Value.String(), "admin-port")
		if err != nil {
			return err
		}
		slog.Info("管理口改了全服设置", "key", req.Key, "value", v)
		return nil
	}
	return errUnknownAdmin(req.Do)
}

func mustJSON(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}
