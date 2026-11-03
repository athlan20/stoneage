//! agent 协同（STAGE-6，`docs/agent.md`、`docs/protocol.md` 第 9 节）：令牌管理、控制权命令、私聊。
//! 命令的分发在 `world.rs` 按命令表进行；观察模式下“操作”类命令的拦截也由命令表驱动（STAGE-26）。

use serde_json::{Value, json};

use super::commands;
use super::App;
use super::destination::{Dest, find_destination};
use super::session::Pending;
use crate::proto::{
    AgentMessage, Control, ControlChanged, CreatedToken, Inventory, MapList, PlaceList, Position, TokenInfo,
    TokenList, WorldEntry,
};

/// 私聊最长字符数。
pub const AGENT_SAY_MAX: usize = 200;
/// 令牌备注最长字符数、有效期范围。
pub const TOKEN_LABEL_MAX: usize = 32;
pub const TOKEN_DAYS_MAX: u32 = 365;

/// 「系统」页里 agent 令牌的状态。
#[derive(Debug, Default)]
pub struct AgentState {
    pub tokens: Vec<TokenInfo>,
    /// 至少收到过一次令牌列表。
    pub loaded: bool,
    /// 刚生成的令牌，原文只显示一次：离开系统页或再执行别的令牌命令就清掉。
    pub fresh: Option<CreatedToken>,
    /// `agent` 命令（STAGE-170）发出了生成请求：令牌回来后把接入说明写进消息栏，不跳「系统」页。
    pub guide_wanted: bool,
}

/// 解析 `token new` 后面的参数：最后一个词是纯数字时当作有效期（天），其余是备注。
pub fn parse_token_args(args: &str) -> (&str, Option<u32>) {
    let args = args.trim();
    match args.rsplit_once(char::is_whitespace) {
        Some((label, d)) if d.parse::<u32>().is_ok() => (label.trim(), d.parse().ok()),
        None if args.parse::<u32>().is_ok() => ("", args.parse().ok()),
        _ => (args, None),
    }
}

impl super::WorldState {
    /// 本连接是不是观察方（agent 在操控）。
    pub fn observing(&self) -> bool {
        self.entry.control.you == "observer"
    }
}

impl App {
    pub(super) fn say_to_agent(&mut self, text: &str) -> bool {
        if self.session.say_busy(){return false;}
        let text = text.trim();
        if text.is_empty() {
            self.log.error(commands::usage_of("ag"));
            return false;
        }
        if text.chars().count() > AGENT_SAY_MAX {
            self.log.error(format!("私聊最多 {AGENT_SAY_MAX} 个字符"));
            return false;
        }
        self.send("agent_say", json!({"text": text}), Pending::AgentSay(text.to_string()))
    }

    /// `token new [备注] [天数]`、`token list`。
    pub(super) fn token_command(&mut self, rest: &str) -> bool {
        let rest = rest.trim();
        let (sub, args) = rest.split_once(char::is_whitespace).map_or((rest, ""), |(a, b)| (a, b.trim()));
        match sub {
            "" | "list" => self.request_tokens(),
            "new" => {
                let (label, days) = parse_token_args(args);
                if label.chars().count() > TOKEN_LABEL_MAX {
                    self.log.error(format!("备注最多 {TOKEN_LABEL_MAX} 个字符"));
                    return false;
                }
                if days.is_some_and(|d| d == 0 || d > TOKEN_DAYS_MAX) {
                    self.log.error(format!("有效期要在 1–{TOKEN_DAYS_MAX} 天之间"));
                    return false;
                }
                if let Some(w) = self.world.as_mut() {
                    w.agent.guide_wanted = false;
                }
                return self.create_token(label, days);
            }
            _ => {
                self.log.error(commands::usage_of("token"));
                false
            }
        }
    }

    /// `agent [备注] [天数]`（STAGE-170）：生成令牌，回来后把接入说明和令牌一起写进消息栏。
    pub(super) fn agent_command(&mut self, rest: &str) -> bool {
        let (label, days) = parse_token_args(rest);
        let label = if label.is_empty() { "agent" } else { label };
        if let Some(w) = self.world.as_mut() {
            w.agent.guide_wanted = false;
        }
        if !self.create_token(label, days) {
            return false;
        }
        if let Some(w) = self.world.as_mut() {
            w.agent.guide_wanted = true;
        }
        true
    }

