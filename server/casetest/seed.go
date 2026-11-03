// 按用例名固定的随机种子（STAGE-58）：give_pet 的宠物资质等随机结果要按用例名
// 可复现，命令行重放（cmd/stoneage-casetest）与进程内回归（cases_test.go）必须用
// 同一个种子，黑盒重放才能对上用例里录制的期望值（docs/server.md）。

package casetest

// SeedForCase 返回按用例名固定的随机种子（用例名的 fnv32 散列）。
func SeedForCase(name string) uint64 {
	var h uint32 = 2166136261
	for _, c := range name {
		h = (h ^ uint32(c)) * 16777619
	}
	return uint64(h)
}
