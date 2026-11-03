package wordfilter

// STAGE-287：Aho-Corasick 自动机。词表构建一次，之后每次匹配的耗时只和文本长度有关，和词数无关。

// automaton 按字符（rune）建的字典树加失败指针。
type automaton struct {
	next []map[rune]int32
	fail []int32
	lens [][]int32 // 以该节点结尾的词长，已并入失败链上的词，匹配时直接取
	end  []bool    // 该节点本身是一个词的结尾（去重、计数用）
}

func newAutomaton() *automaton {
	a := &automaton{}
	a.addNode()
	return a
}

func (a *automaton) addNode() int32 {
	a.next = append(a.next, nil)
	a.fail = append(a.fail, 0)
	a.lens = append(a.lens, nil)
	a.end = append(a.end, false)
	return int32(len(a.next) - 1)
}

// add 插入一个词；返回 false 表示这个词已经有了。
func (a *automaton) add(word []rune) bool {
	node := int32(0)
	for _, r := range word {
		child, ok := a.next[node][r]
		if !ok {
			child = a.addNode()
			if a.next[node] == nil {
				a.next[node] = map[rune]int32{}
			}
			a.next[node][r] = child
		}
		node = child
	}
	if a.end[node] {
		return false
	}
	a.end[node] = true
	a.lens[node] = append(a.lens[node], int32(len(word)))
	return true
}

// build 按层算失败指针，并把失败链上的词长并进每个节点。词全部加完后调用一次。
func (a *automaton) build() {
	queue := make([]int32, 0, len(a.next))
	for _, child := range a.next[0] {
		queue = append(queue, child)
	}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for r, child := range a.next[node] {
			f := a.fail[node]
			for f != 0 {
				if _, ok := a.next[f][r]; ok {
					break
				}
				f = a.fail[f]
			}
			if t, ok := a.next[f][r]; ok && t != child {
				a.fail[child] = t
			}
			a.lens[child] = append(a.lens[child], a.lens[a.fail[child]]...)
			queue = append(queue, child)
		}
	}
}

// find 在 text 里找出所有词，对每处命中调用 hit(start, end)（end 含）。重叠的命中也都报告。
func (a *automaton) find(text []rune, hit func(start, end int)) {
	node := int32(0)
	for i, r := range text {
		for {
			if t, ok := a.next[node][r]; ok {
				node = t
				break
			}
			if node == 0 {
				break
			}
			node = a.fail[node]
		}
		for _, n := range a.lens[node] {
			hit(i-int(n)+1, i)
		}
	}
}
