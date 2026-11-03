//! 战斗消息（`docs/protocol.md` 第 10 章）：只把消息读成结构体，规则和画面都不在这里。
//! 服务端没给的字段取默认值；以后多出来的字段忽略（第 1 节）。

use serde::{Deserialize, Deserializer};

use super::ElementPoints;

/// 截止时间可以是 `null`（真人队同场，10.4 / 10.14）：按 0 处理，0 表示还没有截止时间。
fn null_zero<'de, D: Deserializer<'de>>(d: D) -> Result<u64, D::Error> {
    Ok(Option::<u64>::deserialize(d)?.unwrap_or(0))
}

/// 一个单位（10.2）。对方单位没有 `hp`、`max_hp`；`mp` 只有自己的人物才有。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleUnit {
    #[serde(default)]
    pub no: u8,
    /// `player`、`pet` 或 `monster`。
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub level: u32,
    #[serde(default)]
    pub hp: Option<u32>,
    #[serde(default)]
    pub max_hp: Option<u32>,
    /// 0–100，0 表示已倒下。
    #[serde(default)]
    pub hp_pct: u32,
    #[serde(default)]
    pub mp: Option<u32>,
    #[serde(default)]
    pub max_mp: Option<u32>,
    #[serde(default)]
    pub status: Vec<String>,
    #[serde(default)]
    pub ride: bool,
    #[serde(default)]
    pub elements: ElementPoints,
    /// 分身队的单位（分身人物和它的宠物，STAGE-162）：服务端自动出招，不带这个字段时为 false。
    #[serde(default)]
    pub auto: bool,
    /// STAGE-250：骑着的宠物（10.2 的 `mount`），只有骑乘的人物带，没骑乘时为 `None`。
    #[serde(default)]
    pub mount: Option<Mount>,
}

/// STAGE-250：人物骑着的宠物（10.2 的 `mount`）。骑宠不是单位，它的血量只在这里给出。
/// 对方的人物没有 `hp`、`max_hp`。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct Mount {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub hp: Option<u32>,
    #[serde(default)]
    pub max_hp: Option<u32>,
    /// 0–100，0 表示已倒下（之后伤害全由人承担）。
    #[serde(default)]
    pub hp_pct: u32,
}

/// 场地属性（10.4、10.7 的 `field`，STAGE-200）：整场战斗一个，双方共用。`attr` 为 `earth`/`water`/`fire`/`wind`，
/// 无属性时是 `none`（这时 `pow`、`left` 为 0）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleField {
    #[serde(default)]
    pub attr: String,
    /// 威力，0–100（场地对属性的加成按它算）。
    #[serde(default)]
    pub pow: u32,
    /// 剩余回合，回合末递减，减到 0 恢复无属性。
    #[serde(default)]
    pub left: u32,
}

impl BattleField {
    /// 场地有属性（`none` 与空串都是无属性）。
    pub fn active(&self) -> bool {
        !matches!(self.attr.as_str(), "" | "none")
    }

    /// 属性的中文字：地、水、火、风；其他（包括 none）是空串。
    pub fn attr_text(&self) -> &'static str {
        match self.attr.as_str() {
            "earth" => "地",
            "water" => "水",
            "fire" => "火",
            "wind" => "风",
            _ => "",
        }
    }
}

/// 本场允许的指令（`battle_start` 的 `allow`）。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
pub struct BattleAllow {
    #[serde(default)]
    pub escape: bool,
    #[serde(default)]
    pub capture: bool,
}

/// 推送 `battle_start`（10.3）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleStart {
    #[serde(default)]
    pub battle_id: String,
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub scene: String,
    #[serde(default)]
    pub you: u8,
    #[serde(default)]
    pub allow: BattleAllow,
    #[serde(default)]
    pub units: Vec<BattleUnit>,
    /// NPC 发起的战斗才有（`kind` 为 `npc`，STAGE-96）：`say` 是开战台词，写进战斗记录。
    #[serde(default)]
    pub npc: Option<BattleNpc>,
    /// 当前场地（STAGE-200），只在场地有属性时带。
    #[serde(default)]
    pub field: Option<BattleField>,
    /// 真人队同场（`kind` 为 `party`，STAGE-275，10.14）：同场的真人队员。
    #[serde(default)]
    pub party: Option<BattleParty>,
}

/// 真人队同场的队伍（10.3 的 `party`）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleParty {
    #[serde(default)]
    pub members: Vec<BattleMember>,
}

/// 同场的一个队员：`no` 是他人物的单位号，`leader` 为队长。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleMember {
    #[serde(default)]
    pub no: u8,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub leader: bool,
}

/// 战斗里的 NPC（10.3 的 `npc`、10.9 的 `npc`）：`say` 在 `battle_start` 里是开战台词，在 `battle_end` 里是结束台词。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleNpc {
    #[serde(default)]
    pub id: u32,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub say: String,
}

