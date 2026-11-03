package auth

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	PasswordMin = 6
	PasswordMax = 64
)

var accountPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{2,14}$`)

// NormalizeAccount 把账号转成小写：账号不区分大小写，保存和比较都用小写。
func NormalizeAccount(account string) string { return strings.ToLower(account) }

// ValidAccount 判断（已转小写的）账号是否合法：3–15 个字符，小写字母开头，只含小写字母、数字、下划线。
func ValidAccount(account string) bool { return accountPattern.MatchString(account) }

// ValidPassword 判断密码是否合法：6–64 个字符，不能有首尾空格。
func ValidPassword(password string) bool {
	n := utf8.RuneCountInString(password)
	return n >= PasswordMin && n <= PasswordMax && strings.TrimSpace(password) == password
}

// auditName 截断过长的账号名再写审计，避免恶意输入撑大表。
func auditName(account string) string {
	r := []rune(account)
	if len(r) > 64 {
		r = r[:64]
	}
	return strings.ReplaceAll(strings.ToValidUTF8(string(r), "?"), "\x00", "")
}
