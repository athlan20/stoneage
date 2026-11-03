//! 自动战斗设置（STAGE-173，`docs/auto-battle-settings.md`）：一套按服务器与角色保存的方案，
//! 决定托管出招时人物与战宠做什么、什么情况补血补气、播放怎么走。
//!
//! 这里只有数据结构、默认值和本地文件读写（`store.rs`）；编辑器的光标与草稿在 `edit.rs`，
//! 按键和界面分别在 `app/auto_battle/edit.rs` 与 `ui/autosettings.rs`。托管的出招规则读这份配置
//! （`app/battle/auto.rs`），运行状态（开没开挂机、开没开遇敌）不在这里，也不落盘。

pub mod capture_keep;
pub mod capture_log;
pub mod capture_log_view;
pub mod edit;
mod drop_ask;
mod profiles;
pub mod fields;
pub mod level_input;
pub mod names_input;
pub mod store;

use serde::{Deserialize, Serialize};

/// 方案文件版本。结构变了就加 1；读取时版本更高按“不认识的方案”处理，不覆盖原文件（`store.rs`）。
pub const VERSION: u32 = 2;

/// 人物的常规动作（M01）。精灵用栏位引用，名字只是快照，用来在换装/重学之后认出“这个栏位已经不是那个精灵了”。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ActKind {
    #[default]
    Attack,
    Magic,
    Guard,
}

impl ActKind {
    pub fn label(self) -> &'static str {
        match self {
            Self::Attack => "攻击",
            Self::Magic => "精灵",
            Self::Guard => "防御",
        }
    }

    /// ←→ 在三个动作里循环。
    pub fn step(self, dir: i64) -> Self {
        let all = [Self::Attack, Self::Magic, Self::Guard];
        let i = all.iter().position(|a| *a == self).unwrap_or(0) as i64;
        all[(i + dir).rem_euclid(all.len() as i64) as usize]
    }
}

/// 战宠的常规动作（M02）：宠物只能用攻击、技能、防御（协议 10.5）。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum PetActKind {
    #[default]
    Attack,
    Skill,
    Guard,
}

impl PetActKind {
    pub fn label(self) -> &'static str {
        match self {
            Self::Attack => "攻击",
            Self::Skill => "技能",
            Self::Guard => "防御",
        }
    }

    pub fn step(self, dir: i64) -> Self {
        let all = [Self::Attack, Self::Skill, Self::Guard];
        let i = all.iter().position(|a| *a == self).unwrap_or(0) as i64;
        all[(i + dir).rem_euclid(all.len() as i64) as usize]
    }
}

/// 人物的一个动作配置。`magic_slot` 是精灵栏位 0–4，`magic_name` 是保存时的名字快照。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct PlayerAction {
    pub act: ActKind,
    #[serde(default)]
    pub magic_slot: u8,
    #[serde(default)]
    pub magic_name: String,
}

/// 托管的默认攻击目标（STAGE-173 M01 追加）。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum TargetMode {
    /// 合击顺序：敌方前排从上到下、再后排从上到下，取第一个活着的——人物和战宠打同一个，
    /// 和原版的合击判定对齐（同回合同样目标是合击的前提）。
    #[default]
    Combo,
    /// 随机一个活着的敌人。
    Random,
}

impl TargetMode {
    pub fn label(self) -> &'static str {
        match self {
            Self::Combo => "合击顺序",
            Self::Random => "随机目标",
        }
    }

    pub fn step(self, dir: i64) -> Self {
        let all = [Self::Combo, Self::Random];
        let i = all.iter().position(|m| *m == self).unwrap_or(0) as i64;
        all[(i + dir).rem_euclid(all.len() as i64) as usize]
    }
}

/// 战宠的一个动作配置。`skill_slot` 是宠物页的技能编号 1–7。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct PetAction {
    pub act: PetActKind,
    #[serde(default = "default_skill_slot")]
    pub skill_slot: u8,
    #[serde(default)]
    pub skill_name: String,
}

fn default_skill_slot() -> u8 {
    1
}

/// 首回合策略（M03）：只在真正第 1 回合用一次，之后回到常规动作。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FirstTurn {
    pub enabled: bool,
    pub player: PlayerAction,
    pub pet: PetAction,
}

