// STAGE-241：批量发放的选人。三种人群：角色名清单、账号清单（展开为账号下所有角色）、全部角色（可按注册日期、等级过滤）。
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// parseNameList 解析清单文本：每行一个名字，去掉首尾空白，跳过空行和以 # 开头的注释行，
// 重复的只留第一个，保持原来的顺序。
func parseNameList(text string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, line := range strings.Split(text, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasPrefix(name, "#") || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func readNameList(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读清单文件 %s: %w", path, err)
	}
	return parseNameList(string(data)), nil
}

// selectGrantNames 按命令行选出要发的角色名，返回名字和需要人看的警告（如账号不存在）。
// 名字不在库里不在这里过滤：那是执行时的“找不到角色”，会记进记录文件。
func selectGrantNames(ctx context.Context, pool *pgxpool.Pool, f grantFlags) ([]string, []string, error) {
	switch {
	case f.namesFile != "":
		names, err := readNameList(f.namesFile)
		return names, nil, err
	case f.accountsFile != "":
		accounts, err := readNameList(f.accountsFile)
		if err != nil {
			return nil, nil, err
		}
		return selectByAccounts(ctx, pool, accounts)
	default:
		return selectAllCharacters(ctx, pool, f)
	}
}

// selectByAccounts 把账号清单展开为这些账号下的所有角色。账号名按库里的小写规则比较。
func selectByAccounts(ctx context.Context, pool *pgxpool.Pool, accounts []string) ([]string, []string, error) {
	lower := make([]string, len(accounts))
	for i, a := range accounts {
		lower[i] = strings.ToLower(a)
	}
	rows, err := pool.Query(ctx,
		`SELECT a.name, c.name FROM accounts a LEFT JOIN characters c ON c.account_id = a.id
		 WHERE a.name = ANY($1) ORDER BY a.name, c.slot`, lower)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	found := make(map[string]bool)
	withChars := make(map[string]bool)
	seenChar := make(map[string]bool)
	var names []string
	for rows.Next() {
		var account string
		var char *string
		if err := rows.Scan(&account, &char); err != nil {
			return nil, nil, err
		}
		found[account] = true
		if char == nil {
			continue
		}
		withChars[account] = true
		if !seenChar[*char] {
			seenChar[*char] = true
			names = append(names, *char)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	var warnings []string
	for i, a := range accounts {
		switch {
		case !found[lower[i]]:
			warnings = append(warnings, fmt.Sprintf("账号 %s 不存在，已跳过", a))
		case !withChars[lower[i]]:
			warnings = append(warnings, fmt.Sprintf("账号 %s 下没有角色，已跳过", a))
		}
	}
	return names, warnings, nil
}

// selectAllCharacters 选全部角色。registered-before 比较的是账号的注册时间（accounts.created_at），
// 早于该日 0 点（本机时区）才算；min-level 是角色等级不低于 N。两个过滤都不给时选中全部角色。
func selectAllCharacters(ctx context.Context, pool *pgxpool.Pool, f grantFlags) ([]string, []string, error) {
	var before *time.Time
	if f.registeredBefore != "" {
		t, err := time.ParseInLocation("2006-01-02", f.registeredBefore, time.Local)
		if err != nil {
			return nil, nil, fmt.Errorf("--registered-before 要写成 YYYY-MM-DD: %w", err)
		}
		before = &t
	}
	var minLevel *int
	if f.set["min-level"] {
		minLevel = &f.minLevel
	}
	rows, err := pool.Query(ctx,
		`SELECT c.name FROM characters c JOIN accounts a ON a.id = c.account_id
		 WHERE ($1::timestamptz IS NULL OR a.created_at < $1)
		   AND ($2::integer IS NULL OR c.level >= $2)
		 ORDER BY c.id`, before, minLevel)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, nil, err
		}
		names = append(names, name)
	}
	return names, nil, rows.Err()
}
