-- STAGE-278：公开聊天禁言（docs/chat.md 1.4）。管理员用 stoneage-admin mute 写入解除时间，空为未禁言；
-- 只挡公开频道（map、world、party），不影响 agent_say 私聊。编号紧接 0027（迁移号必须连续，见 casetest 的检查）；与别的需求撞号时合进 test 前改号。
ALTER TABLE characters ADD COLUMN chat_muted_until timestamptz;
