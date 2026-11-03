//! 五栏设置的行表（STAGE-173「入口与交互」）：一栏一串 `Field`，渲染和按键都遍历它，
//! 加一个设置项只要在这里加一个变体 + 三条 match 分支，不用同时改界面和按键。
//!
//! 五栏全部已接进托管决策（M14 自动换宠被老迪取消，界面上没有这一项）；只有「连续遇敌」
//! 和「遇敌区域」是只读展示（M09），值和开关都在系统页/场景页改。

use super::{
    ActKind, AutoProfile, PetActKind,
};
use super::capture_keep::{self, Stat};
use crate::proto::TargetType;

/// 编辑时能看到的角色现状：可选精灵、出战宠与技能、等级、队伍人数。全是只读快照，
/// 由 `WorldState` 现算，避免在借用编辑器时又去借世界状态。
#[derive(Debug, Clone, Default)]
pub struct Context {
    /// 身上的精灵：栏位、名字、消耗 MP、目标类型（判断能不能用来补血）。
    pub magics: Vec<(u8, String, u32, TargetType)>,
    /// 出战宠可用的技能：栏位、名字。
    pub pet_skills: Vec<(u8, String)>,
    /// 出战宠的名字；没有出战宠时为空。
    pub pet_name: String,
    /// 人物等级、队伍人数。
    pub player_level: u32,
    pub party_size: usize,
    /// 当前战斗中则为本回合号（首回合动作只在第 1 回合生效，界面可以据此提示）。
    pub turn: Option<u32>,
    /// 连续遇敌的真实开关（M09 只读展示）：开启 / 关闭 / 正在关闭。
    pub encounter_on: bool,
    pub encounter_busy: bool,
    /// 当前选中的遇敌区域（场景页选过才有；没加载时为空）。
    pub encounter_area: String,
}

/// 设置页的一项。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Field {
    // 行动
    TargetMode,
    PlayerAct,
    PlayerMagic,
    FirstOn,
    FirstPlayerAct,
    FirstPlayerMagic,
    PetAct,
    PetSkill,
    FirstPetAct,
    FirstPetSkill,
    // 恢复
    PlayerHeal,
    PlayerHealPct,
    /// 补血用的精灵（自动 / 指定某一个）。
    PlayerHealMagic,
    PetHeal,
    PetHealPct,
    PlayerMp,
    PlayerMpPct,
    /// 允许使用的补气物品（名单，Enter 编辑）。
    MpItems,
    ItemFirst,
    /// 自动复活（STAGE-262 B 段）：出战宠倒下时复活它。
    ReviveOn,
    /// 复活来源：自动（先精灵后道具）/ 只用精灵 / 只用道具。
    ReviveSource,
    /// 复活后优先带防御的精灵（复活光 / 镜 / 守护）。
    ReviveDefend,
    /// 战后自动吃肉回血（STAGE-260）：开 / 关。
    PostHeal,
    /// 战后吃肉门槛（人物或出战宠血量 ≤ 它就吃）。
    PostHealPct,
    /// 战后自动丢弃肉类道具（STAGE-260）：开 / 关，从关切到开要确认。
    PostDrop,
    /// 打输后提示并停止（STAGE-265）。
    StopOnLose,
    // 挂机
    /// 连续遇敌的真实开关（只读，M09）。
    EncounterOn,
    /// 当前遇敌区域（只读，M09）。
    EncounterArea,
    FleeOn,
    FleePct,
    /// 指定逃跑的敌人名单（Enter 编辑）。
    FleeNames,
    StopNoSupply,
    StopPlayerLevel,
    StopPetLevel,
    StopPartyMin,
    // 捕捉
    CaptureOn,
    /// 抓宠方式（STAGE-285）：边战斗边抓 / 逃跑抓。
    CaptureMode,
    CapturePct,
    /// 捕捉目标的名称筛选（Enter 编辑，空表示不限）。
    CaptureNames,
    CaptureLvMin,
    CaptureLvMax,
    /// 资质筛选（STAGE-264）：四项各一行，档位 不限 / 0 / −1 / −2 / −3。
    CaptureKeep(Stat),
    /// 抓宠丢弃记录（STAGE-264）：Enter 打开只读列表。
    CaptureLog,
    // 方案与播放
    ProfilePick,
    ProfileNew,
    ProfileDelete,
    SaveApply,
    DiscardDraft,
    AutoContinue,
    /// 出招延迟：`battle_turn` 后等多久自动出招。
    Delay,
    /// 玩家动手之后托管让位多久。
    ManualHold,
}

/// 五栏的名字与内容。
pub fn columns() -> [(&'static str, &'static [Field]); 5] {
    [
        ("行动", &ACTIONS[..]),
        ("恢复", &RECOVER[..]),
        ("挂机", &IDLE[..]),
        ("捕捉", &CAPTURE[..]),
        ("方案与播放", &PROFILE[..]),
    ]
}

/// 某一栏的项（越界按最后一栏算）。
pub fn column_fields(column: usize) -> &'static [Field] {
    let cols = columns();
    cols[column.min(cols.len() - 1)].1
}

