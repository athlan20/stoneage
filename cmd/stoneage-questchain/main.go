// stoneage-questchain 评估任务脚本能不能从头走完（STAGE-213，见 docs/quest.md 和 docs/quest-chains.md）。
// 用法：go run ./cmd/stoneage-questchain -o docs/quest-chains.md（不写 -o 输出到标准输出）。
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	out := flag.String("o", "", "输出文件（默认标准输出）")
	flag.Parse()
	md := renderReport(realModel(), realRules())
	if *out == "" {
		fmt.Print(md)
		return
	}
	if err := os.WriteFile(*out, []byte(md), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
