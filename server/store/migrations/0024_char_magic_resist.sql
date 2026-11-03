-- 人物的魔法抗性与熟练度（STAGE-226，原版 include/char_base.h 的 CHAR_EARTH_EXP…、CHAR_EARTH_RESIST…、
-- CHAR_EARTH_ATTMAGIC_EXP…、CHAR_EARTH_DEFMAGIC_EXP…）。下标依次是地、水、火、风，初值全 0。
-- 本需求只存值，不改伤害公式；成长（Magic_ComputeAttExp / DefExp）在后续需求里接入。
ALTER TABLE characters
    ADD COLUMN magic_exp        integer[4] NOT NULL DEFAULT ARRAY[0,0,0,0],
    ADD COLUMN magic_resist     integer[4] NOT NULL DEFAULT ARRAY[0,0,0,0],
    ADD COLUMN magic_att_exp    integer[4] NOT NULL DEFAULT ARRAY[0,0,0,0],
    ADD COLUMN magic_def_exp    integer[4] NOT NULL DEFAULT ARRAY[0,0,0,0];
