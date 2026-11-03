-- STAGE-347：道具仓库固定 50 格，不再随转生（0 转 10 格、每转 +4、最多 30 的旧规则作废）。
-- 已有的仓库道具都在 0–29 内，放宽约束不会丢东西；已转生的老角色同样是 50 格。
ALTER TABLE character_pool_items DROP CONSTRAINT IF EXISTS character_pool_items_slot_check;
ALTER TABLE character_pool_items ADD CONSTRAINT character_pool_items_slot_check CHECK (slot BETWEEN 0 AND 49);