/// 战斗内恢复（M06 补血、M07 补气）。门槛是按当前值/最大值算的百分比，≤ 门槛时触发。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Recover {
    pub player_heal: bool,
    pub player_heal_pct: u8,
    pub pet_heal: bool,
    pub pet_heal_pct: u8,
    pub player_mp: bool,
    pub player_mp_pct: u8,
    /// 回复来源顺序：`true` 先道具后精灵，`false` 先精灵后道具（默认）。存档里没写这个字段时按 `false`（STAGE-254）。
    #[serde(default)]
    pub item_first: bool,
    /// 补血用的精灵：栏位 + 名字（名字为空表示「自动」，按名字关键字认治愈系）。
    /// 玩家指定后只认这一个（MP 不够就走道具，不偷偷换成别的治愈精灵）。
    #[serde(default)]
    pub heal_magic_slot: u8,
    #[serde(default)]
    pub heal_magic_name: String,
    /// 允许使用的补气物品（M07）：名字包含匹配；空表示不限，按回复量自动挑。
    #[serde(default)]
    pub mp_items: Vec<String>,
    /// 自动复活（STAGE-262 B 段）：出战宠倒下时是否复活它。存档里没写这一项按开。
    #[serde(default = "default_true")]
    pub revive_on: bool,
    /// 复活来源（默认先精灵后道具）。存档里没写这一项按默认。
    #[serde(default)]
    pub revive_source: ReviveSource,
    /// 复活后优先带防御的精灵（复活光 / 镜 / 守护）。存档里没写这一项按开。
    #[serde(default = "default_true")]
    pub revive_defend: bool,
    /// 战后自动吃肉回血（STAGE-260）：战斗结束回到世界后，人物或出战宠血量 ≤ 门槛就吃肉。存档里没写这一项按开。
    #[serde(default = "default_true")]
    pub post_heal_item: bool,
    /// 战后吃肉门槛（%，5–95，步进 5）。存档里没写这一项按 90。
    #[serde(default = "default_post_heal_pct")]
    pub post_heal_pct: u8,
    /// 战后自动丢弃肉类道具（STAGE-260）：默认关；从关切到开要在设置页确认。存档里没写这一项按关。
    #[serde(default)]
    pub post_drop_item: bool,
}

fn default_true() -> bool {
    true
}

fn default_post_heal_pct() -> u8 {
    90
}

/// 复活来源（STAGE-262 B1）。精灵免费、道具是消耗品，所以默认先精灵后道具。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ReviveSource {
    /// 先精灵后道具。
    #[default]
    Auto,
    /// 只用复活精灵，身上没有可用的就不复活。
    MagicOnly,
    /// 只用复活道具，包里没有可用的就不复活。
    ItemOnly,
}

impl ReviveSource {
    pub fn label(self) -> &'static str {
        match self {
            Self::Auto => "自动（先精灵后道具）",
            Self::MagicOnly => "只用精灵",
            Self::ItemOnly => "只用道具",
        }
    }

    pub fn step(self, dir: i64) -> Self {
        let all = [Self::Auto, Self::MagicOnly, Self::ItemOnly];
        let i = all.iter().position(|s| *s == self).unwrap_or(0) as i64;
        all[(i + dir).rem_euclid(all.len() as i64) as usize]
    }
}

impl Recover {
    /// 这件补气道具在不在白名单里（名单为空表示不限）。
    pub fn mp_item_allowed(&self, name: &str) -> bool {
        self.mp_items.is_empty() || self.mp_items.iter().any(|n| name.contains(n.as_str()))
    }
}

/// 自动逃跑（M10）：血量门槛或“敌人名单命中”都算触发条件，两者是或的关系。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Flee {
    pub enabled: bool,
    pub hp_pct: u8,
    /// 指定敌人名单：名字包含匹配。命中就逃，不看血量。
    #[serde(default)]
    pub names: Vec<String>,
}

impl Flee {
    /// 这个敌人名在不在逃跑名单里。
    pub fn name_hit(&self, name: &str) -> bool {
        self.names.iter().any(|n| name.contains(n.as_str()))
    }
}

/// 停止保护（M11）：命中任一条件就停止本机自动出招并请求关闭后续遇敌。0 表示该项不启用。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StopGuard {
    pub no_supply: bool,
    pub player_level: u32,
    pub pet_level: u32,
    pub party_min: u8,
}

