//! 战斗里的阶段、按键选择和飘字的类型（`docs/battle-ui.md`「指令输入」「结算回放」）。

use std::time::{Duration, Instant};

use super::Act;
use super::command::Actor;
use crate::proto::TargetType;

/// 战斗阶段：谁该下指令、是否在等别人、是否在播放结算。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Phase {
    /// 轮到这个角色下指令。
    Input(Actor),
    /// 已经交完指令，等其他玩家或服务端结算。
    Waiting,
    /// 正在播放上一回合的结算。
    Replay,
    /// 战斗已经结束，结果窗口显示中。
    Ended,
}

/// 指令条上正在选什么（`docs/battle-ui.md`「三种操作方式」）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Pick {
    /// 指令条上的动作（人物或宠物，看当前阶段）。
    Action,
    /// 道具：背包里的物品。
    Item,
    /// 换宠：宠物栏位。
    Pet,
    /// 精灵（魔法）：人物的精灵栏位（0–4）。
    Magic,
    /// 宠物技能。
    Skill,
    /// 选目标；`extra` 是道具槽位、精灵或技能栏位，提交时一起带上；`kind` 是这一项的目标类型，
    /// 决定只能选哪一方（`docs/battle-ui.md`「指令输入」，STAGE-78）。
    Target { act: Act, extra: Option<i64>, kind: TargetType },
    /// 分身面板：列出能指定目标的分身，选一个去指定（STAGE-186，`clone.rs`）。
    Clones,
    /// 给分身 `who`（单位号）选攻击目标：只能选敌方活着的单位。
    CloneAim { who: u8 },
}

impl Pick {
    /// 是不是列表面板在显示的选择（道具、精灵、换宠、技能）；选目标和指令条不是。
    pub fn is_list(self) -> bool {
        matches!(self, Self::Item | Self::Magic | Self::Pet | Self::Skill)
    }
}

/// 单位身上的飘字种类，界面按它选颜色。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FloatKind {
    Damage,
    Crit,
    Miss,
    Heal,
    Block,
}

/// 单位身上的飘字（`-23`、`miss`、`+80`），到 `until` 为止。
#[derive(Debug, Clone)]
pub struct Float {
    pub no: u8,
    pub text: String,
    pub kind: FloatKind,
    pub until: Instant,
}

/// 字幕里一段文字的颜色类别（界面按它选颜色，`docs/battle-ui.md`「中线字幕」）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Tone {
    /// 己方的名字（绿）。
    Mine,
    /// 敌方的名字（红）。
    Foe,
    /// 动作（黄色加粗）。
    Verb,
    /// 伤害数字（红）。
    Hurt,
    /// 暴击（黄色加粗）。
    Crit,
    /// 回复（绿）。
    Heal,
    /// 捕捉、逃跑成功（绿）。
    Good,
    /// 捕捉、逃跑失败、倒下（红）。
    Bad,
    /// 状态标记（品红）。
    Status,
    /// 闪避、未命中、全防、不能行动的原因（灰）。
    Dim,
    /// 连接符和普通文字。
    Plain,
}

/// 字幕的一段文字。`shrink` 是放不下时先截的优先级：名字用 2（目标）或 1（行动者），其余为 0。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Part {
    pub text: String,
    pub tone: Tone,
    pub shrink: u8,
}

impl Part {
    pub fn new(text: impl Into<String>, tone: Tone) -> Self {
        Self { text: text.into(), tone, shrink: 0 }
    }

    pub fn name(text: impl Into<String>, tone: Tone, shrink: u8) -> Self {
        Self { text: text.into(), tone, shrink }
    }
}

/// 命中拍在单位身上的效果种类（`docs/battle-ui.md`「卡片效果」）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FxKind {
    /// 受伤：红色反色闪，文字左右抖一格。
    Hurt,
    /// 暴击：同受伤，抖动更明显。
    Crit,
    /// 闪避、未命中：文字往远离行动者的方向让一格（-1 或 +1）。
    Dodge(i8),
    /// 全防：白色闪一下。
    Block,
    /// 回复：绿色闪。
    Heal,
    /// 倒下：先红色闪，闪完变灰。
    Down,
    /// 新增状态：品红色闪两下。
    Status,
}

/// 单位身上正在进行的效果，从 `from` 到 `until`；闪和抖按 `slot` 切成一段一段。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Fx {
    pub no: u8,
    pub kind: FxKind,
    pub from: Instant,
    pub until: Instant,
    pub slot: Duration,
}

/// 血条的移动：从 `from` 到 `to`，在 `start` 到 `until` 之间完成。伤害时 `to` 立即生效，多出来的一段画成残影慢慢缩掉；
/// 回复时血条从 `from` 涨到 `to`。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct HpMove {
    pub no: u8,
    pub from: u32,
    pub to: u32,
    pub start: Instant,
    pub until: Instant,
}
