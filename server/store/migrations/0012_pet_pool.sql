-- 宠物店的寄放池（STAGE-73，docs/protocol.md 5.41）：寄放的宠物 state=3（原 0 休息、1 待命、2 出战），
-- slot 用 50 + 寄放池位（0–14，共 15 格，同原版 CHAR_MAXPOOLPETHAVE 的扩展值），
-- 与随身栏位 0–4 的唯一约束不冲突。放宽建表时的 CHECK。
ALTER TABLE character_pets DROP CONSTRAINT character_pets_slot_check;
ALTER TABLE character_pets DROP CONSTRAINT character_pets_state_check;
ALTER TABLE character_pets ADD CONSTRAINT character_pets_slot_check CHECK (slot BETWEEN 0 AND 99);
ALTER TABLE character_pets ADD CONSTRAINT character_pets_state_check CHECK (state BETWEEN 0 AND 3);