/// 抓宠方式（STAGE-285）：自动捕捉开着时，遇到这一场不值得打的怪群怎么办。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum CaptureMode {
    /// 边战斗边抓（默认，与加这一项之前完全一致）：每场都照常打，场上有目标就削血 → 抓。
    #[default]
    Fight,
    /// 逃跑抓：本场没有合格目标就直接逃，专心刷新目标；有目标时照常削血 → 抓，且不逃。
    Escape,
}

impl CaptureMode {
    pub fn label(self) -> &'static str {
        match self {
            Self::Fight => "边战斗边抓",
            Self::Escape => "逃跑抓",
        }
    }

    /// 这一项的说明文字（设置页的小字）。
    pub fn hint(self) -> &'static str {
        match self {
            Self::Fight => "每场都打，遇到目标再抓",
            Self::Escape => "没有目标就逃，专心刷目标",
        }
    }

    pub fn step(self, dir: i64) -> Self {
        let all = [Self::Fight, Self::Escape];
        let i = all.iter().position(|m| *m == self).unwrap_or(0) as i64;
        all[(i + dir).rem_euclid(all.len() as i64) as usize]
    }
}

/// 自动捕捉（M13）：名称与等级作为筛选，削血到门槛后改用捕捉。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Capture {
    pub enabled: bool,
    pub hp_pct: u8,
    pub level_min: u32,
    pub level_max: u32,
    /// 目标名称：包含匹配；空表示不限。
    #[serde(default)]
    pub names: Vec<String>,
    /// 资质筛选（STAGE-264）：抓到后四项都在档位以内才留下；缺省＝全不限。
    #[serde(default)]
    pub keep: capture_keep::CaptureKeep,
    /// 抓宠方式（STAGE-285）：存档里没写这一项时按「边战斗边抓」，与加这一项之前一致。
    #[serde(default)]
    pub mode: CaptureMode,
}

impl Capture {
    /// 名字是否符合筛选（空名单表示不限）。
    pub fn name_ok(&self, name: &str) -> bool {
        self.names.is_empty() || self.names.iter().any(|n| name.contains(n.as_str()))
    }
}

/// 自动换宠（M14）：**本需求不做**（老迪 2026-10-18 决定取消）。
/// 结构体留着只为兼容已经存过的方案文件，界面上不出现，托管也不读它。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Swap {
    pub enabled: bool,
    pub hp_pct: u8,
    #[serde(default)]
    pub order: Vec<u8>,
}

/// 播放（M16）与出招节奏：回放倍速、结果窗口自动继续，以及托管多快出招。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PlaybackPref {
    pub speed: u8,
    pub auto_continue: bool,
    /// 收到 `battle_turn` 后等多久自动出招（毫秒）：给玩家留出手动接管的窗口。
    /// STAGE-173 起可调——默认 500ms，比原来写死的 1500ms 快得多。
    #[serde(default = "default_delay_ms")]
    pub delay_ms: u32,
    /// 玩家动手之后，自动出招顺延多久（毫秒）：一直动手就一直让位。
    #[serde(default = "default_hold_ms")]
    pub manual_hold_ms: u32,
}

fn default_delay_ms() -> u32 {
    500
}

fn default_hold_ms() -> u32 {
    3000
}

impl Default for PlaybackPref {
    fn default() -> Self {
        Self { speed: 1, auto_continue: true, delay_ms: default_delay_ms(), manual_hold_ms: default_hold_ms() }
    }
}

/// 出招延迟的可选档位（毫秒）。用档位而不是任意数值：玩家不用去猜 137ms 和 500ms 有什么区别。
/// 第一档是 **0＝即时**：收到回合结算就发下一条指令，不等（战斗里刷新间隔 33ms，最多差一帧）。
pub const DELAY_STEPS: [u32; 6] = [0, 300, 500, 1000, 1500, 3000];
/// 手动让位时间（毫秒）。
pub const HOLD_STEPS: [u32; 5] = [1000, 2000, 3000, 5000, 8000];

impl PlaybackPref {
    pub fn delay(&self) -> std::time::Duration {
        // 下限是 0：设成「即时」时不该被夹回 50ms。
        std::time::Duration::from_millis(self.delay_ms.min(10_000) as u64)
    }