/// 交完指令的人数（组队时显示“等待其他玩家 2/3”）。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
pub struct Progress {
    #[serde(default)]
    pub done: u32,
    #[serde(default)]
    pub total: u32,
}

/// 推送 `battle_turn`（10.4）。倒计时只用 `deadline_ms` 与 `server_time_ms` 的差，不依赖本机时钟。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleTurn {
    #[serde(default)]
    pub battle_id: String,
    #[serde(default)]
    pub turn: u32,
    #[serde(default, deserialize_with = "null_zero")]
    pub deadline_ms: u64,
    #[serde(default, deserialize_with = "null_zero")]
    pub server_time_ms: u64,
    /// 这个角色本回合要下的指令：`player`、`pet`。
    #[serde(default)]
    pub needs: Vec<String>,
    /// 可以由玩家指定攻击目标的分身（`clone:N`、`clone_pet:N`，STAGE-186）；不在 `needs` 里，不交也能结算。
    #[serde(default)]
    pub optional: Vec<String>,
    #[serde(default)]
    pub progress: Progress,
    /// 当前场地（STAGE-200），只在场地有属性时带。
    #[serde(default)]
    pub field: Option<BattleField>,
}

/// 推送 `battle_progress`（10.6）。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
pub struct BattleProgress {
    #[serde(default)]
    pub turn: u32,
    #[serde(default)]
    pub done: u32,
    #[serde(default)]
    pub total: u32,
    /// 真人队同场：第一条指令被接受时才带截止时间（10.6），否则为 0。
    #[serde(default, deserialize_with = "null_zero")]
    pub deadline_ms: u64,
    #[serde(default, deserialize_with = "null_zero")]
    pub server_time_ms: u64,
}

/// 推送 `battle_leave`（10.14，STAGE-275）：真人队员在战斗中途离场（超时、掉线）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleLeave {
    #[serde(default)]
    pub battle_id: String,
    /// 离场人物的单位号。
    #[serde(default)]
    pub no: u8,
    #[serde(default)]
    pub name: String,
    /// `timeout`（超时踢出）或 `offline`（掉线）。
    #[serde(default)]
    pub cause: String,
    /// 离场的全部单位号（含他的宠物和分身）。
    #[serde(default)]
    pub left: Vec<u8>,
    /// 离场之后的交指令进度，之后的 `battle_progress` 以它为准。
    #[serde(default)]
    pub progress: Progress,
}

/// 一个事件对一个目标产生的结果（10.7 的 `effect`）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleEffect {
    #[serde(default)]
    pub target: u8,
    /// `hit`、`crit`、`miss`、`dodge`、`block`、`heal`、`none`。
    #[serde(default)]
    pub result: String,
    /// 血量变化：伤害为负，回复为正。
    #[serde(default)]
    pub delta: i64,
    #[serde(default)]
    pub hp: Option<u32>,
    #[serde(default)]
    pub hp_pct: Option<u32>,
    #[serde(default)]
    pub down: bool,
    #[serde(default)]
    pub status_add: Vec<String>,
    #[serde(default)]
    pub status_remove: Vec<String>,
    /// 出手时想打的目标（STAGE-167）：忠犬代受时 `target` 是真正挨打的单位，这里还是原目标。
    #[serde(default)]
    pub intent: Option<u8>,
    /// 这一次被打飞（STAGE-167，A05）：打飞和普通倒下不一样，要沿攻击方向离场。
    #[serde(default)]
    pub flown: bool,
}

/// 结算里的一个事件（10.7）。附加字段只在对应的 `kind` 里出现。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleEvent {
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub actor: Option<u8>,
    /// 合击（`combo`）的全部出手者，按出手顺序；`effects` 与它一一对应（STAGE-126）。
    #[serde(default)]
    pub actors: Vec<u8>,
    #[serde(default)]
    pub effects: Vec<BattleEffect>,
    /// 本事件后离开战场的单位。
    #[serde(default)]
    pub left: Vec<u8>,
    /// 本事件后加入战场的单位。
    #[serde(default)]
    pub joined: Vec<BattleUnit>,
    /// 精灵、道具、技能的名字。
    #[serde(default)]
    pub name: Option<String>,
    /// 捕捉、逃跑是否成功。
    #[serde(default)]
    pub ok: Option<bool>,
    /// 捕捉的目标。
    #[serde(default)]
    pub target: Option<u8>,
    /// 换宠、捕捉放进的宠物栏位；换宠收回时为 -1。
    #[serde(default)]
    pub pet_slot: Option<i64>,
    /// 抓成功但被资质筛选（`keep`，STAGE-264）放生：带它时没有 `pet_slot`，宠物不入库。
    #[serde(default)]
    pub discarded: Option<CaptureDiscard>,
    /// `skip` 的原因。
    #[serde(default)]
    pub reason: Option<String>,
    /// 出手的武器身份（STAGE-167）：`fist`、`axe`、`club`、`spear`、`claw`、`bow`、
    /// `throw_axe`、`throw_stone`、`boomerang`。客户端按它挑动作，不靠名字猜。
    #[serde(default)]
    pub weapon: String,
    /// 施法的精灵表现信息（STAGE-167），只有 `magic` 事件带。
    #[serde(default)]
    pub magic: Option<SpellView>,
    /// 回旋镖扫过的横排（STAGE-167），只有回旋镖事件带。
    #[serde(default)]
    pub row: Option<u8>,
    /// 场地变化或恢复（STAGE-200，只有 kind 为 `field` 的事件带）：`attr` 为 `none` 表示无属性。
    #[serde(default)]
    pub field: Option<BattleField>,
    /// 人物装备耐久的提示（STAGE-225，10.7）：挨打或出手打中后可能有，没有变化时为空。
    #[serde(default)]
    pub crush: Vec<CrushNote>,
}

