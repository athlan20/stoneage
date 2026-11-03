// STAGE-55：管理口没有鉴权，监听地址必须是本机回环；非回环地址在启动时就要被拒绝，
// 而不是等运维发现它开在了 0.0.0.0 上。

package app

import "testing"

func TestCheckLoopbackAddr(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:7066", "[::1]:7066", "localhost:7066"} {
		if err := checkLoopbackAddr(addr); err != nil {
			t.Errorf("%s 应允许（回环地址）: %v", addr, err)
		}
	}
	for _, addr := range []string{
		"0.0.0.0:7066",        // 所有网卡
		":7066",               // 没写主机，同样监听所有网卡
		"192.168.1.10:7066",   // 局域网地址
		"example.com:7066",    // 域名
		"[::]:7066",           // IPv6 所有网卡
		"127.0.0.1",           // 缺端口
		"127.0.0.1::notaport", // 端口不合法
	} {
		if err := checkLoopbackAddr(addr); err == nil {
			t.Errorf("%s 应被拒绝（不是回环地址或地址不合法）", addr)
		}
	}
}

// STAGE-159：管理命令里的 panic 只让这条命令返回错误，不能带走整个进程。
func TestAdminDispatchRecoversPanic(t *testing.T) {
	a := &adminServer{} // svcs 为空，start_battle 会空指针 panic
	if err := a.dispatch(adminRequest{Do: "start_battle", Character: "x"}); err == nil {
		t.Fatal("panic 应转成错误返回")
	}
}
