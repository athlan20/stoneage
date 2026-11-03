-- 基线：服务端自己的元数据表。账号、角色等业务表由后续迁移添加（STAGE-3、STAGE-4）。
CREATE TABLE server_meta (
    key        text PRIMARY KEY,
    value      text        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO server_meta (key, value) VALUES ('created_at', to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'));