/// 装备耐久的一条系统消息（STAGE-225，`docs/protocol.md` 10.7）：`red` 为真用红色，否则黄色。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct CrushNote {
    #[serde(default)]
    pub text: String,
    #[serde(default)]
    pub red: bool,
}

/// 精灵事件的表现信息（STAGE-167，`docs/protocol.md` 10.7）：按 `func` / `elem` / `all`
/// 挑特效和范围，`name` 只用来显示。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct SpellView {
    #[serde(default)]
    pub id: u32,
    /// `attack`、`heal`、`heal_other`、`revive`、`cure`、`status`。
    #[serde(default)]
    pub func: String,
    /// 攻击系的属性：`earth`、`water`、`fire`、`wind`。
    #[serde(default)]
    pub elem: String,
    #[serde(default)]
    pub all: bool,
    #[serde(default)]
    pub status: String,
}

impl BattleEvent {
    /// 出手者：合击是 `actors`，其余事件只有 `actor`（STAGE-126）。
    pub fn actor_list(&self) -> Vec<u8> {
        if self.actors.is_empty() {
            self.actor.into_iter().collect()
        } else {
            self.actors.clone()
        }
    }
}

/// 一回合结束后的单位状态（`battle_round` 的 `state`）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleUnitState {
    #[serde(default)]
    pub no: u8,
    #[serde(default)]
    pub hp: Option<u32>,
    #[serde(default)]
    pub hp_pct: u32,
    #[serde(default)]
    pub mp: Option<u32>,
    #[serde(default)]
    pub status: Vec<String>,
    #[serde(default)]
    pub down: bool,
    /// STAGE-250：骑着的宠物的血量，骑乘的人物每回合都带，没骑乘时为 `None`。
    #[serde(default)]
    pub mount: Option<Mount>,
}

/// 推送 `battle_round`（10.7）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct BattleRound {
    #[serde(default)]
    pub battle_id: String,
    #[serde(default)]
    pub turn: u32,
    #[serde(default)]
    pub events: Vec<BattleEvent>,
    #[serde(default)]
    pub state: Vec<BattleUnitState>,
}

/// 资质筛选放生的宠物（10.7 的 `capture` 事件 `discarded`，STAGE-264）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct CaptureDiscard {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub level: u32,
    /// 四项资质，顺序固定为 vital、strength、toughness、dexterity。
    #[serde(default)]
    pub quality: Vec<CaptureQuality>,
    /// 第一个不达标的项，如 `体力 −3 < −2`。
    #[serde(default)]
    pub reason: String,
}

/// 一项资质：素质点和档位（0 为满，越负越差）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Deserialize)]
pub struct CaptureQuality {
    #[serde(default)]
    pub stat: String,
    #[serde(default)]
    pub points: i64,
    #[serde(default)]
    pub tier: i64,
}

/// 指令提交情况（10.11 的 `submitted`）。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Deserialize)]
pub struct Submitted {
    #[serde(default)]
    pub player: bool,
    #[serde(default)]
    pub pet: bool,
}

/// `battle` 查询的结果，或 `enter_world` 响应里的 `battle` 字段（10.11）。
#[derive(Debug, Clone, Default, Deserialize)]
pub struct BattleSnapshot {
    #[serde(default)]
    pub in_battle: bool,
    #[serde(default)]
    pub battle_id: String,
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub scene: String,
    #[serde(default)]
    pub you: u8,
    #[serde(default)]
    pub allow: BattleAllow,
    #[serde(default)]
    pub units: Vec<BattleUnit>,
    #[serde(default)]
    pub turn: u32,
    /// `command`（等出招）或 `replay`（已结算，等播放确认）。
    #[serde(default)]
    pub phase: String,
    #[serde(default, deserialize_with = "null_zero")]
    pub deadline_ms: u64,
    #[serde(default, deserialize_with = "null_zero")]
    pub server_time_ms: u64,
    #[serde(default)]
    pub needs: Vec<String>,
    #[serde(default)]
    pub optional: Vec<String>,
    #[serde(default)]
    pub submitted: Submitted,
    #[serde(default)]
    pub progress: Progress,
    /// 当前场地（STAGE-200），只在场地有属性时带。
    #[serde(default)]
    pub field: Option<BattleField>,
}
