use serde::Deserialize;
use serde_json::{Value, json};

use super::battle::BattleSnapshot;
use super::stats::{AbilityPoints, DerivedStats, ElementPoints};
use super::Party;
use super::TargetType;

/// 服务端返回的错误：`code` 给程序判断，`message` 直接显示给玩家。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct ProtoError {
    #[serde(default)]
    pub code: String,
    #[serde(default)]
    pub message: String,
    #[serde(default)]
    pub data: Value,
}

impl ProtoError {
    pub fn local(code: &str, message: impl Into<String>) -> Self {
        Self {
            code: code.into(),
            message: message.into(),
            data: Value::Null,
        }
    }

    pub fn retry_after_seconds(&self) -> Option<u64> {
        self.data.get("retry_after_seconds")?.as_u64()
    }
}

/// 从服务端收到的一行消息：带 `id` 的是响应，没有的是推送。
#[derive(Debug, Clone)]
pub enum Incoming {
    Response {
        id: u64,
        ty: String,
        result: Result<Value, ProtoError>,
    },
    Push {
        ty: String,
        data: Value,
    },
}

impl Incoming {
    pub fn parse(line: &str) -> Result<Self, String> {
        let v: Value = serde_json::from_str(line).map_err(|e| format!("无法解析服务端消息: {e}"))?;
        let ty = v
            .get("type")
            .and_then(Value::as_str)
            .ok_or("服务端消息缺少 type")?
            .to_string();
        let Some(id) = v.get("id").and_then(Value::as_u64) else {
            let data = v.get("data").cloned().unwrap_or(Value::Null);
            return Ok(Self::Push { ty, data });
        };
        let ok = v.get("ok").and_then(Value::as_bool).unwrap_or(false);
        let result = if ok {
            Ok(v.get("data").cloned().unwrap_or(Value::Null))
        } else {
            let err = v.get("error").cloned().unwrap_or(Value::Null);
            Err(serde_json::from_value(err).unwrap_or_else(|_| ProtoError::local("INTERNAL", "服务端返回了无法识别的错误")))
        };
        Ok(Self::Response { id, ty, result })
    }
}

pub fn request_line(id: u64, ty: &str, data: &Value) -> String {
    json!({"id": id, "type": ty, "data": data}).to_string()
}

