-- 角色的任务进度（STAGE-86，docs/protocol.md 5.44、5.45，docs/ui.md 任务页）。
-- 每个任务编号一行，state：1 进行中、2 已完成；没有行表示没接过（任务页不列）。
-- 一期写这张表的只有管理命令 quest（stoneage-admin quest，直接写库并 NOTIFY character_quests_changed，
-- 服务端收到通知后重读该角色的任务并推送 quests_changed）。任务 NPC 做完后也写这张表。
-- 删除角色时随角色一并清理。
CREATE TABLE character_quests (
    character_id bigint      NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    quest_no     integer     NOT NULL CHECK (quest_no > 0),
    state        smallint    NOT NULL CHECK (state IN (1, 2)),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (character_id, quest_no)
);
