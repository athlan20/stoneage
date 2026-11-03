-- 宠物的转生次数（STAGE-196，原版 CHAR_TRANSMIGRATION 用在宠物上）：骑乘要求转生次数不超过 2（RIDE_TRANS_LIMIT，
-- docs/ride.md 第 9 条）。转生本身（NPC_TransmigrationAddPet）还没做，这一步只记录和读取。
-- 新宠物为 0；旧存档的宠物读出来也是 0（DEFAULT 0 直接补上）。
ALTER TABLE character_pets ADD COLUMN transmigration integer NOT NULL DEFAULT 0 CHECK (transmigration >= 0);
