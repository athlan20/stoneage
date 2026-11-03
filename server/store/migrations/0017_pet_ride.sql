-- 骑乘（STAGE-123，docs/ride.md §4，docs/protocol.md 5.47–5.48）：宠物 state 新增 4 = 骑乘（ride），
-- 与休息、待命、出战、寄放（3）互斥，同时最多 1 只（规则在 pets_ride.go）。放宽 state 的 CHECK。
ALTER TABLE character_pets DROP CONSTRAINT character_pets_state_check;
ALTER TABLE character_pets ADD CONSTRAINT character_pets_state_check CHECK (state BETWEEN 0 AND 4);
