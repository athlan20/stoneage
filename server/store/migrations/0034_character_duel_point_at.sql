-- STAGE-308：强者表（docs/pk.md 3.5、5.3，协议 5.75）。
-- duel_point_at：决斗点数最后一次变化的时间，同分时先达到的排前。老数据统一为迁移执行的时间（同分按名字排）。
ALTER TABLE characters ADD COLUMN duel_point_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX characters_duel_rank_idx ON characters (duel_point DESC, duel_point_at ASC, name ASC);
