-- 角色（STAGE-4）。常用字段（名字、等级、地图、坐标）都是普通列，数据库客户端里能直接看。
-- 以后的排行、名片等表引用 characters(id) 并写 ON DELETE CASCADE，删除角色时就会一并清理。
CREATE TABLE characters (
    id            bigserial   PRIMARY KEY,
    account_id    bigint      NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    slot          smallint    NOT NULL CHECK (slot >= 0),
    name          text        NOT NULL,
    -- 全服唯一用的键：大小写、全角半角折叠之后的名字，防止用相近的字符冒名。
    name_key      text        NOT NULL UNIQUE,

    -- 外形：序号，以及服务端换算出的旧图像编号。
    body          smallint    NOT NULL,
    color         smallint    NOT NULL,
    eye           smallint    NOT NULL,
    mouth         smallint    NOT NULL,
    body_image    integer     NOT NULL,
    face_image    integer     NOT NULL,

    -- 创建时分配的能力点和属性点。
    vital         smallint    NOT NULL,
    strength      smallint    NOT NULL,
    toughness     smallint    NOT NULL,
    dexterity     smallint    NOT NULL,
    earth         smallint    NOT NULL,
    water         smallint    NOT NULL,
    fire          smallint    NOT NULL,
    wind          smallint    NOT NULL,
    hometown      smallint    NOT NULL,

    level         integer     NOT NULL DEFAULT 1,
    transmigration integer    NOT NULL DEFAULT 0,
    exp           bigint      NOT NULL DEFAULT 0,
    gold          bigint      NOT NULL DEFAULT 0,
    hp            integer     NOT NULL,
    mp            integer     NOT NULL,

    floor         integer     NOT NULL,
    x             integer     NOT NULL,
    y             integer     NOT NULL,
    direction     smallint    NOT NULL DEFAULT 4,

    created_at    timestamptz NOT NULL DEFAULT now(),
    saved_at      timestamptz NOT NULL DEFAULT now(),
    last_enter_at timestamptz,

    UNIQUE (account_id, slot)
);