const ACTIONS: [Field; 10] = [
    Field::TargetMode,
    Field::PlayerAct,
    Field::PlayerMagic,
    Field::FirstOn,
    Field::FirstPlayerAct,
    Field::FirstPlayerMagic,
    Field::PetAct,
    Field::PetSkill,
    Field::FirstPetAct,
    Field::FirstPetSkill,
];
const RECOVER: [Field; 16] = [
    Field::PlayerHeal,
    Field::PlayerHealPct,
    Field::PlayerHealMagic,
    Field::PetHeal,
    Field::PetHealPct,
    Field::PlayerMp,
    Field::PlayerMpPct,
    Field::MpItems,
    Field::ItemFirst,
    Field::ReviveOn,
    Field::ReviveSource,
    Field::ReviveDefend,
    Field::PostHeal,
    Field::PostHealPct,
    Field::PostDrop,
    Field::StopOnLose,
];
const IDLE: [Field; 9] = [
    Field::EncounterOn,
    Field::EncounterArea,
    Field::FleeOn,
    Field::FleePct,
    Field::FleeNames,
    Field::StopNoSupply,
    Field::StopPlayerLevel,
    Field::StopPetLevel,
    Field::StopPartyMin,
];
const CAPTURE: [Field; 11] = [
    Field::CaptureOn,
    Field::CaptureMode,
    Field::CapturePct,
    Field::CaptureNames,
    Field::CaptureLvMin,
    Field::CaptureLvMax,
    Field::CaptureKeep(Stat::Vital),
    Field::CaptureKeep(Stat::Strength),
    Field::CaptureKeep(Stat::Toughness),
    Field::CaptureKeep(Stat::Dexterity),
    Field::CaptureLog,
];
const PROFILE: [Field; 8] = [
    Field::ProfilePick,
    Field::ProfileNew,
    Field::ProfileDelete,
    Field::SaveApply,
    Field::DiscardDraft,
    Field::AutoContinue,
    Field::Delay,
    Field::ManualHold,
];

