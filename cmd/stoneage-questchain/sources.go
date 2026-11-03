package main

// 脚本没有设置、却被条件引用的事件（模型按外部可得处理，见 model.go 的 extEnd、extNow）的来源说明。
// 来源是在原版服务端 C 源码里逐个查的（gmsv 下 grep EventSetFlg、NowEventSetFlg），路径相对 original-data/gmsv。
// 没查到设置点的写“来源未确认”，不要猜。

// knownSources 按事实（完成或进行中的事件号）给来源。
var knownSources = map[Fact]string{
	{Kind: KindNow, N: 71}:  "宠物转生 NPC 设进行中：npc/npc_transmigration.c 的 NPC_PetTransManStatus（末尾 NPC_NowEventSetFlg 71）",
	{Kind: KindEnd, N: 48}:  "精灵少女转生设完成：npc/npc_transmigration.c 的 NPC_TransmigrationFlg_CLS（flg 数组 48～52）",
	{Kind: KindEnd, N: 49}:  "精灵少女转生设完成：npc/npc_transmigration.c 的 NPC_TransmigrationFlg_CLS（flg 数组 48～52）",
	{Kind: KindEnd, N: 50}:  "精灵少女转生设完成：npc/npc_transmigration.c 的 NPC_TransmigrationFlg_CLS（flg 数组 48～52）",
	{Kind: KindEnd, N: 51}:  "精灵少女转生设完成：npc/npc_transmigration.c 的 NPC_TransmigrationFlg_CLS（flg 数组 48～52）",
	{Kind: KindEnd, N: 52}:  "精灵少女转生设完成：npc/npc_transmigration.c 的 NPC_TransmigrationFlg_CLS（flg 数组 48～52）",
	{Kind: KindEnd, N: 63}:  "精灵少女转生（6 人版）设完成：npc/npc_transmigration.c 的 NPC_TransmigrationFlg_CLS（num == 6）",
	{Kind: KindEnd, N: 136}: "精灵少女转生（6 人版）设完成：npc/npc_transmigration.c 的 NPC_TransmigrationFlg_CLS（num == 6）",
	{Kind: KindEnd, N: 175}: "精灵少女转生（6 人版）设完成：npc/npc_transmigration.c 的 NPC_TransmigrationFlg_CLS（num == 6）",
	{Kind: KindEnd, N: 176}: "精灵少女转生（6 人版）设完成：npc/npc_transmigration.c 的 NPC_TransmigrationFlg_CLS（num == 6）",
	{Kind: KindEnd, N: 179}: "精灵少女转生设完成：npc/npc_transmigration.c 的 NPC_TransmigrationFlg_CLS（_TRANS_6 的 flg 数组）",
	{Kind: KindEnd, N: 134}: "角色登录时的道具修正设完成：char/char.c（NPC_EventSetFlg 134，与任务无关）",
	{Kind: KindEnd, N: 149}: "角色登录时的道具修正设完成：char/char.c（NPC_EventSetFlg 149，与任务无关）",
	{Kind: KindEnd, N: 156}: "角色登录时设完成：char/char.c（NPC_EventSetFlg 156，与任务无关）",
}

// eventSource 返回外部事件的来源说明；没有查到的写“来源未确认”。
func eventSource(f Fact) string {
	if s, ok := knownSources[f]; ok {
		return s
	}
	return "来源未确认：任务脚本没有设置，已查的 C 代码里也没找到设置点"
}
