-- 四圣石卡关修复：STAGE-341 之前领过鲁乌/迪欧/加美的角色，任务 44/46/48 已完成，但练宠任务 43/45/47 没进入进行中，
-- 泰坦队长、米兰达队长、爱丽儿队长的交付块（要求事件 55/56/57 进行中）永远不命中，岳/涛/岚之圣石拿不到。
-- 这里按 keepDoing 的规则补一遍：44→43、46→45、48→47，没有记录且任务 41 还没完成的补成进行中。
INSERT INTO character_quests (character_id, quest_no, state)
SELECT d.character_id, d.quest_no - 1, 1
FROM character_quests d
WHERE d.state = 2
  AND d.quest_no IN (44, 46, 48)
  AND NOT EXISTS (
      SELECT 1 FROM character_quests c
      WHERE c.character_id = d.character_id AND c.quest_no = d.quest_no - 1)
  AND NOT EXISTS (
      SELECT 1 FROM character_quests f
      WHERE f.character_id = d.character_id AND f.quest_no = 41 AND f.state = 2)
ON CONFLICT DO NOTHING;