impl Field {
    pub fn label(self) -> &'static str {
        match self {
            Self::TargetMode => "默认攻击目标",
            Self::PlayerAct => "人物常规动作",
            Self::PlayerMagic => "人物精灵",
            Self::FirstOn => "首回合单独设置",
            Self::FirstPlayerAct => "首回合人物动作",
            Self::FirstPlayerMagic => "首回合人物精灵",
            Self::PetAct => "战宠常规动作",
            Self::PetSkill => "战宠技能",
            Self::FirstPetAct => "首回合战宠动作",
            Self::FirstPetSkill => "首回合战宠技能",
            Self::PlayerHeal => "人物自动补血",
            Self::PlayerHealPct => "人物补血门槛",
            Self::PlayerHealMagic => "补血精灵",
            Self::PetHeal => "战宠自动补血",
            Self::PetHealPct => "战宠补血门槛",
            Self::PlayerMp => "人物自动补气",
            Self::PlayerMpPct => "人物补气门槛",
            Self::MpItems => "补气物品",
            Self::ItemFirst => "回复来源顺序",
            Self::ReviveOn => "自动复活",
            Self::ReviveSource => "复活来源",
            Self::ReviveDefend => "复活后优先带防御的精灵",
            Self::PostHeal => "战后自动吃肉回血",
            Self::PostHealPct => "战后吃肉门槛",
            Self::PostDrop => "战后自动丢弃肉类道具",
            Self::StopOnLose => "打输后提示并停止",
            Self::EncounterOn => "连续遇敌",
            Self::EncounterArea => "遇敌区域",
            Self::FleeOn => "自动逃跑",
            Self::FleePct => "逃跑血量门槛",
            Self::FleeNames => "逃跑敌人名单",
            Self::StopNoSupply => "补给不足时停止",
            Self::StopPlayerLevel => "人物目标等级",
            Self::StopPetLevel => "战宠目标等级",
            Self::StopPartyMin => "队伍最低人数",
            Self::CaptureOn => "自动捕捉",
            Self::CaptureMode => "抓宠方式",
            Self::CapturePct => "削血到",
            Self::CaptureNames => "捕捉目标名称",
            Self::CaptureLvMin => "捕捉等级下限",
            Self::CaptureLvMax => "捕捉等级上限",
            Self::CaptureKeep(stat) => stat.label(),
            Self::CaptureLog => "抓宠丢弃记录",
            Self::ProfilePick => "当前方案",
            Self::ProfileNew => "新建方案",
            Self::ProfileDelete => "删除方案",
            Self::SaveApply => "保存并应用",
            Self::DiscardDraft => "放弃改动",

            Self::AutoContinue => "结果窗口自动继续",
            Self::Delay => "出招延迟",
            Self::ManualHold => "手动让位",
        }
    }

    /// 名单类设置（逃跑敌人、捕捉目标、补气物品）：Enter 打开行内名单编辑器。
    pub fn is_name_list(self) -> bool {
        matches!(self, Self::FleeNames | Self::CaptureNames | Self::MpItems)
    }

    /// 只读行：值来自世界状态，不能在这里改。
    pub fn read_only(self) -> bool {
        matches!(self, Self::EncounterOn | Self::EncounterArea)
    }

    /// 名单类设置对应的名单（`draft` 里那一份）。不是名单项时为 `None`。
    pub fn name_list(draft: &mut AutoProfile, f: Field) -> Option<&mut Vec<String>> {
        match f {
            Field::FleeNames => Some(&mut draft.flee.names),
            Field::CaptureNames => Some(&mut draft.capture.names),
            Field::MpItems => Some(&mut draft.recover.mp_items),
            _ => None,
        }
    }

    /// 当前值的显示文字。`sub` 是括号里的小字（来源、限制说明）。
    pub fn value(self, p: &AutoProfile, ctx: &Context) -> (String, String) {
        let onoff = |b: bool| if b { "开".to_string() } else { "关".to_string() };
        match self {
            Self::TargetMode => (
                p.target.label().into(),
                match p.target {
                    super::TargetMode::Combo => "前排从上到下，再后排；人物和战宠打同一个".into(),
                    super::TargetMode::Random => "每个行动者各自随机".into(),
                },
            ),
            Self::PlayerAct => (p.player.act.label().into(), String::new()),
            Self::PlayerMagic => magic_text(p.player.act == ActKind::Magic, p.player.magic_slot, &p.player.magic_name, ctx),
            Self::FirstOn => (onoff(p.first.enabled), if p.first.enabled { String::new() } else { "沿用常规动作".into() }),
            Self::FirstPlayerAct => {
                if p.first.enabled { (p.first.player.act.label().into(), String::new()) } else { (p.player.act.label().into(), "未启用".into()) }
            }
            Self::FirstPlayerMagic => magic_text(
                p.first.enabled && p.first.player.act == ActKind::Magic,
                p.first.player.magic_slot,
                &p.first.player.magic_name,
                ctx,
            ),
            Self::PetAct => (p.pet.act.label().into(), if ctx.pet_name.is_empty() { "没有出战宠".into() } else { String::new() }),
            Self::PetSkill => pet_skill_text(p.pet.act == PetActKind::Skill, p.pet.skill_slot, &p.pet.skill_name, ctx),
            Self::FirstPetAct => {
                if p.first.enabled { (p.first.pet.act.label().into(), String::new()) } else { (p.pet.act.label().into(), "未启用".into()) }
            }
            Self::FirstPetSkill => pet_skill_text(
                p.first.enabled && p.first.pet.act == PetActKind::Skill,
                p.first.pet.skill_slot,
                &p.first.pet.skill_name,
                ctx,
            ),
            Self::PlayerHeal => (onoff(p.recover.player_heal), String::new()),
            Self::PlayerHealPct => (format!("≤ {}%", p.recover.player_heal_pct), String::new()),
            Self::PlayerHealMagic => heal_magic_text(p, ctx),
            Self::PetHeal => (onoff(p.recover.pet_heal), String::new()),
            Self::PetHealPct => (format!("≤ {}%", p.recover.pet_heal_pct), String::new()),
            Self::PlayerMp => (onoff(p.recover.player_mp), String::new()),
            Self::PlayerMpPct => (format!("≤ {}%", p.recover.player_mp_pct), String::new()),
            Self::MpItems => (list_text(&p.recover.mp_items), "Enter 改名单；「不限」＝按回复量自动挑".into()),
            Self::ItemFirst => {
                (if p.recover.item_first { "先道具".into() } else { "先精灵".into() }, String::new())
            }
            Self::ReviveOn => (onoff(p.recover.revive_on), "出战宠倒下时复活它".into()),
            Self::ReviveSource => (p.recover.revive_source.label().into(), String::new()),
            Self::ReviveDefend => (onoff(p.recover.revive_defend), "复活光 / 镜 / 守护优先".into()),
            Self::PostHeal => (onoff(p.recover.post_heal_item), "人物或出战宠血量 ≤ 门槛就吃肉".into()),
            Self::PostHealPct => (format!("≤ {}%", p.recover.post_heal_pct), String::new()),
            Self::PostDrop => (onoff(p.recover.post_drop_item), "吃完后丢光背包里的肉；开启要确认".into()),
            Self::StopOnLose => (onoff(p.stop_on_lose), "响提示音，并停止自动挂机与自动遇敌".into()),
            Self::EncounterOn => (
                (if ctx.encounter_busy {
                    "正在关闭"
                } else if ctx.encounter_on {
                    "开启"
                } else {
                    "关闭"
                })
                .to_string(),
                "只读：在系统页或场景页改".into(),
            ),
            Self::EncounterArea => (
                if ctx.encounter_area.is_empty() { "未选择".into() } else { ctx.encounter_area.clone() },
                "只读：场景页 → 遇敌选区".into(),
            ),
            Self::FleeOn => (onoff(p.flee.enabled), String::new()),
            Self::FleePct => (format!("≤ {}%", p.flee.hp_pct), "自身血量".into()),
            Self::FleeNames => (list_text(&p.flee.names), "Enter 改名单；命中就逃，不看血量".into()),
            Self::StopNoSupply => (onoff(p.stop.no_supply), "回复用品用完".into()),
            Self::StopPlayerLevel => (level_text(p.stop.player_level), String::new()),
            Self::StopPetLevel => (level_text(p.stop.pet_level), String::new()),
            Self::StopPartyMin => {
                if p.stop.party_min == 0 { ("不限".into(), String::new()) } else { (format!("≥ {} 人", p.stop.party_min), String::new()) }
            }
            Self::CaptureOn => (onoff(p.capture.enabled), String::new()),
            Self::CaptureMode => (p.capture.mode.label().into(), p.capture.mode.hint().into()),
            Self::CapturePct if p.capture.hp_pct >= 100 => {
                ("满血抓".into(), "不削血，直接抓；高血量目标几乎抓不到".into())
            }
            Self::CapturePct => (format!("≤ {}%", p.capture.hp_pct), "目标血量".into()),
            Self::CaptureNames => (list_text(&p.capture.names), "Enter 改名单；「不限」＝只按等级筛".into()),
            Self::CaptureKeep(stat) => capture_keep::value_text(&p.capture.keep, stat),
            Self::CaptureLog => ("Enter 查看".into(), "只读，每角色最近 50 条".into()),
            Self::CaptureLvMin => (
                if p.capture.level_min == 0 { "不限".into() } else { format!("≥ {}", p.capture.level_min) },
                "Enter 手输 0–200".into(),
            ),
            Self::CaptureLvMax => (
                if p.capture.level_max == 0 { "不限".into() } else { format!("≤ {}", p.capture.level_max) },
                "Enter 手输 0–200".into(),
            ),
            Self::ProfilePick => (p.name.clone(), String::new()),
            Self::ProfileNew => ("Enter 新建".into(), String::new()),
            Self::ProfileDelete => ("Enter 删除".into(), if p.name.is_empty() { "不能删".into() } else { "需确认".into() }),
            Self::SaveApply => ("Enter 保存并应用".into(), String::new()),
            Self::DiscardDraft => ("Enter 放弃".into(), String::new()),

            Self::AutoContinue => (onoff(p.playback.auto_continue), "结果窗口 2.5 秒后自动关".into()),
            Self::Delay => {
                let v = if p.playback.delay_ms == 0 { "即时".to_string() } else { format!("{} ms", p.playback.delay_ms) };
                (v, pace_label(p.playback.delay_ms).into())
            }
            Self::ManualHold => (format!("{} ms", p.playback.manual_hold_ms), "按一下键就顺延这么久".into()),
        }
    }

    /// ←→ 调整这一项。返回 `Some(说明)` 表示没改成（用于提示），`None` 表示改好了或这一项要用 Enter。
    pub fn adjust(self, p: &mut AutoProfile, dir: i64, ctx: &Context) -> Option<String> {
        match self {
            Self::TargetMode => p.target = p.target.step(dir),
            Self::PlayerAct => p.player.act = p.player.act.step(dir),
            Self::PlayerMagic => return cycle_magic(&mut p.player.magic_slot, &mut p.player.magic_name, dir, ctx),
            Self::FirstOn => p.first.enabled = !p.first.enabled,
            Self::FirstPlayerAct => p.first.player.act = p.first.player.act.step(dir),
            Self::FirstPlayerMagic => {
                return cycle_magic(&mut p.first.player.magic_slot, &mut p.first.player.magic_name, dir, ctx);
            }
            Self::PetAct => p.pet.act = p.pet.act.step(dir),
            Self::PetSkill => return cycle_pet_skill(&mut p.pet.skill_slot, &mut p.pet.skill_name, dir, ctx),
            Self::FirstPetAct => p.first.pet.act = p.first.pet.act.step(dir),
            Self::FirstPetSkill => {
                return cycle_pet_skill(&mut p.first.pet.skill_slot, &mut p.first.pet.skill_name, dir, ctx);
            }
            Self::PlayerHeal => p.recover.player_heal = !p.recover.player_heal,
            Self::PlayerHealPct => p.recover.player_heal_pct = pct_step(p.recover.player_heal_pct, dir),
            Self::PlayerHealMagic => {
                return cycle_heal_magic(
                    &mut p.recover.heal_magic_slot,
                    &mut p.recover.heal_magic_name,
                    dir,
                    ctx,
                );
            }
            Self::PetHeal => p.recover.pet_heal = !p.recover.pet_heal,
            Self::PetHealPct => p.recover.pet_heal_pct = pct_step(p.recover.pet_heal_pct, dir),
            Self::PlayerMp => p.recover.player_mp = !p.recover.player_mp,
            Self::PlayerMpPct => p.recover.player_mp_pct = pct_step(p.recover.player_mp_pct, dir),
            Self::ItemFirst => p.recover.item_first = !p.recover.item_first,
            Self::ReviveOn => p.recover.revive_on = !p.recover.revive_on,
            Self::ReviveSource => p.recover.revive_source = p.recover.revive_source.step(dir),
            Self::ReviveDefend => p.recover.revive_defend = !p.recover.revive_defend,
            Self::PostHeal => p.recover.post_heal_item = !p.recover.post_heal_item,
            Self::PostHealPct => p.recover.post_heal_pct = pct_step(p.recover.post_heal_pct, dir),
            Self::PostDrop => p.recover.post_drop_item = !p.recover.post_drop_item,
            Self::StopOnLose => p.stop_on_lose = !p.stop_on_lose,
            Self::FleeOn => p.flee.enabled = !p.flee.enabled,
            Self::FleePct => p.flee.hp_pct = pct_step(p.flee.hp_pct, dir),
            Self::StopNoSupply => p.stop.no_supply = !p.stop.no_supply,
            Self::StopPlayerLevel => p.stop.player_level = level_step(p.stop.player_level, dir),
            Self::StopPetLevel => p.stop.pet_level = level_step(p.stop.pet_level, dir),
            Self::StopPartyMin => p.stop.party_min = (i64::from(p.stop.party_min) + dir).clamp(0, 10) as u8,
            Self::CaptureOn => p.capture.enabled = !p.capture.enabled,
            Self::CaptureMode => p.capture.mode = p.capture.mode.step(dir),
            Self::CapturePct => p.capture.hp_pct = capture_pct_step(p.capture.hp_pct, dir),
            Self::CaptureLvMin => p.capture.level_min = level_step(p.capture.level_min, dir),
            Self::CaptureLvMax => p.capture.level_max = level_step(p.capture.level_max, dir),
            Self::CaptureKeep(stat) => p.capture.keep.step(stat, dir),
            Self::CaptureLog => {}

            Self::AutoContinue => p.playback.auto_continue = !p.playback.auto_continue,
            Self::Delay => p.playback.step_delay(dir),
            Self::ManualHold => p.playback.step_hold(dir),
            Self::MpItems | Self::FleeNames | Self::CaptureNames => {
                return Some("这一项按 Enter 编辑名单".into());
            }
            Self::EncounterOn | Self::EncounterArea => {
                return Some("只读项：在系统页或场景页修改".into());
            }
            Self::ProfilePick | Self::ProfileNew | Self::ProfileDelete | Self::SaveApply | Self::DiscardDraft => {
                return Some("这一项按 Enter".into());
            }
        }
        None
    }

    /// 这一项现在能不能改（禁用的行在界面上变灰，Enter 说明原因）。
    pub fn blocked(self, p: &AutoProfile, ctx: &Context) -> Option<&'static str> {
        match self {
            Self::PlayerMagic if p.player.act != ActKind::Magic => Some("人物动作不是「精灵」"),
            Self::FirstPlayerMagic if !p.first.enabled || p.first.player.act != ActKind::Magic => {
                Some("首回合没开，或首回合动作不是「精灵」")
            }
            Self::PetSkill if p.pet.act != PetActKind::Skill => Some("战宠动作不是「技能」"),
            Self::FirstPetSkill if !p.first.enabled || p.first.pet.act != PetActKind::Skill => {
                Some("首回合没开，或首回合战宠动作不是「技能」")
            }
            Self::PlayerHealPct | Self::PlayerHealMagic if !p.recover.player_heal => Some("人物自动补血关着"),
            Self::PlayerHealMagic if ctx.magics.is_empty() => Some("身上没有精灵"),
            Self::PetHealPct if !p.recover.pet_heal => Some("战宠自动补血关着"),
            Self::PlayerMpPct if !p.recover.player_mp => Some("人物自动补气关着"),
            Self::MpItems if !p.recover.player_mp => Some("人物自动补气关着"),
            Self::ReviveSource | Self::ReviveDefend if !p.recover.revive_on => Some("自动复活关着"),
            Self::PostHealPct if !p.recover.post_heal_item => Some("战后自动吃肉关着"),
            Self::FleePct if !p.flee.enabled => Some("自动逃跑关着"),
            Self::FleeNames if !p.flee.enabled => Some("自动逃跑关着"),
            Self::CaptureNames if !p.capture.enabled => Some("自动捕捉关着"),
            Self::EncounterOn | Self::EncounterArea => Some("只读：在系统页或场景页改"),
            Self::CapturePct | Self::CaptureLvMin | Self::CaptureLvMax | Self::CaptureKeep(_) | Self::CaptureMode if !p.capture.enabled => {
                Some("自动捕捉关着")
            }
            Self::PlayerMagic | Self::FirstPlayerMagic if ctx.magics.is_empty() => Some("还没有学会精灵"),
            Self::PetSkill | Self::FirstPetSkill if ctx.pet_skills.is_empty() => Some("出战宠没有可用技能"),
            Self::FirstPlayerAct | Self::FirstPlayerMagic | Self::FirstPetAct | Self::FirstPetSkill
                if !p.first.enabled =>
            {
                Some("首回合单独设置关着")
            }
            Self::ProfileDelete if p.name.is_empty() => Some("没有可删的方案"),
            _ => None,
        }
    }
}