    pub(super) fn create_token(&mut self, label: &str, days: Option<u32>) -> bool {
                if label.chars().count()>TOKEN_LABEL_MAX || days.is_some_and(|d|d==0 || d>TOKEN_DAYS_MAX) {self.log.error("备注最多32字，有效期为1–365天");return false;}
                if self.world.as_ref().is_some_and(|w|w.agent.loaded && w.agent.tokens.len()>=5){self.log.error("最多五个有效令牌，请先吊销旧令牌");return false;}
                if self.session.token_busy(){return false;}
                let mut data = json!({"label": label});
                if let Some(d) = days {
                    data["expires_in_days"] = json!(d);
                }
                self.send("create_agent_token", data, Pending::TokenCreate)
    }

    pub(super) fn revoke_command(&mut self, rest: &str) -> bool {
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        let Ok(n) = rest.trim().parse::<usize>() else {
            self.log.error(format!("{}（编号见「系统」页的令牌列表）", commands::usage_of("revoke")));
            return false;
        };
        let Some(token) = n.checked_sub(1).and_then(|i| world.agent.tokens.get(i)) else {
            self.log.error("没有这个编号；先输入 token list 刷新列表");
            return false;
        };
        let id = token.token_id.clone();
        self.revoke_token_id(&id)
    }
    pub(super) fn revoke_token_id(&mut self, id:&str)->bool {
        if self.session.token_busy(){return false;}
        if self.world.as_ref().is_none_or(|w|!w.agent.tokens.iter().any(|t|t.token_id==id)){self.log.error("令牌已失效，请刷新列表");return false;}
        self.send("revoke_agent_token", json!({"token_id": id}), Pending::TokenRevoke)
    }

    /// `turn 方向`：转向（操作类，观察模式的拦截由命令表驱动，见 `world.rs`）。
    pub(super) fn turn_command(&mut self, rest: &str) -> bool {
        let Ok(d) = rest.trim().parse::<u8>() else {
            self.log.error(commands::usage_of("turn"));
            return false;
        };
        if d > 7 {
            self.log.error("方向要在 0–7 之间");
            return false;
        }
        self.send("turn", json!({"direction": d}), Pending::Turn)
    }

    /// `warp 地点编号`、`warp 地点名`、`warp 地图编号` 或 `warp 地图名`：直接传送，没有移动过程（`docs/protocol.md` 5.16）。
    /// 名字先在本地换成编号或 floor（`destination.rs`，STAGE-82/83）。
    pub(super) fn warp_command(&mut self, rest: &str) -> bool {
        let arg = rest.trim();
        if arg.is_empty() {
            self.log.error(format!("{}（编号见地图上的 X 标记，也可以直接写地点名或地图名）", commands::usage_of("warp")));
            return false;
        }
        let world = self.world.as_ref();
        let places = world.map(|w| w.places.as_slice()).unwrap_or_default();
        let maps = world.map(|w| w.maps.as_slice()).unwrap_or_default();
        let (data, name) = match find_destination(places, maps, arg) {
            Ok(Dest::Place(p)) => (json!({"place": p.id}), p.name.clone()),
            Ok(Dest::Map(m)) => (json!({"floor": m.floor}), m.name.clone()),
            Ok(Dest::Floor(n)) => (json!({"floor": n}), format!("地图 {n}")),
            Err(msg) => {
                self.log.error(msg);
                return false;
            }
        };
        self.send("warp", data, Pending::Warp(name))
    }

    /// `encounter on|off` 或不带参数查询（`docs/protocol.md` 5.29，STAGE-40）。
    /// 操作类；观察模式的拦截由命令表驱动，见 `world.rs`（STAGE-26）。
    pub(super) fn encounter_command(&mut self, rest: &str) -> bool {
        let set = match rest.trim() {
            "" => None,
            "on" | "开" => Some(true),
            "off" | "关" => Some(false),
            _ => {
                self.log.error(commands::usage_of("encounter"));
                return false;
            }
        };
        // 关闭中不允许重新开启遇敌（STAGE-153）：等权威确认后再由玩家主动打开。
        if set == Some(true) && self.halt_busy() {
            self.log.system("正在关闭自动遇敌，请稍候");
            return false;
        }
        let mut data = json!({});
        if let Some(v) = set {
            data["set"] = json!(if v { "on" } else { "off" });
        }
        self.send("encounter", data, Pending::Encounter { set })
    }