/// 把消息里的 `password` 换成 `"***"`，任何日志、调试输出前都要先过一遍。
pub fn redact(v: &Value) -> Value {
    match v {
        Value::Object(map) => Value::Object(
            map.iter()
                .map(|(k, v)| {
                    let v = if k == "password" || k == "token" { json!("***") } else { redact(v) };
                    (k.clone(), v)
                })
                .collect(),
        ),
        Value::Array(a) => Value::Array(a.iter().map(redact).collect()),
        other => other.clone(),
    }
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct HelloInfo {
    #[serde(default = "default_heartbeat")]
    pub heartbeat_seconds: u64,
    #[serde(default)]
    pub idle_timeout_seconds: u64,
    #[serde(default)]
    pub register_enabled: bool,
    /// 服务端配置的最新客户端版本（STAGE-100）；没配置时为空。
    #[serde(default)]
    pub latest_client: Option<String>,
}

fn default_heartbeat() -> u64 {
    20
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
pub struct Appearance {
    #[serde(default)]
    pub body: u8,
    #[serde(default)]
    pub color: u8,
    #[serde(default)]
    pub eye: u8,
    #[serde(default)]
    pub mouth: u8,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct Location {
    #[serde(default)]
    pub floor: u32,
    #[serde(default)]
    pub map_name: String,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct Character {
    #[serde(default)]
    pub slot: u8,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub level: u32,
    #[serde(default)]
    pub appearance: Appearance,
    #[serde(default)]
    pub location: Location,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct CharacterList {
    #[serde(default)]
    pub max_slots: u8,
    #[serde(default)]
    pub characters: Vec<Character>,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct Position {
    #[serde(default)]
    pub floor: u32,
    #[serde(default)]
    pub map_name: String,
    #[serde(default)]
    pub x: i32,
    #[serde(default)]
    pub y: i32,
    #[serde(default)]
    pub direction: u8,
}

/// 精灵（魔法）栏位的一项（`docs/protocol.md` 5.8 的 `status.magic`，STAGE-48）。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct MagicView {
    /// 栏位 0–4，战斗 `magic` 指令的精灵编号。
    #[serde(default)]
    pub slot: u8,
    /// 魔法表编号（旧 `gmsv/data/magic.txt`）。
    #[serde(default)]
    pub id: u32,
    #[serde(default)]
    pub name: String,
    /// 使用一次所需的 MP。
    #[serde(default)]
    pub mp: u32,
    /// 战斗中的目标类型（STAGE-78）：决定选精灵后要不要再选目标、能选哪一方。
    #[serde(default)]
    pub target_type: TargetType,
    /// 效果文字（STAGE-116，`docs/protocol.md` 5.8）：服务端算好，角色页底边显示。
    #[serde(default)]
    pub effect: String,
}

/// 变幻的精灵的变身中状态（`docs/protocol.md` 5.8 的 `status.metamo`，STAGE-203）：没有变身时整个字段为 None。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct MetamoView {
    /// 变成的宠物名字。
    #[serde(default)]
    pub pet: String,
    /// 宠物模板的图像编号（一期终端不显示，只记录）。
    #[serde(default)]
    pub image: u32,
    /// 到期时间：服务端的毫秒时间戳，剩余秒数由本地时钟算。
    #[serde(default)]
    pub until: i64,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct Status {
    #[serde(default)]
    pub level: u32,
    #[serde(default)]
    pub hp: u32,
    #[serde(default)]
    pub max_hp: u32,
    #[serde(default)]
    pub mp: u32,
    #[serde(default)]
    pub max_mp: u32,
    #[serde(default)]
    pub exp: u64,
    #[serde(default)]
    pub next_exp: u64,
    #[serde(default)]
    pub gold: u64,
    /// 转生次数（角色页）。
    #[serde(default)]
    pub transmigration: u32,
    /// 骑乘等级 0/40/80/120/200（角色页，STAGE-122）：能骑的宠物等级上限。
    #[serde(default)]
    pub ride_level: u32,
    /// 称号；空字符串表示没有称号，界面显示“无”。
    #[serde(default)]
    pub title: String,
    /// 未分配的能力点（升级之后才会有）。
    #[serde(default)]
    pub skill_points: u32,
    #[serde(default)]
    pub stats: AbilityPoints,
    #[serde(default)]
    pub derived: DerivedStats,
    #[serde(default)]
    pub elements: ElementPoints,
    /// 精灵（魔法）栏位；没有学过精灵时空列表（角色页清单与战斗 `magic` 指令都用它）。
    #[serde(default)]
    pub magic: Vec<MagicView>,
    /// 变幻的精灵的变身中状态（STAGE-203）；没有变身时为 None，角色页不显示那一行。
    #[serde(default)]
    pub metamo: Option<MetamoView>,
}

/// 谁在操控这个角色，以及本连接是操控方（`controller`）还是观察方（`observer`），见 `docs/protocol.md` 9.3。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct Control {
    #[serde(default)]
    pub controller: String,
    #[serde(default)]
    pub you: String,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct WorldEntry {
    #[serde(default)]
    pub agent_online: bool,
    #[serde(default)]
    pub character: Character,
    #[serde(default)]
    pub position: Position,
    #[serde(default)]
    pub status: Status,
    #[serde(default)]
    pub control: Control,
    /// 遇敌开关的当前状态（`docs/protocol.md` 5.29，STAGE-40）。
    #[serde(default)]
    pub encounter: bool,
    /// 进入世界时正在战斗中才有（`docs/protocol.md` 10.11）。
    #[serde(default)]
    pub battle: Option<BattleSnapshot>,
    /// 存档点落点（STAGE-134，`docs/protocol.md` 5.12）：客户端的「回城」入口用它。
    /// 没记录过存档点的角色，服务端给的是出生地。
    #[serde(default)]
    pub savepoint: Option<Savepoint>,
    /// 当前队伍（STAGE-162，`docs/protocol.md` 5.57）：没有队伍时为空。断线重连后以它为准。
    #[serde(default)]
    pub party: Option<Party>,
    /// 社交开关（STAGE-276，`docs/protocol.md` 5.69）：进入世界和 `status` 里带；没给就是两个都开。
    #[serde(default)]
    pub social: super::Social,
    /// 决斗点数（STAGE-307，`docs/pk.md` 3.5）：进入世界时带，决斗结束后以 `battle_end.duel_point` 为准更新。
    #[serde(default)]
    pub duel_point: i64,
}

/// 存档点落点（`docs/protocol.md` 5.12 的 `savepoint` 字段，STAGE-134）。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct Savepoint {
    #[serde(default)]
    pub floor: u32,
    #[serde(default)]
    pub x: i32,
    #[serde(default)]
    pub y: i32,
    #[serde(default)]
    pub map_name: String,
}

/// `places` 响应里的一个地点（`docs/protocol.md` 5.15）。`warp` 用 `id`；`floor`/`x`/`y` 是传送后的位置。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct Place {
    #[serde(default)]
    pub id: u32,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub floor: u32,
    #[serde(default)]
    pub x: i32,
    #[serde(default)]
    pub y: i32,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct PlaceList {
    #[serde(default)]
    pub places: Vec<Place>,
}

/// `maps` 响应里的一张地图（`docs/protocol.md` 5.42）：能传到且有名字。`warp` 用 `floor`；名字用来搜索和补全。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct WarpMap {
    #[serde(default)]
    pub floor: u32,
    #[serde(default)]
    pub name: String,
}

/// 快捷传送目录的一个节点（`docs/protocol.md` 5.42，STAGE-161）：地图节点也可以有子节点，
/// `floor` 为空的是纯分组（只用来分层，不能传送），`parent_id` 为空的是根。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct TravelNode {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub parent_id: Option<String>,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub floor: Option<u32>,
    #[serde(default)]
    pub order: u32,
}

impl TravelNode {
    /// 是不是能传送的地图节点（纯分组返回 false）。
    pub fn is_map(&self) -> bool {
        self.floor.is_some()
    }
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct MapList {
    /// 目录版本号（STAGE-161）：层级或落点配置变了它才会变。
    #[serde(default)]
    pub catalog_version: String,
    #[serde(default)]
    pub maps: Vec<WarpMap>,
    /// 包含树。旧服务端不返回这个字段，此时 `nodes` 为空，快捷传送页要显示「目录不可用」。
    #[serde(default)]
    pub nodes: Vec<TravelNode>,
}

impl MapList {
    /// 服务端有没有返回目录树（STAGE-161）。没有时不能退回用平面表冒充全地图。
    pub fn has_catalog(&self) -> bool {
        !self.nodes.is_empty()
    }
}

/// 推送 `control_changed`。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct ControlChanged {
    #[serde(default)]
    pub agent_online: Option<bool>,
    #[serde(default)]
    pub controller: String,
    #[serde(default)]
    pub you: String,
    #[serde(default)]
    pub reason: String,
}

/// 推送 `encounter_changed`：遇敌开关变了（`docs/protocol.md` 5.30，STAGE-40）。
/// 手动开关没有 `reason`；自动关闭（打输、传送走）带原因，并另有一条 `notice` 说明。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct EncounterChanged {
    #[serde(default)]
    pub encounter: bool,
    #[serde(default)]
    pub reason: String,
}

/// 推送 `agent_message`：玩家与 agent 的私聊。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct AgentMessage {
    #[serde(default)]
    pub from: String,
    #[serde(default)]
    pub text: String,
}

