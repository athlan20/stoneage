package store

import (
	"context"
	"strings"
	"testing"
)

// 四圣石卡关修复（0035）：44/46/48 已完成而 43/45/47 没记录的，补成进行中；已有记录、任务 41 已完成的不动；重复执行无副作用。
// 用不带外键的临时表跑迁移里的 SQL，不用造完整角色。
func TestStoneQuestEntryRepair(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	ms, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, m := range ms {
		if strings.HasSuffix(m.Name, "fix_stone_quest_entry") {
			sql = m.SQL
		}
	}
	if sql == "" {
		t.Fatal("找不到 fix_stone_quest_entry 迁移")
	}
	conn, err := s.Pool.Acquire(ctx) // 临时表只在同一条连接上可见
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	exec := func(q string) {
		t.Helper()
		if _, err := conn.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`CREATE TEMP TABLE character_quests (character_id bigint, quest_no int, state smallint,
		updated_at timestamptz DEFAULT now(), PRIMARY KEY (character_id, quest_no))`)
	exec(`INSERT INTO character_quests (character_id, quest_no, state) VALUES
		(1,41,1),(1,44,2),
		(2,41,1),(2,44,2),(2,43,1),
		(3,41,1),(3,43,2),(3,44,2),(3,46,2),(3,48,2),
		(4,41,2),(4,44,2),
		(5,41,1)`)
	exec(sql)
	exec(sql) // 幂等
	rows, err := conn.Query(ctx, `SELECT character_id, quest_no, state FROM character_quests WHERE quest_no IN (43,45,47) ORDER BY 1,2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[[2]int]int{}
	for rows.Next() {
		var c, q, st int
		if err := rows.Scan(&c, &q, &st); err != nil {
			t.Fatal(err)
		}
		got[[2]int{c, q}] = st
	}
	want := map[[2]int]int{{1, 43}: 1, {2, 43}: 1, {3, 43}: 2, {3, 45}: 1, {3, 47}: 1}
	if len(got) != len(want) {
		t.Fatalf("补出的记录 = %v，期望 %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("角色 %d 任务 %d 状态 = %d，期望 %d", k[0], k[1], got[k], v)
		}
	}
}
