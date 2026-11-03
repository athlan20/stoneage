// STAGE-241：批量发放的记录文件（runtime/grants/<批次号>.jsonl，每行一个角色的一次尝试）。
// 记录只追加不改写；同一个角色有多行时以最后一行为准，所以“失败后重试成功”会留下两行。
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// grantRecord 是记录文件的一行。qty 比任务书多记了一个字段：重试时要用同样的数量，
// 不然同一批次号可能被悄悄改成另一个数量。
type grantRecord struct {
	Batch     string `json:"batch"`
	Character string `json:"character"`
	Kind      string `json:"kind"`
	ID        int    `json:"id"`
	Qty       int    `json:"qty"`
	Status    string `json:"status"` // ok | failed
	Error     string `json:"error"`
	Note      string `json:"note,omitempty"` // 发成功但有说明的（石币封顶：应发、实发、上限）
	Time      string `json:"time"`
}

var batchNameRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// validBatchName 只允许字母、数字、-、_，因为批次号要直接当文件名用。
func validBatchName(s string) bool {
	return batchNameRE.MatchString(s)
}

// readGrantRecords 读出记录文件的全部行；文件不存在时返回空，不算错误。
func readGrantRecords(path string) ([]grantRecord, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var recs []grantRecord
	for i, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var r grantRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("%s 第 %d 行不是合法的 JSON，先人工看一下再继续: %w", path, i+1, err)
		}
		recs = append(recs, r)
	}
	return recs, nil
}

// appendGrantRecord 追加一行。调用方在每个角色的事务结束后调用，失败就停止整批，
// 否则下次重跑会重复发放。
func appendGrantRecord(path string, r grantRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// latestStatus 返回每个角色最后一行的状态。
func latestStatus(recs []grantRecord) map[string]string {
	status := make(map[string]string, len(recs))
	for _, r := range recs {
		status[r.Character] = r.Status
	}
	return status
}

// checkRecordSpec 确认记录文件里每一行的物资种类、编号、数量都和这次的一样。
// 不一样就拒绝执行，防止同一个批次号混进别的物资。
func checkRecordSpec(recs []grantRecord, spec grantSpec) error {
	for _, r := range recs {
		if r.Kind != spec.Kind || r.ID != spec.ID || r.Qty != spec.Qty {
			return fmt.Errorf("批次号已经用于 %s %d（数量 %d），和这次的 %s %d（数量 %d）对不上，换一个批次号",
				r.Kind, r.ID, r.Qty, spec.Kind, spec.ID, spec.Qty)
		}
	}
	return nil
}

// specFromRecords 从记录文件里取出这个批次的物资，给 --retry 用。记录为空时返回 false。
func specFromRecords(recs []grantRecord) (grantSpec, bool) {
	if len(recs) == 0 {
		return grantSpec{}, false
	}
	return grantSpec{Kind: recs[0].Kind, ID: recs[0].ID, Qty: recs[0].Qty}, true
}

// failedNames 按第一次出现的顺序返回最后状态为失败的角色名。
func failedNames(recs []grantRecord) []string {
	status := latestStatus(recs)
	seen := make(map[string]bool)
	var names []string
	for _, r := range recs {
		if seen[r.Character] {
			continue
		}
		seen[r.Character] = true
		if status[r.Character] == "failed" {
			names = append(names, r.Character)
		}
	}
	return names
}

// planGrant 把候选角色分成要执行的和跳过的：记录里已经是 ok 的跳过，其余（没发过或上次失败）都执行。
func planGrant(names []string, status map[string]string) (todo, skipped []string) {
	for _, n := range names {
		if status[n] == "ok" {
			skipped = append(skipped, n)
		} else {
			todo = append(todo, n)
		}
	}
	return todo, skipped
}
