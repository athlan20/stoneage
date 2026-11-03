-- 宠物的个体等级上限（STAGE-234，原版 CHAR_LIMITLEVEL 用在宠物上，玛蕾等的对话 LimitLevel 动作改写它，
-- 见 docs/quest.md「宠物对话」）。NULL 表示没有被对话改过，用模板的 LIMITLEVEL（原版创建时也是这样带入的）；
-- 旧存档和新宠物都是 NULL。只有对话写它，战后写回（savePet）和插入都不碰这一列。
ALTER TABLE character_pets ADD COLUMN limit_level integer CHECK (limit_level IS NULL OR limit_level > 0);
