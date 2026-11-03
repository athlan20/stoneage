-- 宠物的击杀数（STAGE-150，原版 CHAR_KILLPETCOUNT，battle.c 胜利结算）：出过手的宠物每击败一只怪 +1，
-- 骑乘宠物的记入在 STAGE-124 的合体结算里补。只在服务端记录，协议暂不下发。
-- 新宠物为 0；旧存档的宠物读出来也是 0（DEFAULT 0 直接补上）。
ALTER TABLE character_pets ADD COLUMN kills integer NOT NULL DEFAULT 0 CHECK (kills >= 0);
