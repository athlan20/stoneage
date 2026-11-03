-- 角色的魅力与运气（STAGE-61，docs/protocol.md 5.8，docs/ui.md 角色页 A）。
-- charm 是魅力的基础值：新角色 60，战斗中的增减直接改它，夹在 0–100（原版 CHAR_AddCharm）。
-- luck 是当日运气（1–5 档，0 表示还没掷过），luck_rolled_at 是上次掷的时间：进入世界时若不是同一个游戏日就重掷（原版 CHAR_setLuck）。
-- 老角色的 charm 默认 60（原版固定值），luck_rolled_at 为空，下次进入世界时掷一次。
ALTER TABLE characters
    ADD COLUMN charm integer NOT NULL DEFAULT 60 CHECK (charm BETWEEN 0 AND 100),
    ADD COLUMN luck smallint NOT NULL DEFAULT 0 CHECK (luck BETWEEN 0 AND 5),
    ADD COLUMN luck_rolled_at timestamptz;
