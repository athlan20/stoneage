-- 角色未分配的能力点（STAGE-24，docs/protocol.md 5.8、5.21，docs/ui.md 角色页 B）。
-- 来源：升级（还没做）和管理命令 stoneage-admin givepoint（直接写库并 NOTIFY character_points_changed）。
-- 加点（addpoint）在同一条 UPDATE 里扣减这一列并加上对应的能力，条件是点数够，所以不会扣成负数。
ALTER TABLE characters ADD COLUMN skill_points integer NOT NULL DEFAULT 0 CHECK (skill_points >= 0);
