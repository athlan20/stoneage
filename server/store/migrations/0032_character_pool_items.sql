-- STAGE-309：便利店道具仓库（docs/protocol.md 5.60）。寄存的物品整件（堆叠数量、耐久、合成、精工都原样）挪进这张表，
-- 槽位 0 起，上限随转生（0 转 10 格，每转 +4，最多 30）。编号紧接 0031；合进 test 前若撞号，git mv 到 test 上最大号之后。
CREATE TABLE character_pool_items (
    character_id  bigint      NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    slot          smallint    NOT NULL CHECK (slot BETWEEN 0 AND 29),
    item_id       integer     NOT NULL,
    count         integer     NOT NULL DEFAULT 1 CHECK (count >= 1),
    merged        boolean     NOT NULL DEFAULT false,
    durability    integer     NOT NULL DEFAULT 0,
    max_durability integer    NOT NULL DEFAULT 0,
    crush_level   integer     NOT NULL DEFAULT 0,
    inlay         jsonb,
    PRIMARY KEY (character_id, slot)
);
