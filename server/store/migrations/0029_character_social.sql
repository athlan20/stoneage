-- STAGE-276：社交开关（docs/protocol.md 5.69，docs/social-players.md 第三节第 4 项）。
-- social_visible：对附近玩家可见（false 即隐身）；social_invite：接收组队邀请（false 即拒绝邀请）。默认都开，老数据为 true。
-- 编号紧接 0028（迁移号必须连续，见 casetest 的检查）；与别的需求撞号时合进 test 前改号。
ALTER TABLE characters ADD COLUMN social_visible boolean NOT NULL DEFAULT true;
ALTER TABLE characters ADD COLUMN social_invite boolean NOT NULL DEFAULT true;