/// 延迟档位的人话标签：玩家不用记毫秒数。
fn pace_label(ms: u32) -> &'static str {
    match ms {
        0 => "即时：收到回合结算就出招，不等",
        1..=150 => "极快（几乎立刻出招）",
        151..=400 => "很快",
        401..=700 => "快",
        701..=1200 => "标准",
        1201..=2000 => "慢",
        _ => "很慢",
    }
}

/// 名单的显示：空名单是「不限」，太长就省略成「…共 N 项」。
fn list_text(names: &[String]) -> String {
    match names.len() {
        0 => "不限".into(),
        1..=3 => names.join("、"),
        n => format!("{}…（共 {n} 项）", names[..2].join("、")),
    }
}

fn level_text(v: u32) -> String {
    if v == 0 { "不限".into() } else { format!("{v} 级") }
}

fn pct_step(v: u8, dir: i64) -> u8 {
    (i64::from(v) + dir * 5).clamp(5, 95) as u8
}

/// 捕捉的削血档位：5–95 按 5 走，再往上多一档「满血抓」（100，不削血直接抓）。
fn capture_pct_step(v: u8, dir: i64) -> u8 {
    (i64::from(v) + dir * 5).clamp(5, 100) as u8
}

fn level_step(v: u32, dir: i64) -> u32 {
    (i64::from(v) + dir * 5).clamp(0, 200) as u32
}

