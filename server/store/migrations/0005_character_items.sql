-- 角色的物品（STAGE-28，docs/protocol.md 5.17）。槽位 0–19：0–4 装备（头、身、手、饰品×2），
-- 5–19 背包；同一角色同一槽位只能有一个物品。删除角色时随角色一并清理。
-- 一期写这张表的只有管理命令 give（stoneage-admin give，直接写库并 NOTIFY character_items_changed，
-- 服务端收到通知后重读该角色的物品并推送 inventory_changed）；穿脱和使用物品见后续需求。
CREATE TABLE character_items (
    id           bigserial   PRIMARY KEY,
    character_id bigint      NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    slot         smallint    NOT NULL CHECK (slot BETWEEN 0 AND 19),
    item_id      integer     NOT NULL,
    count        integer     NOT NULL DEFAULT 1 CHECK (count >= 1),
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (character_id, slot)
);
CREATE INDEX character_items_character_idx ON character_items (character_id);
