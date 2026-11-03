package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadInlayOverlay 检查启动时的精工覆盖表：没配能过，指定的文件读不出来或编号不存在都要报错（STAGE-297）。
func TestLoadInlayOverlay(t *testing.T) {
	if err := loadInlayOverlay(""); err != nil {
		t.Fatalf("没配覆盖表应能启动: %v", err)
	}
	if err := loadInlayOverlay(filepath.Join(t.TempDir(), "missing.tsv")); err == nil {
		t.Error("文件不存在应报错")
	}
	dir := t.TempDir()
	good := filepath.Join(dir, "good.tsv")
	bad := filepath.Join(dir, "bad.tsv")
	if err := os.WriteFile(good, []byte("物品编号\t类别码\t初始镶嵌码\n0\tAXE\tRUBY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("999999\tRUBY\t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := loadInlayOverlay(good); err != nil {
		t.Errorf("编号都存在应能加载: %v", err)
	}
	if err := loadInlayOverlay(bad); err == nil {
		t.Error("编号不存在应报错，启动失败")
	}
}
