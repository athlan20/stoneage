-- STAGE-306：决斗（docs/pk.md 4.1，docs/protocol.md 3.5、5.74）。
-- duel_point：决斗点数，默认 100，范围 0–100000000（pk.md 2.2）；社交开关 duel 只在内存里，不在这里。
-- duel_logs：一场决斗一行，用来查刷分和申诉（pk.md 4.1）；开战写入的部分随双真人战斗引擎一起接入。
ALTER TABLE characters ADD COLUMN duel_point integer NOT NULL DEFAULT 100 CHECK (duel_point BETWEEN 0 AND 100000000);

CREATE TABLE duel_logs (
    id          bigserial PRIMARY KEY,
    battle_id   text NOT NULL,
    started_at  timestamptz NOT NULL DEFAULT now(),
    ended_at    timestamptz,
    floor       integer NOT NULL,
    members     jsonb NOT NULL DEFAULT '[]'::jsonb,
    dp_changes  jsonb NOT NULL DEFAULT '[]'::jsonb,
    result      text NOT NULL DEFAULT ''
);
CREATE INDEX duel_logs_started_idx ON duel_logs (started_at);
