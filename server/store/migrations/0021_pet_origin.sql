-- 宠物的起始等级与起始能力（STAGE-215）：宠物进库（到手）那一刻的等级和能力，之后不再改动，用来看它“原来有多强”。
-- 捕捉、出生送宠、任务送宠、宠物店买入、givepet 在插入时写入；升级、战斗结算、存取宠物店、改名、改技能、
-- 忠诚与转生次数的变化都不改这五列。能力单位同 vital 等列（1/100 点）。
-- 五列都允许 NULL：NULL 表示历史宠物，没有基准，不倒推、不补默认值；要么全空，要么全有（CHECK 保证）。
ALTER TABLE character_pets
    ADD COLUMN origin_level     integer,
    ADD COLUMN origin_vital     integer,
    ADD COLUMN origin_strength  integer,
    ADD COLUMN origin_toughness integer,
    ADD COLUMN origin_dexterity integer,
    ADD CONSTRAINT character_pets_origin_check CHECK (
        (origin_level IS NULL AND origin_vital IS NULL AND origin_strength IS NULL
            AND origin_toughness IS NULL AND origin_dexterity IS NULL)
        OR (origin_level >= 1 AND origin_vital IS NOT NULL AND origin_strength IS NOT NULL
            AND origin_toughness IS NOT NULL AND origin_dexterity IS NOT NULL)
    );
