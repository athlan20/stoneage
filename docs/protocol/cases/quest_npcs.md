# STAGE-136 任务 NPC 查询契约

`quest_npcs {no:1}` 返回实际角色任务 state、npcs 数组、reason。每个候选必须能追溯到当前开放的 changeevent 脚本对该任务事件号的引用；current 仅标记本次选块实际命中的关联块，reference 不表示下一步。

不存在的任务返回 BAD_REQUEST；观察方可查询。空映射返回 [] 并给出搜索建议。不暴露暗号或发奖励。

可执行契约测试：server/character/quest_npcs_test.go。客户端浏览与过期对象校验：client/tests/browse.rs、keys_quest_wiki.rs。
