-- STAGE-294：交易（docs/trade.md 2.7、4.1，docs/protocol.md 5.72）。
-- social_trade：接收交易邀请（false 即拒绝，默认开，老数据为 true）；trade_logs：一笔成功交易一行，取消的交易不写表。
-- 编号紧接 0029（迁移号必须连续，见 casetest 的检查）；与 test 上别的需求撞号时合进 test 前改号。
ALTER TABLE characters ADD COLUMN social_trade boolean NOT NULL DEFAULT true;

CREATE TABLE trade_logs (
    id          bigserial PRIMARY KEY,
    created_at  timestamptz NOT NULL DEFAULT now(),
    a_char_id   bigint NOT NULL,
    b_char_id   bigint NOT NULL,
    a_name      text NOT NULL,
    b_name      text NOT NULL,
    a_account   text NOT NULL,
    b_account   text NOT NULL,
    a_gold      bigint NOT NULL DEFAULT 0,
    b_gold      bigint NOT NULL DEFAULT 0,
    a_items     jsonb NOT NULL DEFAULT '[]'::jsonb,
    b_items     jsonb NOT NULL DEFAULT '[]'::jsonb,
    a_pet       jsonb,
    b_pet       jsonb,
    via_agent   boolean NOT NULL DEFAULT false
);

CREATE INDEX trade_logs_created_at_idx ON trade_logs (created_at);
