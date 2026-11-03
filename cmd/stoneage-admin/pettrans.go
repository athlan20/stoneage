package main

import (
	"fmt"
	"strconv"

	"github.com/athlan20/stoneage/server/character"
)

// parsePetTrans 解析 `setpettrans <角色名> <栏位> <次数>` 的参数（不含命令名）。
// STAGE-196：栏位是随身栏位 0–19（页面上的编号，STAGE-346），次数不能是负数；超过 2 次的宠物不能骑（RIDE_TRANS_LIMIT）。
func parsePetTrans(args []string) (name string, slot, trans int, err error) {
	if len(args) != 3 {
		return "", 0, 0, fmt.Errorf("需要角色名、栏位和次数三个参数")
	}
	slot, err = strconv.Atoi(args[1])
	if err != nil {
		return "", 0, 0, fmt.Errorf("栏位要是整数: %w", err)
	}
	if slot < 0 || slot >= character.MaxPetSlots {
		return "", 0, 0, fmt.Errorf("栏位要在 0–%d 之间", character.MaxPetSlots-1)
	}
	trans, err = strconv.Atoi(args[2])
	if err != nil {
		return "", 0, 0, fmt.Errorf("转生次数要是整数: %w", err)
	}
	if trans < 0 {
		return "", 0, 0, fmt.Errorf("转生次数不能是负数")
	}
	return args[0], slot, trans, nil
}
