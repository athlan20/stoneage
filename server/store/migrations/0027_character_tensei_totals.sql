-- STAGE-266：人物转生的累计任务数和累计等级（原版 CHAR_TRANSEQUATION 的高 16 位和低 16 位，拆成两列）。
-- 转生次数仍用 characters.transmigration；转生公式见 server/character/tensei_calc.go。老数据默认 0。
ALTER TABLE characters ADD COLUMN trans_quest_total integer NOT NULL DEFAULT 0 CHECK (trans_quest_total >= 0);
ALTER TABLE characters ADD COLUMN trans_level_total integer NOT NULL DEFAULT 0 CHECK (trans_level_total >= 0);
