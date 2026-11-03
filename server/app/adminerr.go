package app

import "fmt"

// errUnknownAdmin 是管理口不认识的命令。
func errUnknownAdmin(do string) error {
	return fmt.Errorf("没有叫 %q 的管理命令", do)
}