/// 补血精灵这一行：空名字是「自动」，指定了就把名字与 MP 写出来，失效了就提示重选。
fn heal_magic_text(p: &AutoProfile, ctx: &Context) -> (String, String) {
    if p.recover.heal_magic_name.is_empty() {
        return ("自动".into(), "按效果认回复精灵（回复自己/己方 HP，含恩惠）".into());
    }
    match ctx.magics.iter().find(|(s, _, _, _)| *s == p.recover.heal_magic_slot) {
        Some((_, n, mp, kind)) if *n == p.recover.heal_magic_name => (
            n.clone(),
            match kind {
                TargetType::Enemy | TargetType::EnemyAll | TargetType::AllyDown => {
                    "这个精灵不能补血，改成别的或「自动」".into()
                }
                _ => format!("{mp} MP"),
            },
        ),
        _ => (
            format!("{}（已失效）", p.recover.heal_magic_name),
            "按最新精灵列表重新选".into(),
        ),
    }
}

/// 补血精灵在「自动 → 身上每个精灵 → 自动」之间循环；`←` 从自动直接到最后一个。
fn cycle_heal_magic(slot: &mut u8, name: &mut String, dir: i64, ctx: &Context) -> Option<String> {
    let n = ctx.magics.len() as i64 + 1;
    let cur = if name.is_empty() {
        0
    } else {
        ctx.magics
            .iter()
            .position(|(s, nm, _, _)| *s == *slot && nm == name)
            .map_or(0, |i| i as i64 + 1)
    };
    let next = (cur + dir).rem_euclid(n);
    if next == 0 {
        name.clear();
        *slot = 0;
    } else {
        let (s, nm, _, _) = &ctx.magics[(next - 1) as usize];
        *slot = *s;
        *name = nm.clone();
    }
    None
}

