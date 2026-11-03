# NPC 菜单 v2 契约验收

1. npc_open 198 → 带 window_id 的菜单；npc_choose 1 携带该 ID → 商品列表。
2. 选商品 → integer 输入（1–15）；reply 2 → 报价 count=2、total=unit_price×2，物品金币不变。
3. npc_confirm false → 商品列表，无成交；旧 ID confirm true → WRONG_STATE。
4. 新报价 confirm true → 整笔成交，刷新 inventory/status；重复 ID → WRONG_STATE。
5. 卖出 count=3 的格子，数量 2 → 报价 → 确认，仅剩 1；取消无变化。
6. 报价后背包不足、换物、金币不足或超限、超过 60 秒、换窗口、战斗、接管 → 拒绝，无部分成交。
7. 普通 NPC、分页、暗号的原选项编号及原文字节不变；无 window_id 不可操作 v2 窗口。

可执行网络与数据库契约位于 server/character/shop_menu_test.go；该文件的测试使用真实 JSON 请求与独立 schema。
