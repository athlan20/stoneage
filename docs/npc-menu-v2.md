# NPC 菜单协议（STAGE-135）

NPC 菜单 v2 通过独立 `npc_open` 请求启用，旧 `talk` 与旧买卖语义保留。旧服务端对未知请求明确拒绝，新客户端不能回退到立即成交。

`npc_open {npc}` 与 talk 校验一致，响应为空，窗口包含 `window_id`（每次推送更新的不可复用标识）、`stage`、`options`、可选 `input` 和 `trade`。所有后续动作必须携带对应 `window_id`；过期或被替换返回 WRONG_STATE。

`npc_choose {no,window_id}`、`npc_reply {text,window_id}` 保留既有语义；菜单 v2 商店选商品只进入数量，回复数量只产生报价。`npc_confirm {window_id,confirm}` 仅用于报价：false 返回商品列表，true 核验并原子成交。报价 60 秒有效，单次消费，重复确认不成交。

`input` 为 `{kind:"text"|"integer",min,max,max_chars}`。暗号不去空格；整数必须在上下界内。选项附商品 `{item_id,slot,unit_price,owned,max_count}`，不解析 label 或正文。`trade` 为 `{direction,item_id,slot,name,unit_price,count,total}`；确认默认取消。部分出售保留剩余数量；买入空间不足按空位截断（报价时 `trade.count` 就是截断后的数，STAGE-192），一格都没有才回 `BAG_FULL`。确认时重验位置、控制权、战斗、物品快照、金币和背包，失败不改物品或金币。

普通 NPC、护士、存档、宠物店、传送员和任务窗口保留原规则与服务端编号。关闭仍发送 npc_close，服务端推送后才移除。新旧连接共享角色窗口；v2 窗口拒绝无 window_id 的变更请求。

`npc_back {window_id}` 由服务端返回商店上层：数量→商品，商品→买卖菜单，菜单→关闭；报价取消回商品列表。其他 NPC 返回等同关闭。新客户端的 talk 命令、场景、目录与跨图对话统一发送 npc_open，不回退到旧立即成交接口；老服务端会明确报告未知请求。
