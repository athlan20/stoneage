-- 角色的宠物（STAGE-31，docs/protocol.md 5.19、5.20，docs/ui.md 宠物页）。栏位 0–4，同一角色同一栏位只能有一只。
-- 一期写这张表的只有管理命令 givepet（stoneage-admin givepet，直接写库并 NOTIFY character_pets_changed，
-- 服务端收到通知后重读该角色的宠物并推送 pets_changed）。资质、原始能力只保存在服务端，不发给客户端；
-- 忠诚由服务端现算，不存。删除角色时随角色一并清理。
--
-- 能力单位是 1/100 点（原版服务端 CHAR_VITAL 等的存法）；资质是生成时 ±2 之后、+10 之前的值（CHAR_ALLOCPOINT）。
-- state：0 休息、1 待命、2 出战。skills 按技能栏位顺序存 7 个编号，空栏位为 -1。
CREATE TABLE character_pets (
    id             bigserial   PRIMARY KEY,
    character_id   bigint      NOT NULL REFERENCES characters (id) ON DELETE CASCADE,
    slot           smallint    NOT NULL CHECK (slot BETWEEN 0 AND 4),
    template_id    integer     NOT NULL,
    name           text        NOT NULL CHECK (name <> ''),
    level          integer     NOT NULL CHECK (level >= 1),
    exp            bigint      NOT NULL DEFAULT 0 CHECK (exp >= 0),
    vital          integer     NOT NULL,
    strength       integer     NOT NULL,
    toughness      integer     NOT NULL,
    dexterity      integer     NOT NULL,
    base_vital     integer     NOT NULL,
    base_strength  integer     NOT NULL,
    base_toughness integer     NOT NULL,
    base_dexterity integer     NOT NULL,
    rank           smallint    NOT NULL CHECK (rank BETWEEN 0 AND 5),
    variable_ai    integer     NOT NULL DEFAULT 0,
    hp             integer     NOT NULL CHECK (hp >= 0),
    state          smallint    NOT NULL DEFAULT 0 CHECK (state BETWEEN 0 AND 2),
    skills         integer[]   NOT NULL CHECK (array_length(skills, 1) = 7),
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (character_id, slot)
);
