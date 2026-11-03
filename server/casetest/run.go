package casetest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/athlan20/stoneage/server/protocol"
)

// RunOptions 是重放用例时的参数。
type RunOptions struct {
	Timeout time.Duration // 等待服务端一条消息的时限，默认 5 秒
	Log     func(format string, args ...any)
	// Admin 执行用例里的 admin 行（如 do=disable_account、unlock_account、give_item、give_pet）。
	// 没有提供时遇到 admin 行会报错。
	Admin func(Step) error
	// Record 为 true 时不比对 s2c 期望，直接把服务端的真实回复记到 c.Steps[i].Msg，
	// 供调用方写回用例文件（校准用例：go test ./server/casetest -run TestProtocolCases -update-cases）。
	Record bool
}

type conn struct {
	c net.Conn
	r *bufio.Reader
}

// Run 对 addr 上的服务端重放这个用例，返回第一处不符。每个用例都应在空数据库上运行，
// 由调用方负责在用例之间重置数据。
func (c *Case) Run(addr string, opts RunOptions) error {
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Second
	}
	logf := opts.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	vars := map[string]any{} // s2c 里用 "*>名字" 捕获的值，c2s 里用 "$名字" 引用
	conns := map[string]*conn{}
	defer func() {
		for _, cn := range conns {
			cn.c.Close()
		}
	}()
	get := func(name string) (*conn, error) {
		if cn, ok := conns[name]; ok {
			return cn, nil
		}
		nc, err := net.DialTimeout("tcp", addr, opts.Timeout)
		if err != nil {
			return nil, fmt.Errorf("连接 %s 失败: %w", addr, err)
		}
		cn := &conn{c: nc, r: bufio.NewReaderSize(nc, 1<<20)}
		conns[name] = cn
		return cn, nil
	}

	for i := range c.Steps {
		st := &c.Steps[i]
		where := fmt.Sprintf("%s:%d [%s]", c.Name, st.Line, st.Conn)
		if st.Dir == "admin" {
			if opts.Admin == nil {
				return fmt.Errorf("%s: 用例需要管理员操作 %s，但没有提供", where, st.Do)
			}
			if err := opts.Admin(*st); err != nil {
				return fmt.Errorf("%s: 管理员操作 %s %s%s 失败: %w", where, st.Do, st.Account, st.Character, err)
			}
			logf("%s admin %s %s %s", where, st.Do, st.Account, st.Character)
			continue
		}
		cn, err := get(st.Conn)
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		switch st.Dir {
		case "close": // 客户端直接断开这条连接（不发 leave_world）；同名连接之后再次出现会重新拨号
			if err := cn.c.Close(); err != nil {
				return fmt.Errorf("%s: 关闭连接失败: %w", where, err)
			}
			delete(conns, st.Conn)
			logf("%s ✂ 连接已断开", where)
			continue
		case "c2s":
			if st.DelayMs > 0 {
				time.Sleep(time.Duration(st.DelayMs) * time.Millisecond)
			}
			var line []byte
			switch {
			case st.RawLen > 0:
				line = append(bytes.Repeat([]byte("a"), st.RawLen), '\n')
			case st.Raw != "":
				line = []byte(st.Raw + "\n")
			default:
				msg := st.Msg
				if bytes.Contains(msg, []byte(`"$`)) {
					var v any
					if err := json.Unmarshal(msg, &v); err != nil {
						return fmt.Errorf("%s: 用例里的 msg 不是 JSON: %w", where, err)
					}
					if v, err = Substitute(v, vars); err != nil {
						return fmt.Errorf("%s: %w", where, err)
					}
					if msg, err = json.Marshal(v); err != nil {
						return err
					}
				}
				line = append(append([]byte{}, msg...), '\n')
			}
			_ = cn.c.SetWriteDeadline(time.Now().Add(opts.Timeout))
			if _, err := cn.c.Write(line); err != nil {
				return fmt.Errorf("%s: 发送失败: %w", where, err)
			}
			logf("%s → %s", where, summarize(line, true))
		case "s2c":
			timeout := opts.Timeout + time.Duration(st.DelayMs)*time.Millisecond
			_ = cn.c.SetReadDeadline(time.Now().Add(timeout))
			line, err := cn.r.ReadBytes('\n')
			if st.Closed {
				if err == nil {
					return fmt.Errorf("%s: 期望服务端关闭连接，却收到 %s", where, summarize(line, true))
				}
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					return fmt.Errorf("%s: 期望服务端关闭连接，%v 内仍然打开", where, timeout)
				}
				logf("%s ← (连接已关闭)", where)
				continue
			}
			if err != nil {
				return fmt.Errorf("%s: 等待服务端消息失败: %w", where, readErr(err))
			}
			logf("%s ← %s", where, summarize(line, true))
			// STAGE-246：压缩行先还原；线上是不是压缩行要和用例的 z 字段一致，漏压或多压都算错。
			plain, perr := protocol.DecodeLine(line)
			if perr != nil {
				return fmt.Errorf("%s: 服务端压缩行无法解压: %v", where, perr)
			}
			if compressed := !bytes.Equal(plain, line); compressed != (st.Z != "") {
				return fmt.Errorf("%s: 用例期望压缩=%q，服务端实际压缩=%t", where, st.Z, compressed)
			}
			var want, got any
			if err := json.Unmarshal(st.Msg, &want); err != nil {
				return fmt.Errorf("%s: 用例里的 msg 不是 JSON: %w", where, err)
			}
			if err := json.Unmarshal(plain, &got); err != nil {
				return fmt.Errorf("%s: 服务端发来的不是 JSON: %q", where, summarize(line, true))
			}
			if opts.Record {
				// 校准也保留通配捕获：旧期望里的 "*>名字" 从真实回复里取值，
				// 之后 c2s 的 "$名字" 才能继续替换（记录下来的行再手工把捕获写回去）。
				_ = MatchCapture(want, got, "msg", vars)
				encoded, err := json.Marshal(got)
				if err != nil {
					return fmt.Errorf("%s: 记录服务端回复失败: %w", where, err)
				}
				st.Msg = encoded
				continue
			}
			if err := MatchCapture(want, got, "msg", vars); err != nil {
				return fmt.Errorf("%s: %w\n  服务端回复: %s", where, err, summarize(line, true))
			}
		}
	}
	return nil
}

func readErr(err error) error {
	var ne net.Error
	switch {
	case errors.Is(err, io.EOF):
		return errors.New("服务端关闭了连接")
	case errors.As(err, &ne) && ne.Timeout():
		return errors.New("等待超时")
	}
	return err
}

// summarize 输出一行的摘要；redact 为 true 时先把密码和令牌换成 ***，避免明文进入日志。
func summarize(b []byte, redact bool) string {
	s := strings.TrimSpace(string(b))
	if redact {
		s = protocol.Redact(b)
	}
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
