-- 存档点（STAGE-72，docs/protocol.md 5.40）：角色记录的存档点 NPC 编号，
-- 0 表示没有（战败回出生地）。落点从 gamedata 的存档点数据查，不重复存坐标。
ALTER TABLE characters ADD COLUMN savepoint int NOT NULL DEFAULT 0;
