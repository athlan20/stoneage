-- 任务 NPC 接任务时收走的东西（STAGE-94，docs/quest.md「放弃时撤销」）。
-- 接任务（REQUEST）的交易里 DelItem 实际收走的道具和 DelStone 收走的石币，按事件号记在这里；
-- 放弃（StopMsg 选是）时退回。items 是 [{"id": 道具编号, "n": 件数}]，gold 是石币。
-- 接任务时整笔为空（什么都没收）就不写；事件完成或重新接任务时覆盖。删除角色时随角色一并清理。
CREATE TABLE character_event_refunds (
    character_id bigint      NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    event_no     integer     NOT NULL CHECK (event_no > 0),
    items        jsonb       NOT NULL DEFAULT '[]',
    gold         bigint      NOT NULL DEFAULT 0 CHECK (gold >= 0),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (character_id, event_no)
);