    /// 请求令牌列表（进入「系统」页、生成或吊销之后）。
    pub(super) fn request_tokens(&mut self) -> bool {
        self.send("agent_tokens", json!({}), Pending::TokenList)
    }

    // ---- 响应 ----

    pub(super) fn on_status(&mut self, data: Value) {
        let entry: WorldEntry = serde_json::from_value(data).unwrap_or_default();
        if let Some(w) = self.world.as_mut() {
            w.entry = entry;
        }
    }

    pub(super) fn on_places(&mut self, data: Value) {
        let list: PlaceList = serde_json::from_value(data).unwrap_or_default();
        if let Some(w) = self.world.as_mut() {
            w.places = list.places;
        }
    }

    /// `maps` 响应：替换能传到的地图表（STAGE-83，`docs/protocol.md` 5.42）。
    pub(super) fn on_maps(&mut self, data: Value) {
        let list: MapList = serde_json::from_value(data).unwrap_or_default();
        if let Some(w) = self.world.as_mut() {
            w.maps = list.maps;
            // 快捷传送目录（STAGE-161）：平面表照旧给 `warp 地图名` 用，节点树给目录页用。
            // 旧服务端不返回 nodes，此时 `install` 会把状态置成「目录不可用」，页面提示重试。
            w.travel.install(list.catalog_version, &list.nodes);
            w.travel.here_floor = Some(w.entry.position.floor);
        }
    }

    /// `inventory` 响应与 `inventory_changed` 推送的内容相同（STAGE-28）。
    pub(super) fn on_inventory(&mut self, data: Value) {
        if let Some(w)=self.world.as_mut(){w.inventory_epoch+=1;}
        let inv: Inventory = serde_json::from_value(data).unwrap_or_default();
        if let Some(w) = self.world.as_mut() {
            // STAGE-295：石币在背包快照里（成交的 inventory_changed 也走这里），角色页的石币跟着刷新。
            w.entry.status.gold = inv.gold;
            w.inventory = inv;
        }
    }

    pub(super) fn on_token_created(&mut self, data: Value) {
        let t: CreatedToken = serde_json::from_value(data).unwrap_or_default();
        if self.world.as_mut().is_some_and(|w| std::mem::take(&mut w.agent.guide_wanted)) {
            let lines = super::agent_guide::guide_lines(&self.session.addr, &t.token);
            for line in &lines {
                self.log.system(line.as_str());
            }
            self.clipboard_out = Some(super::agent_guide::copy_text(&lines));
            self.log.system("已复制到剪贴板，到 agent 的对话框里粘贴即可（令牌也在剪贴板里，剪贴板历史工具可能会记下；不放心可用 revoke 吊销）");
            // 不跳「系统」页，所以列表要自己刷新（跳页时由进页面的请求负责）。
            self.request_tokens();
            return;
        }
        self.log.system(format!("已生成令牌 {}，原文只显示这一次，在「系统」页查看并立即交给你的 agent", t.token_id));
        self.switch_menu(super::SYSTEM_MENU);
        if let Some(w) = self.world.as_mut() {
            w.system.step=Some(super::system::SystemStep::Fresh);
            w.agent.fresh = Some(t);
        }
    }

    pub(super) fn on_token_list(&mut self, data: Value) {
        let list: TokenList = serde_json::from_value(data).unwrap_or_default();
        if let Some(w) = self.world.as_mut() {
            w.agent.tokens = list.tokens;
            w.agent.loaded = true;
        }
    }

    // ---- 推送 ----

    pub(super) fn on_control_changed(&mut self, data: Value) {
        let c: ControlChanged = serde_json::from_value(data).unwrap_or_default();
        let observing = c.you == "observer";
        let mut trade_gone = false;
        if let Some(w) = self.world.as_mut() {
            w.entry.agent_online=c.agent_online.unwrap_or_else(||match c.reason.as_str(){"PEER_LEFT"=>false,"AGENT_ENTERED"|"TAKEN_BY_PLAYER"|"RELEASED_BY_PLAYER"=>true,_=>w.entry.agent_online});
            if observing {
                trade_gone = w.trade.take().is_some();
                w.confirm=None;w.bag_step=None;w.pet_step=None;w.char_step=None;if let Some(b)=w.battle.as_mut(){b.prepared_row=None;}}
            w.entry.control = Control { controller: c.controller.clone(), you: c.you.clone() };
            // 玩家收回控制权：盖在战斗上的菜单自动退出，回到战斗画面（STAGE-182）。
            if !observing {
                w.close_battle_veil();
            }
        }
        // agent 接管控制权后客户端不再托管（docs/agent.md 的控制权规则，STAGE-42）。
        if observing {
            self.stop_auto("agent 操控中");
        }
        let text = match (c.reason.as_str(), c.you.as_str()) {
            ("AGENT_ENTERED", _) => "agent 进入游戏并接管了控制权；输入 take 收回",
            ("TAKEN_BY_PLAYER", _) => "你已收回控制权",
            ("RELEASED_BY_PLAYER", _) => "控制权已交还给 agent",
            ("PEER_LEFT", "controller") => "agent 已离开，控制权回到你手里",
            ("PEER_LEFT", _) => "对方已离开，控制权已转交",
            _ => "控制权发生了变化",
        };
        self.log.system(text);
        if trade_gone {
            self.log.system("交易因控制权变化取消");
        }
    }

