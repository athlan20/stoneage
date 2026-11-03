package gamedata

// 任务表（STAGE-86，docs/protocol.md 5.44）：任务编号、名字，以及原版的完成/进行中事件号（只作对照）。
// 数据由 tools/gen-quests.py 从原版服务端 gmsv/data/jobdaily.txt 生成，名字已转 UTF-8。

import (
	"bufio"
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

//go:embed quests.tsv
var questsTSV string

// questColumns 是 TSV 的列数，与 tools/gen-quests.py 的输出一致。
const questColumns = 4

// Quest 是一个任务模板。服务端只按 No 记录进度（进行中或已完成），事件号不参与判断。
type Quest struct {
	No       int
	Name     string
	EndEvent int // 原版完成事件号（对照用）
	NowEvent int // 原版进行中事件号（对照用）
}

var (
	questOnce sync.Once
	questTab  map[int]Quest
	questList []Quest // 按任务编号升序
)

// QuestByNo 按任务编号取模板；不认识的编号返回 false。
func QuestByNo(no int) (Quest, bool) {
	questOnce.Do(loadQuests)
	q, ok := questTab[no]
	return q, ok
}

// Quests 返回全部任务，按编号升序。返回的是内部切片，调用方不要修改。
func Quests() []Quest {
	questOnce.Do(loadQuests)
	return questList
}

// loadQuests 解析 quests.tsv。列顺序见 tools/gen-quests.py：任务编号、任务名、完成事件号、进行中事件号。
func loadQuests() {
	questTab = make(map[int]Quest, 128)
	sc := bufio.NewScanner(strings.NewReader(questsTSV))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != questColumns {
			continue
		}
		num := func(i int) int {
			v, _ := strconv.Atoi(f[i])
			return v
		}
		q := Quest{No: num(0), Name: f[1], EndEvent: num(2), NowEvent: num(3)}
		questTab[q.No] = q
		questList = append(questList, q)
	}
}
