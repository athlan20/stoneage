-- 账号、登录审计和登录锁定（STAGE-3）。密码只保存 Argon2id 哈希。
CREATE TABLE accounts (
    id              bigserial   PRIMARY KEY,
    name            text        NOT NULL UNIQUE CHECK (name = lower(name)),
    password_hash   text        NOT NULL,
    disabled        boolean     NOT NULL DEFAULT false,
    disabled_reason text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_ip      text        NOT NULL DEFAULT '',
    last_login_at   timestamptz,
    last_login_ip   text        NOT NULL DEFAULT ''
);

-- 每次注册和登录（无论成功还是失败）都写一条。频率限制也是从这张表统计出来的。
CREATE TABLE auth_audit (
    id      bigserial   PRIMARY KEY,
    at      timestamptz NOT NULL DEFAULT now(),
    kind    text        NOT NULL,  -- register | login
    account text        NOT NULL,
    ip      text        NOT NULL,
    success boolean     NOT NULL,
    result  text        NOT NULL   -- ok 或错误码
);
CREATE INDEX auth_audit_ip_idx      ON auth_audit (ip, kind, at);
CREATE INDEX auth_audit_account_idx ON auth_audit (account, kind, at);

-- 被锁定的账号名。按名字而不是账号 id 记录，所以不存在的账号名也会被锁，
-- 外人无法借“能否被锁定”判断账号是否存在。
CREATE TABLE login_locks (
    account      text        PRIMARY KEY,
    locked_until timestamptz NOT NULL
);
