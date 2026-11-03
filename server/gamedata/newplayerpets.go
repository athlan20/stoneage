package gamedata

// 新建角色出生时赠送的宠物（STAGE-62）。规则出处：gmsv/char/char.c 的 CHAR_createNewChar
// （_NEW_PLAYER_CF 分支，给 0 号栏 setNewplayergivepet(0, hometown+1)）与 gmsv/char/char_data.c
// 的 CHAR_PlayerRide（MetamoList 表，按身体图像编号查配套虎和雷龙）。
//
// 配置（gmsv/setup.cf）：NPRIDE=3、PETLV=1、PET1=101152、PET2=20004。原版里 PET1/PET2 在
// enemy.txt 查不到（ENEMY_getEnemyArrayFromId 失败后跳过），所以虎和雷龙落在 1、2 号栏
// （出生地宠物占 0 号栏），一共 3 只。下面都是旧 enemybase.txt 的模板编号（E_T_TEMPNO）。

// startPetByHometown 是各出生地送的第一只宠物的模板编号，顺序同 Hometowns：
// 萨姆吉尔送乌力乌力、玛丽娜丝送凯比、加加村送克克尔、卡鲁它那送威伯。
// 出处：enemy.txt 里 ENEMY_ID 为 1–4 的“初期拥有”行，其 ENEMY_TEMPNO 分别是 2、112、102、34。
var startPetByHometown = [...]int{2, 112, 102, 34}

// ShopPetIDs 是宠物店的在售清单（STAGE-73）：出生宠物同款的三只（乌力乌力、格鲁西斯虎、
// 布洛多斯雷龙）。原版宠物店不卖宠物（只有卖/寄放/领取），这是新功能的占位数据——
// 商品清单与定价待产品确定后换掉（价格公式见 character/petshop.go 的 petBuyCost）。
var ShopPetIDs = [...]int{2, 193, 252}

// NewPlayerPetIDs 返回新角色出生时按顺序赠送的宠物模板编号：出生地宠物、配套虎、配套雷龙，
// 依此放进 0、1、2 号栏（docs/protocol.md 5.6）。hometown 是出生地 0–3，body、color 是外形与颜色。
// 参数越界或查不到对应项时返回 nil（调用方已校验范围，正常不会发生）。
func NewPlayerPetIDs(hometown, body, color int) []int {
	if hometown < 0 || hometown >= len(startPetByHometown) ||
		body < 0 || body >= BodyCount || color < 0 || color >= ColorCount {
		return nil
	}
	tiger, dragon, ok := RidePair(body, color)
	if !ok {
		return nil
	}
	return []int{startPetByHometown[hometown], tiger, dragon}
}
