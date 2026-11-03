// STAGE-241：stoneage-admin grant 批量发放命令：参数解析、预演/正式/重试的编排和汇总。
// 选人在 grant_select.go，记录文件在 grant_record.go，单个角色怎么发在 grant_apply.go。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/athlan20/stoneage/server/gamedata"
)

const grantPreviewLimit = 20

const grantUsage = `用法: stoneage-admin grant --kind item|pet|magic|point|gold --batch <批次号> <数量参数> <人群参数> [--yes]
      stoneage-admin grant --batch <批次号> --retry [--yes]

  --kind item|pet|magic|point|gold  物资类型：物品、宠物、精灵、能力点、石币
  --id <编号>                  物品/宠物/精灵编号（能力点不用）
  --count N                    物品个数，默认 1（只给 item）
  --level N                    宠物等级，默认 1（只给 pet）
  --points N                   能力点数，必填（只给 point）
  --amount N                   石币数量，必填（只给 gold）；超过持有上限的部分不发，记录里写明封顶
  --batch <批次号>             只允许字母、数字、-、_；同一批次号重跑时已成功的跳过
  人群（三选一）：
  --names-file F               角色名清单，每行一个
  --accounts-file F            账号清单，每行一个，展开为这些账号下的所有角色
  --all                        全部角色；可加 --registered-before YYYY-MM-DD（账号注册早于该日）、--min-level N（等级不低于 N）
  --retry                      只重试该批次记录文件里失败的角色，物资沿用记录，不接受其他选人/物资参数
  --yes                        真正写库；不加只预演（打印人数和前 20 个名字，不改库）
  --dir DIR                    发放记录目录，默认 runtime/grants
`

// grantFlags 是 grant 子命令的参数。set 记下命令行上真正给了哪些参数，
// 因为 0 既可能是合法编号（小斧头是 0），也可能是没填。
type grantFlags struct {
	kind, batch, namesFile, accountsFile, registeredBefore, dir string
	id, count, level, points, amount, minLevel                  int
	all, retry, yes                                             bool
	set                                                         map[string]bool
}

func parseGrantArgs(args []string) (grantFlags, error) {
	var f grantFlags
	fs := flag.NewFlagSet("grant", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, grantUsage) }
	fs.StringVar(&f.kind, "kind", "", "物资类型：item|pet|magic|point")
	fs.IntVar(&f.id, "id", 0, "物品/宠物/精灵编号")
	fs.IntVar(&f.count, "count", 0, "物品个数")
	fs.IntVar(&f.level, "level", 0, "宠物等级")
	fs.IntVar(&f.points, "points", 0, "能力点数")
	fs.IntVar(&f.amount, "amount", 0, "石币数量")
	fs.StringVar(&f.batch, "batch", "", "批次号")
	fs.StringVar(&f.namesFile, "names-file", "", "角色名清单文件")
	fs.StringVar(&f.accountsFile, "accounts-file", "", "账号清单文件")
	fs.BoolVar(&f.all, "all", false, "全部角色")
	fs.StringVar(&f.registeredBefore, "registered-before", "", "账号注册早于该日（YYYY-MM-DD）")
	fs.IntVar(&f.minLevel, "min-level", 0, "角色等级不低于 N")
	fs.BoolVar(&f.retry, "retry", false, "只重试记录里失败的角色")
	fs.BoolVar(&f.yes, "yes", false, "真正写库")
	fs.StringVar(&f.dir, "dir", "runtime/grants", "发放记录目录")
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	if fs.NArg() > 0 {
		return f, fmt.Errorf("多余的参数 %q\n\n%s", fs.Args(), grantUsage)
	}
	f.set = make(map[string]bool)
	fs.Visit(func(fl *flag.Flag) { f.set[fl.Name] = true })
	return f, nil
}

// buildGrantSpec 检查参数组合，并给出这次要发的物资（缺省的数量在这里补上）。
func buildGrantSpec(f grantFlags) (grantSpec, error) {
	s := grantSpec{Kind: f.kind}
	if err := checkKindFlags(f); err != nil {
		return s, err
	}
	switch f.kind {
	case "item":
		s.ID, s.Qty = f.id, 1
		if f.set["count"] {
			s.Qty = f.count
		}
	case "pet":
		s.ID, s.Qty = f.id, gamedata.NewPetLevel
		if f.set["level"] {
			s.Qty = f.level
		}
	case "magic":
		s.ID, s.Qty = f.id, 1
	case "point":
		s.Qty = f.points
	case "gold":
		s.Qty = f.amount
	}
	return s, validateGrantSpec(s)
}

// checkKindFlags 要求每种物资必填的参数都给了，并拒绝用不上的参数（免得写成 --kind pet --count 5 还以为生效）。
func checkKindFlags(f grantFlags) error {
	need := map[string][]string{"item": {"id"}, "pet": {"id"}, "magic": {"id"}, "point": {"points"}, "gold": {"amount"}}
	allowed := map[string][]string{
		"item":  {"id", "count"},
		"pet":   {"id", "level"},
		"magic": {"id"},
		"point": {"points"},
		"gold":  {"amount"},
	}
	ok, known := allowed[f.kind]
	if !known {
		return fmt.Errorf("--kind 只能是 item|pet|magic|point|gold，现在是 %q", f.kind)
	}
	for _, n := range need[f.kind] {
		if !f.set[n] {
			return fmt.Errorf("--kind %s 需要 --%s", f.kind, n)
		}
	}
	for _, n := range []string{"id", "count", "level", "points", "amount"} {
		if f.set[n] && !contains(ok, n) {
			return fmt.Errorf("--kind %s 不能用 --%s", f.kind, n)
		}
	}
	return nil
}

