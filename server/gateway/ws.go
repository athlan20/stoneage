package gateway

// WebSocket 入口（STAGE-101）：和裸 TCP 共用同一套会话、协议和守卫，只是底层换成 WebSocket，
// 一行 JSON 对应一个文本帧（docs/protocol.md 第 1 节）。路径固定为 /ws，供 Cloudflare 等前置代理转发。

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// WSPath 是 WebSocket 入口的路径。
const WSPath = "/ws"

// CloudflareIPHeader 是 Cloudflare 填入的玩家真实 IP 头。只有直连地址在可信代理里时才采信。
const CloudflareIPHeader = "CF-Connecting-IP"

// ServeWS 在 l 上提供 WebSocket 入口，直到 l 被关闭或服务关闭。
// 它和 Serve 共用连接登记与 Shutdown，所以关服时两个入口的连接一起通知、一起断开。
func (s *Server) ServeWS(l net.Listener) error {
	mux := http.NewServeMux()
	mux.HandleFunc(WSPath, s.serveWS)
	for _, r := range s.routes {
		mux.Handle(r.pattern, r.handler)
	}
	hs := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		_ = l.Close()
		return nil
	}
	s.wsHTTP = hs
	s.mu.Unlock()
	if err := hs.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		// Accept 已经把握手失败的响应写回去了，这里只记日志。
		s.log.Debug("WebSocket 握手失败", "err", err, "remote", r.RemoteAddr)
		return
	}
	remote := s.wsRemoteAddr(r)
	conn := newWSConn(c, remote, s.opts.MaxLine, s.opts.WriteTimeout)
	if !s.admit(conn, remote) {
		return
	}
	go conn.keepAlive(s.opts.WSPingInterval)
}

// wsRemoteAddr 取玩家的地址。直连地址在可信代理里、且带了 CF-Connecting-IP 时用那个 IP，
// 否则用直连地址。限流等按这个地址计算，所以经过 Cloudflare 后不会所有玩家共用一个桶。
func (s *Server) wsRemoteAddr(r *http.Request) net.Addr {
	if ip, ok := s.forwardedIP(r); ok {
		return &net.TCPAddr{IP: ip.AsSlice()}
	}
	if addr, err := net.ResolveTCPAddr("tcp", r.RemoteAddr); err == nil {
		return addr
	}
	return &net.TCPAddr{}
}

// forwardedIP 返回可信代理转来的玩家 IP。直连地址不可信，或头不是合法 IP 时返回 false。
func (s *Server) forwardedIP(r *http.Request) (netip.Addr, bool) {
	if len(s.opts.TrustedProxies) == 0 {
		return netip.Addr{}, false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	peer = peer.Unmap()
	trusted := false
	for _, p := range s.opts.TrustedProxies {
		if p.Contains(peer) {
			trusted = true
			break
		}
	}
	if !trusted {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get(CloudflareIPHeader)))
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}
