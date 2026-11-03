package gamewiki

// addAgentGuide 收录「Agent」分类（STAGE-118）：玩家让 AI 替自己玩时，先读这句话指向的手册 /agent.md。
// 站点只渲染 https 链接，所以地址写在描述里，不做成链接。
func (b *builder) addAgentGuide() {
	e := b.add("agent", "manual", "让 AI 替你玩", "把令牌和本站的手册地址 /agent.md 一起交给你的 AI：它读完手册，连上游戏后先私聊问你想做什么。")
	e.Notes = []string{"令牌在游戏客户端「系统」页输入 token new 生成，只显示一次；生成后直接交给 AI，不要发到聊天或公开的地方。"}
	e.Sources = []string{"服务端 /agent.md（docs/agent-guide.md）"}
}
