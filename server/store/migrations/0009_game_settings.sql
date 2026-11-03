-- 全服游戏设置（STAGE-59，docs/server.md 管理命令）：一个设置一行，名字是主键。
-- 第一项是战斗经验倍数 exp_rate（原版 setup.cf 的 battleexp 对应物，默认 1 倍同原版节奏）；
-- 以后金币倍数、掉落倍数这类全服设置也放这张表，只需要加行。
-- 管理命令 stoneage-admin set 在同一个事务里 UPDATE 并 NOTIFY game_settings_changed，
-- 运行中的服务端 LISTEN 收到后把整张表重读进内存，不用重启。
CREATE TABLE game_settings (
    key        text PRIMARY KEY,
    value      numeric(8,2) NOT NULL,
    note       text        NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by text        NOT NULL DEFAULT ''
);

INSERT INTO game_settings (key, value, note) VALUES ('exp_rate', 1, '战斗经验倍数');