// checkSourceFlags 检查人群参数：正式发放要且只能选一种人群；--retry 不能和选人、物资参数同用。
func checkSourceFlags(f grantFlags) error {
	if f.retry {
		for _, n := range []string{"kind", "id", "count", "level", "points", "names-file", "accounts-file", "all", "registered-before", "min-level"} {
			if f.set[n] {
				return fmt.Errorf("--retry 只按记录文件重试，不能和 --%s 同用", n)
			}
		}
		return nil
	}
	n := 0
	for _, src := range []string{"names-file", "accounts-file", "all"} {
		if f.set[src] {
			n++
		}
	}
	if n != 1 {
		return errors.New("人群参数要三选一：--names-file、--accounts-file 或 --all")
	}
	if !f.all && (f.set["registered-before"] || f.set["min-level"]) {
		return errors.New("--registered-before 和 --min-level 只能配合 --all 用")
	}
	if f.set["registered-before"] {
		if _, err := time.ParseInLocation("2006-01-02", f.registeredBefore, time.Local); err != nil {
			return fmt.Errorf("--registered-before 要写成 YYYY-MM-DD: %w", err)
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// runGrant 是 stoneage-admin grant 的入口。没有 --yes 只预演，不写库也不写记录。
func runGrant(ctx context.Context, pool *pgxpool.Pool, args []string) error {
	f, err := parseGrantArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if !validBatchName(f.batch) {
		return errors.New("--batch 必填，只允许字母、数字、-、_，最长 64 个字符")
	}
	if err := checkSourceFlags(f); err != nil {
		return err
	}
	path := filepath.Join(f.dir, f.batch+".jsonl")
	prev, err := readGrantRecords(path)
	if err != nil {
		return err
	}

	var spec grantSpec
	var names, warnings []string
	if f.retry {
		var ok bool
		if spec, ok = specFromRecords(prev); !ok {
			return fmt.Errorf("批次 %s 没有发放记录（%s），没有可重试的", f.batch, path)
		}
		if err := checkRecordSpec(prev, spec); err != nil {
			return err
		}
		names = failedNames(prev)
	} else {
		if spec, err = buildGrantSpec(f); err != nil {
			return err
		}
		if err := checkRecordSpec(prev, spec); err != nil {
			return err
		}
		if names, warnings, err = selectGrantNames(ctx, pool, f); err != nil {
			return err
		}
	}
	todo, skipped := planGrant(names, latestStatus(prev))

	if !f.yes {
		printGrantPreview(f.batch, spec, names, todo, skipped, warnings, path)
		return nil
	}
	return execGrant(ctx, pool, f.batch, spec, path, todo, skipped, warnings)
}

func printGrantPreview(batch string, spec grantSpec, names, todo, skipped, warnings []string, path string) {
	fmt.Printf("【预演，不改库】批次 %s：%s，共 %d 个角色\n", batch, spec.describe(), len(names))
	fmt.Printf("  将发放 %d 个，已成功跳过 %d 个（记录文件 %s）\n", len(todo), len(skipped), path)
	if len(todo) > 0 {
		show := todo
		if len(show) > grantPreviewLimit {
			show = show[:grantPreviewLimit]
		}
		fmt.Printf("  前 %d 个：%s\n", len(show), strings.Join(show, "、"))
	}
	for _, w := range warnings {
		fmt.Println("  注意：" + w)
	}
	if len(todo) > 0 {
		fmt.Println("确认无误后加 --yes 正式发放。")
	}
}

// execGrant 逐个角色发放，每个角色独立成功或失败，失败的记下原因继续下一个。
// 每个角色的结果写进记录文件后才处理下一个；记录写不进去就停下，免得重跑时重复发放。
func execGrant(ctx context.Context, pool *pgxpool.Pool, batch string, spec grantSpec, path string,
	todo, skipped, warnings []string) error {
	for _, w := range warnings {
		fmt.Println("注意：" + w)
	}
	rnd := newGrantRand(time.Now().UnixNano())
	var okN int
	var failures []string
	for _, name := range todo {
		rec := grantRecord{Batch: batch, Character: name, Kind: spec.Kind, ID: spec.ID, Qty: spec.Qty,
			Status: "ok", Time: time.Now().Format(time.RFC3339)}
		note, err := applyGrant(ctx, pool, spec, name, rnd)
		rec.Note = note
		if err != nil {
			rec.Status, rec.Error = "failed", err.Error()
			failures = append(failures, name+"："+err.Error())
		} else {
			okN++
		}
		if err := appendGrantRecord(path, rec); err != nil {
			return fmt.Errorf("写发放记录 %s 失败，已处理到 %s，之后的角色没有发，先查清原因再重跑: %w", path, name, err)
		}
	}
	fmt.Printf("批次 %s 发放完成：%s\n", batch, spec.describe())
	fmt.Printf("  成功 %d，失败 %d，跳过（已成功过）%d\n", okN, len(failures), len(skipped))
	fmt.Printf("  记录文件 %s\n", path)
	for _, f := range failures {
		fmt.Println("  失败：" + f)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d 个角色失败（见上面明细）；处理原因后用 --retry 重试同一批次", len(failures))
	}
	return nil
}