/// 令牌列表里的一项（不含令牌原文）。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct TokenInfo {
    #[serde(default)]
    pub token_id: String,
    #[serde(default)]
    pub label: String,
    #[serde(default)]
    pub expires_at: String,
    #[serde(default)]
    pub last_used_at: Option<String>,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct TokenList {
    #[serde(default)]
    pub tokens: Vec<TokenInfo>,
}

/// 刚生成的令牌：`token` 原文只在这一次响应里出现。
#[derive(Clone, Default, Deserialize)]
pub struct CreatedToken {
    #[serde(default)]
    pub token_id: String,
    #[serde(default)]
    pub token: String,
    #[serde(default)]
    pub label: String,
    #[serde(default)]
    pub expires_at: String,
}

impl std::fmt::Debug for CreatedToken{
    fn fmt(&self,f:&mut std::fmt::Formatter<'_>)->std::fmt::Result{
        f.debug_struct("CreatedToken").field("token_id",&self.token_id).field("token",&"***").finish()
    }
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct Disconnect {
    #[serde(default)]
    pub reason: String,
    #[serde(default)]
    pub message: String,
}

#[derive(Debug, Clone, Default, Deserialize)]
pub struct Notice {
    #[serde(default)]
    pub level: String,
    #[serde(default)]
    pub text: String,
}
