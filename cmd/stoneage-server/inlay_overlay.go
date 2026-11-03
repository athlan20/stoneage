package main

import (
	"fmt"
	"os"

	"github.com/athlan20/stoneage/server/gamedata"
)

// loadInlayOverlay 在启动时套上精工覆盖表（STAGE-297，docs/inlay.md 2.4）。path 为空用随程序发布的覆盖表
// （只有表头，不改任何物品）；指定了文件就必须读得进来、编号都存在，否则起不来，免得悄悄没生效。
func loadInlayOverlay(path string) error {
	tsv := gamedata.DefaultInlayOverlay()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取精工覆盖表 %s: %w", path, err)
		}
		tsv = string(b)
	}
	return gamedata.LoadInlayOverlay(tsv)
}
