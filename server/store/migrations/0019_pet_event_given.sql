-- 任务送的宠物（STAGE-197，原版 CHAR_ENDEVENT = 1，npc_exchangeman.c 的 NPC_EventAddPet mode 0）：
-- 普通的 PET 条件和 DelPet 不数它，只在服务端记录，协议不下发。旧存档的宠物读出来是 false。
ALTER TABLE character_pets ADD COLUMN event_given boolean NOT NULL DEFAULT false;