fn magic_text(on: bool, slot: u8, name: &str, ctx: &Context) -> (String, String) {
    if !on {
        return (format!("栏位 {}", slot + 1), "人物动作不是「精灵」".into());
    }
    match ctx.magics.iter().find(|(s, _, _, _)| *s == slot) {
        Some((_, n, mp, _)) => (n.clone(), format!("{mp} MP")),
        None if name.is_empty() => ("未选择".into(), "还没有学会精灵".into()),
        None => (format!("{name}（已失效）"), "按最新精灵列表重新选".into()),
    }
}

fn pet_skill_text(on: bool, slot: u8, name: &str, ctx: &Context) -> (String, String) {
    if !on {
        return (format!("栏位 {slot}"), "战宠动作不是「技能」".into());
    }
    match ctx.pet_skills.iter().find(|(s, _)| *s == slot) {
        Some((_, n)) => (n.clone(), String::new()),
        None if name.is_empty() => ("未选择".into(), "出战宠没有可用技能".into()),
        None => (format!("{name}（已失效）"), "按最新技能列表重新选".into()),
    }
}

fn cycle_magic(slot: &mut u8, name: &mut String, dir: i64, ctx: &Context) -> Option<String> {
    if ctx.magics.is_empty() {
        return Some("还没有学会精灵，先去学一个".into());
    }
    let n = ctx.magics.len() as i64;
    // 名字为空算“还没选过”，栏位与名字都对得上才算当前这一项；对不上就落到第一个/最后一个。
    let cur = if name.is_empty() {
        None
    } else {
        ctx.magics.iter().position(|(s, nm, _, _)| *s == *slot && nm == name)
    };
    let next = match cur {
        Some(i) => (i as i64 + dir).rem_euclid(n) as usize,
        None if dir > 0 => 0,
        None => (n - 1) as usize,
    };
    let (s, n, _, _) = &ctx.magics[next];
    *slot = *s;
    *name = n.clone();
    None
}

fn cycle_pet_skill(slot: &mut u8, name: &mut String, dir: i64, ctx: &Context) -> Option<String> {
    if ctx.pet_skills.is_empty() {
        return Some("出战宠没有可用技能".into());
    }
    let n = ctx.pet_skills.len() as i64;
    let cur = if name.is_empty() {
        None
    } else {
        ctx.pet_skills.iter().position(|(s, nm)| *s == *slot && nm == name)
    };
    let next = match cur {
        Some(i) => (i as i64 + dir).rem_euclid(n) as usize,
        None if dir > 0 => 0,
        None => (n - 1) as usize,
    };
    let (s, n) = &ctx.pet_skills[next];
    *slot = *s;
    *name = n.clone();
    None
}

#[cfg(test)]
mod tests {
    use super::*;

    fn ctx() -> Context {
        Context {
            magics: vec![
                (0, "火球".into(), 5, TargetType::Enemy),
                (2, "治愈".into(), 12, TargetType::SelfOnly),
            ],
            pet_skills: vec![(1, "抓击".into()), (3, "猛击".into())],
            pet_name: "乌力".into(),
            player_level: 30,
            party_size: 1,
            turn: None,
            encounter_on: false,
            encounter_busy: false,
            encounter_area: String::new(),
        }
    }

    #[test]
    fn columns_cover_every_field_once() {
        let mut seen: Vec<Field> = Vec::new();
        for (_, fields) in columns() {
            for f in fields {
                assert!(!seen.contains(f), "{} 重复出现", f.label());
                seen.push(*f);
            }
        }
        assert_eq!(seen.len(), 54, "五栏一共 54 项（STAGE-285 加了抓宠方式）");
    }

    #[test]
    fn magic_cycles_through_what_the_character_knows() {
        let mut p = AutoProfile::default();
        let c = ctx();
        assert!(Field::PlayerMagic.adjust(&mut p, 1, &c).is_none());
        assert_eq!(p.player.magic_slot, 0);
        assert_eq!(p.player.magic_name, "火球");
        Field::PlayerMagic.adjust(&mut p, 1, &c);
        assert_eq!(p.player.magic_name, "治愈");
        Field::PlayerMagic.adjust(&mut p, 1, &c);
        assert_eq!(p.player.magic_name, "火球", "循环回第一个");
    }

    #[test]
    fn magic_without_knowing_any_says_so() {
        let mut p = AutoProfile::default();
        let msg = Field::PlayerMagic.adjust(&mut p, 1, &Context::default());
        assert!(msg.is_some_and(|m| m.contains("还没有学会精灵")));
    }

    #[test]
    fn percentages_move_in_steps_and_stay_in_range() {
        let mut p = AutoProfile::default();
        let c = ctx();
        p.recover.player_heal_pct = 30;
        Field::PlayerHealPct.adjust(&mut p, 1, &c);
        assert_eq!(p.recover.player_heal_pct, 35);
        p.recover.player_heal_pct = 95;
        Field::PlayerHealPct.adjust(&mut p, 1, &c);
        assert_eq!(p.recover.player_heal_pct, 95, "上限 95%");
        p.recover.player_heal_pct = 5;
        Field::PlayerHealPct.adjust(&mut p, -1, &c);
        assert_eq!(p.recover.player_heal_pct, 5, "下限 5%");
    }

