//! STAGE-139：系统动作与表单共享发送入口。
use super::{App, ExitKind, TextInput, session::Pending};
use serde_json::{Value, json};

#[derive(Debug, Default)]
pub struct SystemState {
    pub index: usize,
    pub step: Option<SystemStep>,
    pub battle_open: bool,
}
#[derive(Debug)]
pub enum SystemStep {
    Chat(TextInput),
    Create {
        label: TextInput,
        days: TextInput,
        focus: usize,
        confirm: bool,
    },
    Tokens {
        index: usize,
    },
    Revoke {
        id: String,
        label: String,
        confirm: bool,
    },
    Fresh,
    Info,
    Help,
    /// 自动战斗设置（STAGE-173）：五栏编辑器，非战斗与战斗控制面板共用一份。
    AutoBattle,
}
pub const ROWS: [&str; 16] = [
    "收回控制权",
    "交还控制权",
    "让 agent 下线",
    "私聊 agent",
    "查看／吊销令牌",
    "生成令牌",
    "自动战斗",
    "连续遇敌",
    "Unicode 图形字符",
    "默认回放倍速",
    "自动战斗设置",
    "重新连接",
    "连接信息",
    "操作帮助",
    "离开世界",
    "退出游戏",
];

/// 战斗中打开的控制面板只留和战斗相关的项（STAGE-153）：控制权、私聊、自动挂机、停止自动遇敌、
/// 自动战斗设置、显示与回放、连接、帮助、退出。正常玩家也能打开，所以挂机和遇敌的入口对所有人可见。
pub fn visible_rows(w:&super::WorldState)->Vec<usize>{
    if w.battle.is_some(){vec![0,1,2,3,6,7,10,8,9,11,12,13,15]}else{(0..ROWS.len()).collect()}
}

/// 管理与只读请求不受观察方限制；所有游戏写操作在同一发送点复核。
pub fn game_mutation(ty: &str, data: &Value) -> bool {
    matches!(
        ty,
        "warp"
            | "use_exit"
            | "turn"
            | "talk"
            | "npc_open"
            | "npc_choose"
            | "npc_reply"
            | "npc_confirm"
            | "npc_back"
            | "battle_command"
            | "add_point"
    ) || ty.starts_with("item_")
        || ty.starts_with("party_")
        || ty.starts_with("pet_")
        || (ty == "encounter" && data.get("set").is_some())
}
impl App {
    pub(super) fn system_enter(&mut self) {
        let Some(w) = self.world.as_ref() else { return };
        let index = w.system.index;
        if self.session.system_busy() {
            self.log.system("正在等待系统操作结果");
            return;
        }
        if matches!(index, 0..=3) && !w.entry.agent_online {
            self.log.error("agent 当前不在线");
            return;
        }
        match index {
            0 => {
                self.send("take_control", json!({}), Pending::Take);
            }
            1 => {
                self.send("release_control", json!({}), Pending::Release);
            }
            2 => {
                self.send("kick_agent", json!({}), Pending::KickAgent);
            }
            3 => self.system_step(SystemStep::Chat(TextInput::new(201))),
            4 => {
                self.request_tokens();
                self.system_step(SystemStep::Tokens { index: 0 });
            }
            5 => self.system_step(SystemStep::Create {
                label: TextInput::new(33),
                days: TextInput::with_text(5, "30"),
                focus: 0,
                confirm: false,
            }),
            6 => {
                // 动态项：开着就是「停止自动挂机」，关着就是「开始自动挂机」。
                // 停止走统一入口（联动关遇敌）；开始只开本机自动出招。
                if w.auto_battle {
                    self.stop_auto_all("系统页");
                } else {
                    self.start_auto("系统页");
                }
            }
            7 => {
                // 战斗中只能停，不在这里开新的遇敌（会抢走这一场的焦点）；平时是开关切换。
                if w.battle.is_some() {
                    self.stop_encounter_only();
                } else {
                    let on = !w.entry.encounter;
                    self.encounter_command(if on { "on" } else { "off" });
                }
            }
            8 => self.unicode = !self.unicode,
            9 => {
                // 回放三档（STAGE-173）：1× → 2× → 跳过 → 1×。战斗中不再有 f 键，统一在这里设。
                if self.skip_replay {
                    self.skip_replay = false;
                    self.default_battle_speed = 1;
                } else if self.default_battle_speed == 1 {
                    self.default_battle_speed = 2;
                } else {
                    self.default_battle_speed = 1;
                    self.skip_replay = true;
                }
                if let Some(b) = self.world.as_mut().and_then(|w| w.battle.as_mut()) {
                    b.speed = u32::from(self.default_battle_speed);
                }
            }
            10 => {
                // 战斗中从 Tab 面板进来就是 Panel（退出去回面板），平时是系统页的一项。
                let origin = if w.system.battle_open {
                    super::auto_battle::edit::Origin::Panel
                } else {
                    super::auto_battle::edit::Origin::SystemPage
                };
                self.open_auto_settings(origin);
                self.system_step(SystemStep::AutoBattle);
            }
            11 => self.system_reconnect(),
            12 => self.system_step(SystemStep::Info),
            13 => self.system_step(SystemStep::Help),
            14 => self.request_exit(ExitKind::Leave),
            15 => self.request_exit(ExitKind::Quit),
            _ => {}
        }
    }
    pub(super) fn system_step(&mut self, step: SystemStep) {
        if let Some(w) = self.world.as_mut() {
            w.system.step = Some(step);
        }
    }
    // STAGE-140 将这条入口接到保留快照的重新认证流程。
    pub(super) fn system_reconnect(&mut self) {
        self.begin_reconnect();
    }
}
