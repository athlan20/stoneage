-- STAGE-236：宠物转生后个体等级上限为 -1（原版 CHAR_LIMITLEVEL=-1，STAGE-235 写回）。0025（STAGE-234）的检查只认正数，
-- 这里放开 -1；-1 和 0 同样表示“没有个体上限”（gamedata.PetMaxLevelFor 只认正数）。
ALTER TABLE character_pets DROP CONSTRAINT IF EXISTS character_pets_limit_level_check;
ALTER TABLE character_pets ADD CONSTRAINT character_pets_limit_level_check
    CHECK (limit_level IS NULL OR limit_level > 0 OR limit_level = -1);