    #[test]
    fn capture_hp_steps_up_to_full_hp_capture_and_back() {
        let mut p = AutoProfile::default();
        let c = ctx();
        p.capture.hp_pct = 90;
        Field::CapturePct.adjust(&mut p, 1, &c);
        assert_eq!(p.capture.hp_pct, 95);
        Field::CapturePct.adjust(&mut p, 1, &c);
        assert_eq!(p.capture.hp_pct, 100, "95 之后多一档满血抓");
        Field::CapturePct.adjust(&mut p, 1, &c);
        assert_eq!(p.capture.hp_pct, 100, "100 是上限");
        assert_eq!(Field::CapturePct.value(&p, &c).0, "满血抓");
        assert!(Field::CapturePct.value(&p, &c).1.contains("不削血，直接抓"));
        Field::CapturePct.adjust(&mut p, -1, &c);
        assert_eq!(p.capture.hp_pct, 95, "往下回到 95");
        p.capture.hp_pct = 5;
        Field::CapturePct.adjust(&mut p, -1, &c);
        assert_eq!(p.capture.hp_pct, 5, "下限 5%");
    }

    #[test]
    fn capture_mode_cycles_and_is_blocked_while_capture_is_off() {
        let mut p = AutoProfile::default();
        let c = ctx();
        // 默认「边战斗边抓」，与加这一项之前一致。
        assert_eq!(p.capture.mode, crate::app::auto_battle::CaptureMode::Fight);
        assert_eq!(Field::CaptureMode.value(&p, &c).0, "边战斗边抓");
        // 自动捕捉关着时不能改，Enter 会说原因。
        assert_eq!(Field::CaptureMode.blocked(&p, &c), Some("自动捕捉关着"));
        // 开着以后 ←→ 在两种方式之间循环。
        p.capture.enabled = true;
        assert!(Field::CaptureMode.blocked(&p, &c).is_none());
        Field::CaptureMode.adjust(&mut p, 1, &c);
        assert_eq!(p.capture.mode, crate::app::auto_battle::CaptureMode::Escape);
        assert_eq!(Field::CaptureMode.value(&p, &c).0, "逃跑抓");
        assert!(Field::CaptureMode.value(&p, &c).1.contains("没有目标就逃"));
        Field::CaptureMode.adjust(&mut p, 1, &c);
        assert_eq!(p.capture.mode, crate::app::auto_battle::CaptureMode::Fight, "两个取值循环");
        Field::CaptureMode.adjust(&mut p, -1, &c);
        assert_eq!(p.capture.mode, crate::app::auto_battle::CaptureMode::Escape, "反方向也循环");
    }

