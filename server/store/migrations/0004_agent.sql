-- agent 协同（STAGE-6）：专用令牌和审计。令牌原文只在生成时返回一次，这里只保存哈希值。
CREATE TABLE agent_tokens (
    id           bigserial   PRIMARY KEY,
    token_id     text        NOT NULL UNIQUE,   -- 给玩家看的编号，如 t_7Hq2Xa9k，不是令牌本身
    account_id   bigint      NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    label        text        NOT NULL DEFAULT '',
    token_hash   text        NOT NULL UNIQUE,   -- 令牌原文的 SHA-256（令牌是 256 位随机数，不需要慢哈希）
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    last_used_at timestamptz,
    revoked_at   timestamptz
);
CREATE INDEX agent_tokens_account_idx ON agent_tokens (account_id);

-- agent 连接发起的每个“操作”类请求，以及控制权的每次变化。
CREATE TABLE agent_audit (
    id           bigserial   PRIMARY KEY,
    at           timestamptz NOT NULL DEFAULT now(),
    account_id   bigint      NOT NULL,
    account      text        NOT NULL,
    character_id bigint,                         -- 角色已被删除时仍保留记录，所以不加外键
    role         text        NOT NULL,           -- 发起方的连接身份：player | agent
    token_id     text        NOT NULL DEFAULT '', -- agent 连接用的令牌编号
    kind         text        NOT NULL,           -- request | control
    request      text        NOT NULL,           -- 请求类型；control 时是变化原因（TAKEN_BY_PLAYER 等）
    result       text        NOT NULL,           -- ok 或错误码
    detail       text        NOT NULL DEFAULT ''
);
CREATE INDEX agent_audit_account_idx ON agent_audit (account_id, at);
