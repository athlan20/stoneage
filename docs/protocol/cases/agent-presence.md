# STAGE-139 在线状态契约

真实数据库回归 `server/character/control_test.go::TestControlPassesOnDisconnect` 验证：

1. agent 进入，control_changed 带 agent_online=true。
2. 玩家收回控制权，agent 仍在线，agent_online=true。
3. 观察方 agent 离开，玩家收到 agent_presence {online:false}，玩家仍能转向。
4. 操控方 agent 离开，control_changed 带 agent_online=false，控制权转交玩家。

enter_world/status 一律提供 agent_online；不得由 controller 字段推断在线状态。