    /// 旧方案文件没有 `mode` 字段时按「边战斗边抓」读，行为与加这一项之前一致。
    #[test]
    fn old_profiles_without_capture_mode_read_as_fight() {
        let c: crate::app::auto_battle::Capture =
            serde_json::from_str(r#"{"enabled":true,"hp_pct":20,"level_min":1,"level_max":0}"#).expect("旧方案能读");
        assert_eq!(c.mode, crate::app::auto_battle::CaptureMode::Fight);
    }

    #[test]
    fn dependent_fields_are_blocked_with_a_reason() {
        let p = AutoProfile::default();
        let c = ctx();
        assert!(Field::PlayerMagic.blocked(&p, &c).is_some(), "默认动作是攻击，精灵项不生效");
        assert!(Field::PlayerHealPct.blocked(&p, &c).is_none());
        let mut off = AutoProfile::default();
        off.recover.player_heal = false;
        assert!(Field::PlayerHealPct.blocked(&off, &c).is_some());
    }

    #[test]
    fn first_turn_fields_follow_the_switch() {
        let mut p = AutoProfile::default();
        let c = ctx();
        assert!(Field::FirstPlayerAct.blocked(&p, &c).is_some(), "默认没开首回合");
        Field::FirstOn.adjust(&mut p, 1, &c);
        assert!(p.first.enabled);
        assert!(Field::FirstPlayerAct.blocked(&p, &c).is_none());
    }

    #[test]
    fn name_lists_and_read_only_rows_are_marked() {
        assert!(Field::FleeNames.is_name_list() && Field::MpItems.is_name_list());
        assert!(Field::CaptureNames.is_name_list());
        assert!(Field::EncounterOn.read_only() && Field::EncounterArea.read_only());
        assert!(!Field::FleeOn.read_only() && !Field::FleeNames.read_only());
    }

    /// STAGE-173 M09：遇敌开关与选区是只读展示，值直接来自真实状态。
    #[test]
    fn encounter_rows_show_the_real_switch_and_area() {
        let mut p = AutoProfile::default();
        let mut c = ctx();
        c.encounter_on = true;
        assert_eq!(Field::EncounterOn.value(&p, &c).0, "开启");
        c.encounter_busy = true;
        assert_eq!(Field::EncounterOn.value(&p, &c).0, "正在关闭");
        c.encounter_busy = false;
        c.encounter_on = false;
        assert_eq!(Field::EncounterOn.value(&p, &c).0, "关闭");
        c.encounter_area = "区域 #3".into();
        assert_eq!(Field::EncounterArea.value(&p, &c).0, "区域 #3");
        c.encounter_area.clear();
        assert_eq!(Field::EncounterArea.value(&p, &c).0, "未选择");
        // 只读项改不动，Enter/←→ 只给一句说明。
        assert!(Field::EncounterOn.adjust(&mut p, 1, &c).is_some());
        assert_eq!(Field::EncounterOn.value(&p, &c).0, "关闭", "值没被改");
    }

    /// 名单类设置的显示：空是「不限」，超过三项省略。
    #[test]
    fn name_lists_read_as_unlimited_or_shortened() {
        let mut p = AutoProfile::default();
        let c = ctx();
        assert_eq!(Field::FleeNames.value(&p, &c).0, "不限");
        p.flee.names = vec!["野猪".into(), "乌力".into()];
        assert_eq!(Field::FleeNames.value(&p, &c).0, "野猪、乌力");
        p.flee.names.push("加加".into());
        p.flee.names.push("威威".into());
        assert!(Field::FleeNames.value(&p, &c).0.ends_with("（共 4 项）"));
    }

    /// STAGE-173：补血精灵可以在「自动 → 身上每个精灵」之间手动指定。
    #[test]
    fn heal_magic_cycles_from_auto_through_the_known_magics() {
        let mut p = AutoProfile::default();
        let c = ctx(); // 火球(0)、治愈(2)
        assert_eq!(Field::PlayerHealMagic.value(&p, &c).0, "自动", "默认自动认");
        Field::PlayerHealMagic.adjust(&mut p, 1, &c);
        assert_eq!((p.recover.heal_magic_slot, p.recover.heal_magic_name.as_str()), (0, "火球"));
        Field::PlayerHealMagic.adjust(&mut p, 1, &c);
        assert_eq!((p.recover.heal_magic_slot, p.recover.heal_magic_name.as_str()), (2, "治愈"));
        Field::PlayerHealMagic.adjust(&mut p, 1, &c);
        assert!(p.recover.heal_magic_name.is_empty(), "再往右循环回「自动」");
        Field::PlayerHealMagic.adjust(&mut p, -1, &c);
        assert_eq!(p.recover.heal_magic_name, "治愈", "从自动往左直接到最后一个");

        // 选成攻击类精灵：界面直接说不能用来补血。
        let mut atk = AutoProfile::default();
        atk.recover.heal_magic_slot = 0;
        atk.recover.heal_magic_name = "火球".into();
        assert!(Field::PlayerHealMagic.value(&atk, &c).1.contains("不能补血"));

        // 指定的精灵不在列表里了：提示已失效（托管会回落到自动认）。
        p.recover.heal_magic_name = "不存在的精灵".into();
        assert!(Field::PlayerHealMagic.value(&p, &c).0.contains("已失效"));

        // 补血关着时这一项不可改。
        let mut off = AutoProfile::default();
        off.recover.player_heal = false;
        assert!(Field::PlayerHealMagic.blocked(&off, &c).is_some());
        assert!(Field::PlayerHealMagic.blocked(&AutoProfile::default(), &Context::default()).is_some(), "身上没精灵");
    }

    /// STAGE-173：出招延迟的第一档是「即时」（0ms），界面写中文而不是 `0 ms`。
    #[test]
    fn immediate_delay_reads_as_instant() {
        let mut p = AutoProfile::default();
        let c = ctx();
        p.playback.delay_ms = 0;
        assert_eq!(Field::Delay.value(&p, &c).0, "即时");
        assert!(Field::Delay.value(&p, &c).1.contains("立刻") || Field::Delay.value(&p, &c).1.contains("不等"));
        p.playback.delay_ms = 500;
        assert_eq!(Field::Delay.value(&p, &c).0, "500 ms");
        // 左到头停在即时，右一下回到 300ms。
        p.playback.delay_ms = 0;
        Field::Delay.adjust(&mut p, 1, &c);
        assert_eq!(p.playback.delay_ms, 300);
        Field::Delay.adjust(&mut p, -1, &c);
        assert_eq!(p.playback.delay_ms, 0);
        // 方案里手改成 50（旧文件）也贴到最近档位。
        p.playback.delay_ms = 50;
        Field::Delay.adjust(&mut p, 1, &c);
        assert_eq!(p.playback.delay_ms, 300, "50 最近的是 0，往右是 300");
    }

    /// STAGE-262 B1：复活三行默认「开 / 自动（先精灵后道具）/ 开」，自动复活关着时另两行不能改。
    #[test]
    fn revive_rows_default_and_follow_the_switch() {
        let mut p = AutoProfile::default();
        let c = ctx();
        assert_eq!(Field::ReviveOn.value(&p, &c).0, "开");
        assert_eq!(Field::ReviveSource.value(&p, &c).0, "自动（先精灵后道具）");
        assert_eq!(Field::ReviveDefend.value(&p, &c).0, "开");
        Field::ReviveSource.adjust(&mut p, 1, &c);
        assert_eq!(p.recover.revive_source, crate::app::auto_battle::ReviveSource::MagicOnly);
        Field::ReviveSource.adjust(&mut p, 1, &c);
        assert_eq!(Field::ReviveSource.value(&p, &c).0, "只用道具");
        Field::ReviveOn.adjust(&mut p, 1, &c);
        assert!(!p.recover.revive_on);
        assert_eq!(Field::ReviveSource.blocked(&p, &c), Some("自动复活关着"));
        assert_eq!(Field::ReviveDefend.blocked(&p, &c), Some("自动复活关着"));
        assert_eq!(Field::ReviveOn.blocked(&p, &c), None);
    }
}