    pub fn manual_hold(&self) -> std::time::Duration {
        std::time::Duration::from_millis(self.manual_hold_ms.clamp(200, 30_000) as u64)
    }

    /// ←→ 在档位之间走；不在档位里的值（手改过方案文件）先落到最近的档位。
    pub fn step_delay(&mut self, dir: i64) {
        self.delay_ms = step_in(&DELAY_STEPS, self.delay_ms, dir);
    }

    pub fn step_hold(&mut self, dir: i64) {
        self.manual_hold_ms = step_in(&HOLD_STEPS, self.manual_hold_ms, dir);
    }
}

/// 在档位表里走一格；当前值不在表里时按“最近的档位”算起点。
fn step_in(steps: &[u32], cur: u32, dir: i64) -> u32 {
    let n = steps.len() as i64;
    let at = steps
        .iter()
        .position(|s| *s == cur)
        .unwrap_or_else(|| {
            let nearest = steps
                .iter()
                .enumerate()
                .min_by_key(|(_, s)| s.abs_diff(cur))
                .map(|(i, _)| i)
                .unwrap_or(0);
            nearest
        }) as i64;
    steps[(at + dir).rem_euclid(n) as usize]
}

/// 一整套自动战斗设置。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AutoProfile {
    pub name: String,
    /// 默认攻击目标（STAGE-173）：合击顺序 / 随机。
    #[serde(default)]
    pub target: TargetMode,
    pub player: PlayerAction,
    pub pet: PetAction,
    pub first: FirstTurn,
    pub recover: Recover,
    pub flee: Flee,
    pub stop: StopGuard,
    pub capture: Capture,
    pub swap: Swap,
    pub playback: PlaybackPref,
    /// 打输后提示并停止（STAGE-265）：挂机或无限遇敌在跑时打输，响提示音并停止挂机和遇敌。存档里没写时按开（`default_stop_on_lose`）。
    #[serde(default = "default_stop_on_lose")]
    pub stop_on_lose: bool,
}

/// 打输后提示并停止的默认值：开（STAGE-265）。旧存档缺这个字段时就是它。
pub fn default_stop_on_lose() -> bool {
    true
}

impl Default for AutoProfile {
    fn default() -> Self {
        Self {
            name: "默认方案".into(),
            target: TargetMode::Combo,
            player: PlayerAction::default(),
            pet: PetAction::default(),
            first: FirstTurn {
                enabled: false,
                player: PlayerAction::default(),
                pet: PetAction { skill_slot: 1, ..PetAction::default() },
            },
            recover: Recover {
                player_heal: true,
                player_heal_pct: 30,
                pet_heal: true,
                pet_heal_pct: 30,
                player_mp: false,
                player_mp_pct: 20,
                item_first: false,
                mp_items: Vec::new(),
                heal_magic_slot: 0,
                heal_magic_name: String::new(),
                revive_on: true,
                revive_source: ReviveSource::Auto,
                revive_defend: true,
                post_heal_item: true,
                post_heal_pct: 90,
                post_drop_item: false,
            },
            flee: Flee { enabled: false, hp_pct: 15, names: Vec::new() },
            stop: StopGuard { no_supply: true, player_level: 0, pet_level: 0, party_min: 0 },
            capture: Capture { enabled: false, hp_pct: 20, level_min: 1, level_max: 0, names: Vec::new(), keep: capture_keep::CaptureKeep::UNLIMITED, mode: CaptureMode::Fight },
            swap: Swap { enabled: false, hp_pct: 20, order: Vec::new() },
            playback: PlaybackPref::default(),
            stop_on_lose: true,
        }
    }
}

impl AutoProfile {
    /// 一句话概括，方案列表里显示。
    pub fn summary(&self) -> String {
        let mut bits = vec![format!("人物{}", self.player.act.label()), format!("宠物{}", self.pet.act.label())];
        if self.recover.player_heal {
            bits.push(format!("补血≤{}%", self.recover.player_heal_pct));
        }
        if self.capture.enabled {
            bits.push("捕捉".into());
        }
        if self.flee.enabled {
            bits.push(format!("逃跑≤{}%", self.flee.hp_pct));
        }
        bits.join(" · ")
    }
}
