package gamedata

// 新建角色出生时赠送的物品。这是本项目自己加的福利，原版 CHAR_createNewChar 不送物品。

// NewPlayerGift 是出生礼包里的一件物品。
type NewPlayerGift struct {
	ItemID int
	Count  int
}

// NewPlayerGifts 是每个新角色出生时放进背包的物品，按顺序占背包的前几格：
// 马祖的棍棒（2690）：武器，自带恩惠的精灵 Lv3，精灵耗气 0、可装备等级 1（数据见 itemset.tsv）。
var NewPlayerGifts = []NewPlayerGift{
	{ItemID: 2690, Count: 1},
}
