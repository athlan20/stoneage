-- 合成品标记（STAGE-207，原版 ITEM_MERGEFLG）：加工、料理合成出的产物置真，材料的复制品（合成失败、冷却时还回的）不置。
-- 本需求只写标记，不实现它的效果（战斗里合成装备的随机效果、地面物品的颜色见 STAGE-209）。
ALTER TABLE character_items ADD COLUMN merged boolean NOT NULL DEFAULT false;
