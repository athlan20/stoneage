package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// HashParams 是 Argon2id 参数。默认值与旧项目 internal/auth 一致。
type HashParams struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
}

// DefaultHashParams 是生产参数：64 MiB 内存、3 轮、2 线程。
var DefaultHashParams = HashParams{MemoryKiB: 64 * 1024, Iterations: 3, Parallelism: 2}

const (
	saltBytes = 16
	keyBytes  = 32
)

// Hash 生成 Argon2id 哈希，编码为 $argon2id$v=19$m=…,t=…,p=…$盐$哈希，自包含、可迁移。
func Hash(password string, p HashParams) (string, error) {
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("生成盐失败: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, p.Iterations, p.MemoryKiB, p.Parallelism, keyBytes)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", p.MemoryKiB, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify 校验密码。哈希格式不对或参数超出合理范围时返回 false。
func Verify(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var mem, iter, par uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iter, &par); err != nil {
		return false
	}
	if mem < 8*1024 || mem > 512*1024 || iter == 0 || iter > 10 || par == 0 || par > 16 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 || len(want) > 64 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iter, mem, uint8(par), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
