-- 骑乘等级（STAGE-122，docs/ride.md §2，docs/protocol.md 5.12、5.49）：原版 CHAR_LEARNRIDE。
-- 取值 0 / 40 / 80 / 120 / 200，新角色为 0（必须去骑乘训练师处上课），能骑的宠物等级上限就是它。
ALTER TABLE characters
    ADD COLUMN ride_level smallint NOT NULL DEFAULT 0 CHECK (ride_level IN (0, 40, 80, 120, 200));
