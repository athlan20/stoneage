package main

import "testing"

// STAGE-196：setpettrans 的参数解析（数据库部分由协议用例 pet_ride_trans 覆盖）。
func TestParsePetTrans(t *testing.T) {
	name, slot, trans, err := parsePetTrans([]string{"勇者小明", "1", "3"})
	if err != nil {
		t.Fatalf("合法参数被拒: %v", err)
	}
	if name != "勇者小明" || slot != 1 || trans != 3 {
		t.Fatalf("解析结果不对: %q %d %d", name, slot, trans)
	}

	bad := map[string][]string{
		"参数不够":   {"勇者小明", "1"},
		"参数多了":   {"勇者小明", "1", "3", "4"},
		"栏位不是整数": {"勇者小明", "x", "3"},
		"栏位越界":   {"勇者小明", "5", "3"},
		"栏位是负数":  {"勇者小明", "-1", "3"},
		"次数不是整数": {"勇者小明", "1", "x"},
		"次数是负数":  {"勇者小明", "1", "-1"},
	}
	for what, args := range bad {
		if _, _, _, err := parsePetTrans(args); err == nil {
			t.Errorf("%s：应当报错，实际通过（%v）", what, args)
		}
	}
}
