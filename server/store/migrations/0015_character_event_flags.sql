-- 不对应任务表的事件号的进度（STAGE-94 问题 1，docs/quest.md「事件位与任务进度」）。
-- 原版的 ENDEV / NOWEV 是按事件号的位图；任务表（character_quests）只收 jobdaily.txt 里的事件，脚本里还有只有事件号、
-- 没有任务的中间步骤（如造斧的 19 号）。这些事件号的进度记在这里：state 同 character_quests，1 进行中、2 已完成，
-- 没有行表示没有。任务表里有的事件号不写这张表（它们的进度在 character_quests）。删除角色时随角色一并清理。
CREATE TABLE character_event_flags (
    character_id bigint      NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    event_no     integer     NOT NULL CHECK (event_no > 0),
    state        smallint    NOT NULL CHECK (state IN (1, 2)),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (character_id, event_no)
);