    pub(super) fn on_position_changed(&mut self, data: Value) {
        let Ok(p) = serde_json::from_value::<Position>(data) else {self.log.error("位置推送格式无效");return;};
        // 位置变了（floor/x/y，转向只变朝向不算）就停托管：被传送后不该接着原地挂机（STAGE-42）。
        let moved = self.world.as_ref().is_some_and(|w| {
            let old = &w.entry.position;
            (old.floor, old.x, old.y) != (p.floor, p.x, p.y)
        });
        // 换了地图就重新取一次本地图 NPC（STAGE-68）；同一张图内移动不用重取。
        // 观察方也会跟着重取：NPC 列表始终是操控方所在地图的。
        let changed_floor = self.world.as_ref().is_some_and(|w| w.entry.position.floor != p.floor);
        if let Some(w) = self.world.as_mut() {
            w.entry.position = p.clone();
            // 快捷传送页的「← 当前」跟着位置走（STAGE-161）；目录本身和玩家选到哪一层都保留。
            w.travel.here_floor = Some(p.floor);
            // 换了地图，场景页的 NPC 和出口都变了（STAGE-68）。
            if changed_floor {
                w.scene.invalidate();
                w.npcs.clear();
                w.scene_index=0;
                w.scene_scroll=0;
            }
        }
        if changed_floor {
            self.refresh_scene();
        }
        if moved {
            self.stop_auto("位置发生了变化");
        }
    }

    pub(super) fn on_agent_message(&mut self, data: Value) {
        let m: AgentMessage = serde_json::from_value(data).unwrap_or_default();
        // 服务端只把对方的私聊推给我们；自己发出的由 agent_say 的成功响应写入消息栏。
        // 消息类型标签已经是“agent ”，正文不再重复前缀。
        self.log.agent(m.text);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::app::pending::Pending;
    use crate::app::{Options, WorldState};
    use crate::proto::WorldEntry;

    #[test]
    fn token_args() {
        assert_eq!(parse_token_args(""), ("", None));
        assert_eq!(parse_token_args("我的 Claude"), ("我的 Claude", None));
        assert_eq!(parse_token_args("我的 Claude 30"), ("我的 Claude", Some(30)));
        assert_eq!(parse_token_args("  助手   7  "), ("助手", Some(7)));
        assert_eq!(parse_token_args("7"), ("", Some(7)));
        assert_eq!(parse_token_args("agent2"), ("agent2", None)); // 备注里带数字不算有效期
        assert_eq!(parse_token_args("小号 0"), ("小号", Some(0))); // 范围由调用方检查
    }

    /// STAGE-171：`agent` 命令生成失败（比如已有 5 个令牌）时，不写剪贴板，也不留着“要说明”的标志，
    /// 否则之后在系统页生成的令牌会被误复制。
    #[test]
    fn failed_agent_command_leaves_clipboard_and_guide_flag_alone() {
        let mut a = App::new(Options::default());
        a.world = Some(WorldState::new(WorldEntry::default()));
        a.world.as_mut().unwrap().agent.guide_wanted = true;
        let err = crate::proto::ProtoError {
            code: "TOKEN_LIMIT".into(),
            message: "有效令牌已达上限".into(),
            ..Default::default()
        };
        a.on_error(Pending::TokenCreate, err);
        assert!(!a.world.as_ref().unwrap().agent.guide_wanted, "失败后标志应清掉");
        assert!(a.take_clipboard().is_none(), "失败时不应写剪贴板");
    }
}
