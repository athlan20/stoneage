-- 装备耐久（STAGE-209，原版 ITEM_DAMAGECRUSHE / ITEM_MAXDAMAGECRUSHE / ITEM_CRUSHLEVEL，docs/durability.md）。
-- 老数据不回填：max_durability 为 0 的物品不损坏；之后新建的物品按物品表设初始耐久。
ALTER TABLE character_items ADD COLUMN durability int NOT NULL DEFAULT 0;
ALTER TABLE character_items ADD COLUMN max_durability int NOT NULL DEFAULT 0;
ALTER TABLE character_items ADD COLUMN crush_level int NOT NULL DEFAULT 0;
