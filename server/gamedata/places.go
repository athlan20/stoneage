package gamedata

// Place 是 warp 可以传送到的地点（docs/protocol.md 5.15、5.16）。ID 从 1 开始，Floor/X/Y 是传送后角色所在的位置。
type Place struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Floor int    `json:"floor"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
}

// Places 是地点表（STAGE-40 起分两组）：
//   - 1–4 四个出生村，站在村里（村内地图没有遇敌区域，encounter on 会提示“这里没有敌人”）；
//   - 5–8 各村附近的初级野外练级点，站在大岛上的遇敌区域里（encount.txt 的区域 21/28/88/95，
//     敌人 1–4 级，最多 2–4 只），打开遇敌开关就能一场接一场地打。
//
// 村内坐标是 mapwarp.txt 里从大岛走进村后站的位置；野外坐标取在对应遇敌区域内、村门口外几步。
// 追加地点时在末尾加一行，编号顺延；客户端按 floor/x/y 在世界图上换算位置（村内地图标在村的入口上）。
var Places = []Place{
	{1, "萨姆吉尔村", 1000, 50, 116},
	{2, "玛丽娜丝渔村", 2000, 72, 33},
	{3, "加加村", 3000, 74, 54},
	{4, "卡鲁它那村", 4000, 100, 96},
	{5, "萨姆吉尔近郊", 100, 600, 550},
	{6, "玛丽娜丝近郊", 100, 30, 580},
	{7, "加加村近郊", 200, 760, 445},
	{8, "卡鲁它那近郊", 200, 275, 575},
}

// PlaceByID 按编号查地点。
func PlaceByID(id int) (Place, bool) {
	for _, p := range Places {
		if p.ID == id {
			return p, true
		}
	}
	return Place{}, false
}
