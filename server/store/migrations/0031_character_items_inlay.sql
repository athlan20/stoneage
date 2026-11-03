-- STAGE-298：精工实例覆盖（docs/inlay.md 2.4）。inlay 为 NULL 表示没镶过；结构见 server/character/inlay_state.go。
-- 编号紧接 0029（迁移号必须连续，casetest 会检查）。test 上 0030 已是交易（STAGE-294）：合进 test 前 git mv 到最大号之后（当前 0031）。
ALTER TABLE character_items ADD COLUMN inlay jsonb;
