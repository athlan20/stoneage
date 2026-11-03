package docs

import "testing"

// STAGE-188：文档修订号只随内容变：同样的内容算出同样的值，改一个字就变，两段文字挪位置也变。
func TestRevOfFollowsContent(t *testing.T) {
	a := RevOf([]byte("协议"), []byte("手册"))
	if b := RevOf([]byte("协议"), []byte("手册")); a != b {
		t.Fatalf("同样的内容应得到同样的修订号：%s != %s", a, b)
	}
	if len(a) != 12 {
		t.Fatalf("修订号应是 12 位十六进制，实际 %q", a)
	}
	if c := RevOf([]byte("协议。"), []byte("手册")); c == a {
		t.Fatal("协议改了一个字，修订号应变化")
	}
	if d := RevOf([]byte("协议手"), []byte("册")); d == a {
		t.Fatal("两段文字的边界挪动，修订号应变化")
	}
}

// 当前嵌入的文档算出的修订号是稳定的：两次调用相同。
func TestEmbeddedRevIsStable(t *testing.T) {
	if Rev() != Rev() {
		t.Fatal("同一次编译里的修订号应相同")
	}
}
