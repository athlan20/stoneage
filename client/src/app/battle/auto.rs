//! 托管（自动战斗，STAGE-42 起，STAGE-173 改成读方案）：纯客户端实现，
//! 每回合收到 `battle_turn` 后自己算出招，发和手动出招完全一样的 `battle_command`，
//! 协议和服务端都不改。
//!
//! 出招按 `docs/auto-battle-settings.md` 的固定优先级：中断判断与停止保护（`App::auto_tick`）→
//! 自动逃跑 → 自动复活（出战宠倒下时，`auto_revive.rs`）→ 人物补血 → 战宠补血 → 补气 → 自动捕捉 → 首回合动作 → 常规动作及兜底。
//! 每一步只产生一条指令；配置从 `App::auto_profile()` 来，改动后从下一个未提交阶段生效。

use std::time::{Duration, Instant};

use super::auto_heal::{self, heal_move, mp_move};
use super::auto_revive;
use super::command::{Act, Actor};
use super::slots;
use super::{BattleState, Phase, Pick};
use crate::app::auto_battle::{ActKind, AutoProfile, CaptureMode, PetActKind, TargetMode};
use crate::app::{App, WorldState};
use crate::proto::TargetType;

// 出招延迟与让位时间不再是常量（STAGE-173 M16）：读方案里的 `playback.delay_ms` /
// `playback.manual_hold_ms`，由 `App::auto_delay()` / `App::auto_hold()` 给出。
/// 托管指令被拒后，隔多久换一个来源重试。
const RETRY_DELAY: Duration = Duration::from_millis(300);
/// 每个下指令阶段最多重试几次，超过就等服务端的超时默认动作。
const MAX_RETRIES: u32 = 3;
/// 随身宠物栏位数（服务端 `character.MaxPetSlots`，STAGE-346 起 20 个栏位）：
/// 抓宠挂机判定“宠位满”用它。
pub(super) const PET_SLOTS: usize = 20;
/// 托管中结果窗口显示多久后自动关闭，让“一场接一场”不用按键。
pub(super) const RESULT_HOLD: Duration = Duration::from_millis(2500);

/// 一次托管出招：动作、目标、参数，以及写进战斗记录的原因。
pub(super) struct AutoMove {
    pub act: Act,
    pub target: Option<u8>,
    pub arg: Option<i64>,
    pub reason: String,
}

/// 算出这个角色这一回合的托管出招。顺序照需求的「基础执行规则」（停止保护在 `auto_tick` 里先判）：
/// 自动逃跑 → 人物补血 / 战宠补血 → 人物补气 → 自动捕捉 → 首回合动作（只在第 1 回合）→ 常规动作。
/// 每一步都可能因为“配置关着、来源没有、目标不合法”而跳过，最后落到普攻或防御。
pub(super) fn plan(b: &BattleState, w: &WorldState, actor: Actor, cfg: &AutoProfile) -> AutoMove {
    let mv = plan_inner(b, w, actor, cfg);
    // 想逃但这场不许逃：照常打完，但把原因写进战斗记录（需求：不能逃跑要有说明）。
    if actor == Actor::Player
        && mv.act != Act::Escape
        && matches!(mv.act, Act::Attack | Act::Guard)
        && !b.allow.escape
        && let Some(why) = flee_wanted(b, cfg)
    {
        return AutoMove { reason: format!("本场不能逃跑（{}），{}", why.text(cfg), mv.reason), ..mv };
    }
    mv
}

fn plan_inner(b: &BattleState, w: &WorldState, actor: Actor, cfg: &AutoProfile) -> AutoMove {
    let first = b.turn <= 1 && cfg.first.enabled;
    let mode = cfg.target;
    match actor {
        Actor::Player => {
            // 抓宠方式（STAGE-285）：逃跑抓时，本场有没有合格目标决定「打」还是「逃」。
            let catch_mode = cfg.capture.enabled && cfg.capture.mode == CaptureMode::Escape;
            let catch_target = if catch_mode { capture_target(b, w, cfg) } else { None };
            // ② 自动逃跑（M10）：危险血量或指定敌人上场，先跑，别的都不做。
            //    逃跑抓且本场有目标时，「逃跑敌人名单」让位给抓宠——正在抓的场次不该因为名单里
            //    别的怪就逃；血量门槛仍然优先（保命比抓宠重要）。
            if let Some(why) = flee_wanted(b, cfg)
                && !(catch_target.is_some() && why == FleeWhy::Named)
                && let Some(mv) = flee_move(b, cfg)
            {
                return mv;
            }
            // ②' 逃跑抓：本场没有合格目标，直接结束这一场，专心刷新目标。
            //     不要求「自动逃跑」开着（这是抓宠策略，不是保命策略）；本场不许逃时照常打完。
            if catch_mode && catch_target.is_none() && b.allow.escape {
                return AutoMove {
                    act: Act::Escape,
                    target: None,
                    arg: None,
                    reason: "逃跑抓：本场没有捕捉目标".into(),
                };
            }
            // ③ 自动复活（STAGE-262 B2）：出战宠倒下且来源可用，先救起来（只管宠物，人物倒下不归这里）。
            if let Some(mv) = auto_revive::revive_move(b, w, cfg) {
                return mv;
            }
            if let Some(mv) = heal_move(b, w, cfg) {
                return mv;
            }
            if let Some(mv) = mp_move(b, w, cfg) {
                return mv;
            }
            // ⑦ 自动捕捉（M13）：削血 / 捕捉，排在首回合与常规动作前面，避免兜底把目标打死。
            if let Some(mv) = capture_move(b, w, cfg) {
                return mv;
            }
            let action = if first { &cfg.first.player } else { &cfg.player };
            match action.act {
                ActKind::Guard => AutoMove {
                    act: Act::Guard,
                    target: None,
                    arg: None,
                    reason: if first { "首回合防御".into() } else { "防御".into() },
                },
                ActKind::Magic => magic_move(b, w, action.magic_slot, &action.magic_name, first, mode)
                    .unwrap_or_else(|| attack_or_guard(b, mode, "精灵已失效，改攻击")),
                ActKind::Attack => attack_or_guard(b, mode, ""),
            }
        }
        Actor::Pet => {
            // 捕捉期间战宠一律防御（M13）：它的攻击会把待捕目标打死。
            if capture_target(b, w, cfg).is_some() {
                return AutoMove { act: Act::Guard, target: None, arg: None, reason: "战宠防御（捕捉中）".into() };
            }
            let action = if first { &cfg.first.pet } else { &cfg.pet };
            match action.act {
                PetActKind::Guard => AutoMove {
                    act: Act::Guard,
                    target: None,
                    arg: None,
                    reason: "战宠防御".into(),
                },
                PetActKind::Skill => pet_skill_move(b, w, action.skill_slot, &action.skill_name, mode)
                    .unwrap_or_else(|| attack_or_guard(b, mode, "战宠技能已失效，改攻击")),
                PetActKind::Attack => attack_or_guard(b, mode, ""),
            }
        }
    }
}

/// 人物用精灵：栏位按最新精灵列表核对（名字对不上就是换过装/重学过），敌方类的带上编号最小的活敌，
/// 己方一个的选自己；复活类（`ally_down`）不进自动候选——自动复活不在本需求范围内。
fn magic_move(
    b: &BattleState,
    w: &WorldState,
    slot: u8,
    name: &str,
    first: bool,
    mode: TargetMode,
) -> Option<AutoMove> {
    let m = w.entry.status.magic.iter().find(|m| m.slot == slot)?;
    if !name.is_empty() && !m.name.eq(name) {
        return None; // 栏位换成了别的精灵：不拿旧名字硬套
    }
    if m.mp > w.entry.status.mp {
        return None; // MP 不够，回退普攻
    }
    let target = match m.target_type {
        TargetType::Enemy => Some(pick_enemy(b, mode)?),
        TargetType::Ally => Some(b.you),
        TargetType::AllyDown => return None,
        _ => None,
    };
    Some(AutoMove {
        act: Act::Magic,
        target,
        arg: Some(i64::from(slot)),
        reason: format!("{}用 {}（{} MP）", if first { "首回合" } else { "" }, m.name, m.mp),
    })
}

/// 战宠用技能：栏位按宠物页最新技能核对，目标规则同人物精灵。
fn pet_skill_move(b: &BattleState, w: &WorldState, slot: u8, name: &str, mode: TargetMode) -> Option<AutoMove> {
    let pet = w.pets.pets.iter().find(|p| p.state == "battle")?;
    let s = pet.skills.iter().find(|s| s.slot == slot)?;
    if !name.is_empty() && !s.name.eq(name) {
        return None;
    }
    if s.target_type == TargetType::AllyDown {
        return None;
    }
    let target = match s.target_type {
        TargetType::Enemy => Some(pick_enemy(b, mode)?),
        TargetType::Ally => Some(b.you + 5),
        _ => None,
    };
    Some(AutoMove {
        act: Act::Skill,
        target,
        arg: Some(i64::from(slot)),
        reason: format!("战宠用 {}", s.name),
    })
}

/// 是不是敌方单位（编号的高位是阵营）。
fn is_enemy(b: &BattleState, no: u8) -> bool {
    no / 10 == slots::other_side(b.me_side())
}

/// 该不该逃（M10）的原因。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum FleeWhy {
    /// 自己血量到门槛（保命，优先于抓宠）。
    LowHp,
    /// 场上敌人命中「逃跑敌人名单」。
    Named,
}

impl FleeWhy {
    fn text(self, cfg: &AutoProfile) -> String {
        match self {
            Self::LowHp => format!("自己血量 ≤ {}%", cfg.flee.hp_pct),
            Self::Named => "场上敌人命中名单".to_string(),
        }
    }
}

/// 该不该逃（M10）：自己血量 ≤ 门槛，或场上活着的敌人名字命中名单。返回命中的原因。
/// 只看条件，不看这场允不允许逃——不允许逃时由 `plan` 补一句说明。
fn flee_wanted(b: &BattleState, cfg: &AutoProfile) -> Option<FleeWhy> {
    if !cfg.flee.enabled {
        return None;
    }
    if b.units.get(&b.you).is_some_and(|u| u.hp_pct > 0 && u.hp_pct <= u32::from(cfg.flee.hp_pct)) {
        return Some(FleeWhy::LowHp);
    }
    let named = b.here.iter().any(|no| {
        is_enemy(b, *no) && b.units.get(no).is_some_and(|u| u.hp_pct > 0 && cfg.flee.name_hit(&u.name))
    });
    named.then_some(FleeWhy::Named)
}

/// 自动逃跑（M10）：条件命中且这场允许逃跑就发出逃跑；不允许时返回 `None`，交给后面的动作——
/// 硬发只会被服务端拒。
fn flee_move(b: &BattleState, cfg: &AutoProfile) -> Option<AutoMove> {
    let why = flee_wanted(b, cfg)?;
    if !b.allow.escape {
        return None;
    }
    Some(AutoMove { act: Act::Escape, target: None, arg: None, reason: format!("逃跑（{}）", why.text(cfg)) })
}

/// 这次逃跑算不算「逃跑抓」的逃跑（STAGE-285）：抓宠循环里逃一场是为了刷新目标，
/// 跑完还要继续刷，所以**不关后续遇敌、也不停挂机**——保命（血量到门槛）那次也算：
/// 战后吃肉会把血补回来，循环才能继续；停掉挂机等于让玩家自己重开。
pub(super) fn is_catch_flee(cfg: &AutoProfile) -> bool {
    cfg.capture.enabled && cfg.capture.mode == CaptureMode::Escape
}

/// 捕捉的候选目标（M13）：名字与等级都要过筛，且等级不能超过人物 +5（服务端的抓宠规则）。
/// 多个合格目标按现有站位顺序（`enemy_order`）取第一个。
fn capture_target(b: &BattleState, w: &WorldState, cfg: &AutoProfile) -> Option<u8> {
    if !cfg.capture.enabled || !b.allow.capture {
        return None;
    }
    let mine = w.entry.status.level;
    enemy_order(b).into_iter().find(|no| {
        let Some(u) = b.units.get(no) else {
            return false;
        };
        if u.kind != "monster" {
            return false;
        }
        if !cfg.capture.name_ok(&u.name) {
            return false;
        }
        if cfg.capture.level_min != 0 && u.level < cfg.capture.level_min {
            return false;
        }
        if cfg.capture.level_max != 0 && u.level > cfg.capture.level_max {
            return false;
        }
        // 服务端只让抓“不超过自己 5 级”的怪，筛掉免得白跑一条指令。
        u.level <= mine.saturating_add(5)
    })
}

/// 自动捕捉（M13）：目标血量高于门槛就先削血，到了门槛改用捕捉。只用单体动作，
/// 群攻和随机攻击不参与（需求：不能保证必捕获，但不能误伤别的目标）。
fn capture_move(b: &BattleState, w: &WorldState, cfg: &AutoProfile) -> Option<AutoMove> {
    let no = capture_target(b, w, cfg)?;
    let u = b.units.get(&no)?;
    if u.hp_pct > u32::from(cfg.capture.hp_pct) {
        return Some(AutoMove {
            act: Act::Attack,
            target: Some(no),
            arg: None,
            reason: format!("捕捉：先把 {} 削到 {}% 以下", u.name, cfg.capture.hp_pct),
        });
    }
    Some(AutoMove {
        act: Act::Capture,
        target: Some(no),
        arg: None,
        reason: format!("捕捉 {}（HP {}%）", u.name, u.hp_pct),
    })
}

/// 普攻编号最小的活敌；没有活敌就防御。`why` 是有回退原因时写进战斗记录的前缀。
fn attack_or_guard(b: &BattleState, mode: TargetMode, why: &str) -> AutoMove {
    match pick_enemy(b, mode) {
        Some(no) => AutoMove {
            act: Act::Attack,
            target: Some(no),
            arg: None,
            reason: if why.is_empty() {
                format!("攻击 {}", b.name(no))
            } else {
                format!("{why}：攻击 {}", b.name(no))
            },
        },
        None => AutoMove { act: Act::Guard, target: None, arg: None, reason: "防御（没有活着的敌人）".into() },
    }
}

/// 敌方目标的顺序（STAGE-173「合击顺序」）：先前排、再后排，每排从上到下（`slots::ROW_ORDER`），
/// 只留活着的。人物、战宠都取这一串的第一个，于是打同一个目标——原版合击判定要求同目标。
/// STAGE-312 起服务端在没指定分身目标时也按这个顺序挑，分身面板拿它显示默认目标。
pub(crate) fn enemy_order(b: &BattleState) -> Vec<u8> {
    let side = slots::other_side(b.me_side());
    let mut out = Vec::new();
    for front in [true, false] {
        for col in 0..5 {
            let no = slots::no_at(slots::Cell { side, front, col });
            if b.here.contains(&no) && b.units.get(&no).is_some_and(|u| u.hp_pct > 0) {
                out.push(no);
            }
        }
    }
    out
}

/// 按方案的「默认攻击目标」挑一个敌人：合击顺序取第一个，随机取列表里的一个。
fn pick_enemy(b: &BattleState, mode: TargetMode) -> Option<u8> {
    let list = enemy_order(b);
    match mode {
        TargetMode::Combo => list.first().copied(),
        TargetMode::Random => {
            if list.is_empty() {
                return None;
            }
            // 线性同余，够随机且不用引入依赖；每场战斗的种子不同。
            let seed = b.rng.get().wrapping_mul(1_664_525).wrapping_add(1_013_904_223);
            b.rng.set(seed);
            list.get((seed as usize) % list.len()).copied()
        }
    }
}

impl App {
    /// `auto-battle on|off|status`（别名 `auto`，不带参数查询，风格同 `encounter`）。
    /// 开启是“操作”类：观察模式（agent 操控）下被拦，由命令表的标记驱动；关闭走主动停机
    /// （`halt.rs`，STAGE-153），和战斗菜单、系统页的「停止自动挂机」是同一个动作。
    pub(in crate::app) fn auto_battle_command(&mut self, rest: &str) {
        match rest.trim() {
            "" | "status" | "状态" => {
                let (auto, encounter) = self
                    .world
                    .as_ref()
                    .map(|w| (w.auto_text(), w.encounter_text()))
                    .unwrap_or(("关闭", "关闭"));
                self.log.system(format!("自动挂机：{auto}（每回合自动出招）· 自动遇敌：{encounter}"));
            }
            "on" | "开" => self.start_auto("命令"),
            "off" | "关" => self.stop_auto_all("命令"),
            _ => {
                self.log.error("用法：auto-battle on|off|status（简写 auto）");
            }
        }
    }

    /// 开启自动挂机（统一入口）：只开本机的自动出招，不碰遇敌，也不改选区的遇敌计划。
    /// 开始时丢掉尚未提交的选物/目标，回到安全动作层；焦点仍停在管理项上。
    pub(crate) fn start_auto(&mut self, origin: &str) {
        if self.halt_busy() {
            self.log.system("正在关闭自动遇敌，请稍候再开始自动挂机");
            return;
        }
        let Some(w) = self.world.as_ref() else {
            return;
        };
        if !w.online {
            self.log.error("连接已断开，不能开始自动挂机；请从系统菜单重新连接");
            return;
        }
        if w.observing() {
            self.log.error("agent 操控中，请先收回控制权");
            return;
        }
        let now = Instant::now();
        let delay = self.auto_delay();
        let input_now = w.battle.as_ref().is_some_and(|b| matches!(b.phase, Phase::Input(_)) && !b.submitting);
        if let Some(w) = self.world.as_mut() {
            w.auto_battle = true;
            if let Some(b) = w.battle.as_mut() {
                b.prepared_row = None;
                b.pick = Pick::Action;
                b.list_focus = 0;
                if input_now {
                    b.schedule_auto(now, delay);
                }
            }
        }
        self.log.system(format!(
            "自动挂机已开启（{origin}）：每回合自动出招，不打开自动遇敌；手动出招会让位，再按一次停止"
        ));
    }

    /// 关掉自动挂机并说明原因（战斗失败、位置变化、agent 接管）；本来就关着就不打扰。
    /// 被动停机不动遇敌：那是服务端自己的自动关闭，和玩家主动停止不是一件事（STAGE-153）。
    pub(crate) fn stop_auto(&mut self, reason: &str) {
        let was_on = self.world.as_mut().is_some_and(|w| w.auto_battle);
        if !was_on {
            return;
        }
        if let Some(w) = self.world.as_mut() {
            w.auto_battle = false;
            if let Some(b) = w.battle.as_mut() {
                b.auto_due = None;
            }
        }
        self.log.system(format!("自动挂机已关闭：{reason}"));
    }

    /// 停止保护的命中条件（M11）：返回一句话列出全部命中的原因，没命中返回 `None`。
    /// 条件是固定的几项，没有可编排的规则；配置里 0 表示该项不启用。
    fn stop_guard_reasons(&self) -> Option<String> {
        let w = self.world.as_ref()?;
        if !w.auto_battle || self.halt_busy() {
            return None;
        }
        let cfg = self.auto_profile();
        let mut hit: Vec<String> = Vec::new();
        // 补给不足：要补血/补气，而且**现在确实低于门槛**，却没有任何可用来源。
        // 只看战斗内——不在战斗里不需要补，背包空也不该把人停掉。
        if cfg.stop.no_supply && let Some(b) = w.battle.as_ref() {
            let st = &w.entry.status;
            let need_hp = cfg.recover.player_heal
                && b.units
                    .get(&b.you)
                    .is_some_and(|u| u.hp_pct > 0 && u.hp_pct <= u32::from(cfg.recover.player_heal_pct));
            let need_mp = cfg.recover.player_mp
                && st.max_mp > 0
                && u64::from(st.mp) * 100 <= u64::from(st.max_mp) * u64::from(cfg.recover.player_mp_pct);
            if (need_hp || need_mp) && !auto_heal::has_supply(w, &cfg) {
                hit.push("补给用完".into());
            }
        }
        if cfg.stop.player_level != 0 && w.entry.status.level >= cfg.stop.player_level {
            hit.push(format!("人物已到 {} 级", cfg.stop.player_level));
        }
        if let Some(pet) = w.pets.pets.iter().find(|p| p.state == "battle")
            && cfg.stop.pet_level != 0
            && pet.level >= cfg.stop.pet_level
        {
            hit.push(format!("战宠已到 {} 级", cfg.stop.pet_level));
        }
        if cfg.stop.party_min != 0 {
            let n = w.entry.party.as_ref().map_or(1, |p| p.members.len());
            if n < usize::from(cfg.stop.party_min) {
                hit.push(format!("队伍 {n} 人，少于 {} 人", cfg.stop.party_min));
            }
        }
        // 抓宠时宠位满：再抓也没地方放，停在这里让人去宠店腾位置。
        if cfg.capture.enabled && w.pets.pets.len() >= PET_SLOTS {
            hit.push(format!("宠物栏位已满（{PET_SLOTS} 只）"));
        }
        (!hit.is_empty()).then(|| hit.join("、"))
    }

    /// 托管的定时部分，每个刷新周期查一次：到了时刻自动出招；结果窗口看一会儿自动关。
    pub(crate) fn auto_tick(&mut self, now: Instant) {
        self.post_battle_tick();
        let on = self.world.as_ref().is_some_and(|w| w.auto_battle);
        if !on {
            // STAGE-284：观察模式不依赖本机托管，结果窗口看够 RESULT_HOLD 也自动关。
            let observed_result_due = self.world.as_ref().is_some_and(|w| w.observing())
                && self.battle_ref().is_some_and(|b| {
                    b.result.is_some() && b.result_at.is_some_and(|at| now >= at + RESULT_HOLD)
                });
            if observed_result_due {
                self.close_battle();
            }
            return;
        }
        // 停止保护（M11）：命中就立即停本机自动出招、请求关闭后续遇敌，并把全部原因写出来。
        if let Some(why) = self.stop_guard_reasons() {
            self.stop_by_guard(&why);
            return;
        }
        // 结果窗口：托管中不用按键关闭，看一会儿自动关，配合“一场接一场”。方案里可以关掉自动继续。
        let result_due = self.auto_continue
            && self.battle_ref().is_some_and(|b| {
                b.result.is_some() && b.result_at.is_some_and(|at| now >= at + RESULT_HOLD)
            });
        if result_due {
            self.close_battle();
            return;
        }
        if !self.battle_ref().is_some_and(|b| b.auto_due.is_some_and(|d| d <= now)) {
            return;
        }
        // 玩家正在打命令（输入行有字）：等打完这一下，托管再接手。
        if self.world.as_ref().is_some_and(|w| w.nav.is(&crate::app::Layer::Command) && !w.input.is_empty()) {
            let hold = self.auto_hold();
            self.battle_mut().map(|b| b.defer_auto(now, hold));
            return;
        }
        let ready = self.battle_ref().is_some_and(|b| matches!(b.phase, Phase::Input(_)) && !b.submitting);
        if !ready {
            // 玩家手动接管了（指令已发出）或阶段已过去：这次安排作废。
            self.battle_mut().map(|b| b.auto_due = None);
            return;
        }
        self.auto_act();
    }

    /// 按当前阶段算一招并发出去，写一行战斗记录。
    fn auto_act(&mut self) {
        if self.world.as_ref().is_some_and(|w| w.observing()) {
            self.stop_auto("agent 操控中");
            return;
        }
        let cfg = self.auto_profile();
        let Some((mv, actor)) = (self.battle_ref().zip(self.world.as_ref())).and_then(|(b, w)| {
            let Phase::Input(actor) = b.phase else { return None };
            Some((plan(b, w, actor, cfg), actor))
        }) else {
            return;
        };
        // 自动捕捉带资质筛选（STAGE-264）：全不限时 `wire` 为空，指令里就不带 keep。
        let keep = (mv.act == Act::Capture).then(|| cfg.capture.keep.clone());
        // 逃跑要先请求关闭后续遇敌（M10）：跑掉以后不该再开新的一场。
        // 逃跑抓的逃跑例外（STAGE-285）：跑完还要继续刷目标，既不关遇敌、也不停挂机。
        // 这一步要在下面借用 `battle_mut` 之前算，`cfg` 还借着 `self`。
        let catch_flee = mv.act == Act::Escape && is_catch_flee(cfg);
        // 复活没有来源（或试满了）：写一句「复活失败」，每只宠每场一次；本招照常发。
        if actor == Actor::Player
            && mv.act != Act::Escape
            && let Some((pet, text)) =
                self.battle_ref().zip(self.world.as_ref()).and_then(|(b, w)| auto_revive::give_up_note(b, w, cfg))
            && let Some(b) = self.battle_mut()
        {
            auto_revive::mark_noted(b, pet);
            b.note(text);
        }
        // 已经关着或正在关就不再重复请求，免得每回合刷一条消息。
        if mv.act == Act::Escape
            && !catch_flee
            && !self.halt_busy()
            && self.world.as_ref().is_some_and(|w| w.entry.encounter)
        {
            self.stop_encounter_only();
        }
        if !self.send_command_keep(actor, mv.act, mv.target, mv.arg, keep.as_ref()) {
            return; // 没发出去（不在战斗里），等下个周期
        }
        if let Some(b) = self.battle_mut() {
            auto_revive::note_issued(b, &mv);
            b.last_auto = Some((mv.act, mv.arg));
            b.note(format!("自动挂机：{}", mv.reason));
        }
        self.flush_battle_log();
    }

    /// 托管发出的指令被服务端拒了：道具槽位本场拉黑，稍后换个来源重试（不超过 MAX_RETRIES）。
    pub(crate) fn on_auto_command_failed(&mut self, now: Instant) {
        let on = self.world.as_ref().is_some_and(|w| w.auto_battle);
        let Some(b) = self.battle_mut() else {
            return;
        };
        let Some((act, arg)) = b.last_auto.take() else {
            return; // 被拒的是玩家手动发的指令
        };
        auto_revive::note_rejected(b, act, arg);
        if !on || !matches!(b.phase, Phase::Input(_)) || b.auto_retries >= MAX_RETRIES {
            return;
        }
        if act == Act::Item && let Some(slot) = arg.and_then(|n| u8::try_from(n).ok()).filter(|s| (5..=19).contains(s)) {
            b.bad_slots.insert(slot);
        }
        if act == Act::Escape {
            // 逃跑被拒（本场不许逃、或状态不允许）：不重试，下一回合重新判断，免得空转。
            return;
        }
        b.auto_retries += 1;
        b.auto_due = Some(now + RETRY_DELAY);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    /// 只在敌方放几只指定编号（血量）的野猪，用来核对目标顺序。
    fn with_enemies(list: &[(u8, u32)]) -> BattleState {
        let mut units = vec![BattleUnit { no: 0, name: "勇者".into(), hp_pct: 100, ..BattleUnit::default() }];
        units.extend(list.iter().map(|(no, pct)| BattleUnit {
            no: *no,
            name: "野猪".into(),
            hp_pct: *pct,
            ..BattleUnit::default()
        }));
        let mut b = BattleState::from_start(BattleStart {
            scene: "野外".into(),
            you: 0,
            units,
            ..BattleStart::default()
        });
        b.phase = Phase::Input(Actor::Player);
        b
    }

    /// STAGE-173「合击顺序」：敌方前排从上到下、再后排从上到下；默认方案就是它。
    #[test]
    fn combo_order_is_front_row_then_back_row() {
        // 前排 col0 = 18、col1 = 16；后排 col0 = 13、col2 = 10。
        let b = with_enemies(&[(18, 100), (16, 100), (13, 100), (10, 100)]);
        assert_eq!(enemy_order(&b), vec![18, 16, 13, 10]);
        assert_eq!(AutoProfile::default().target, TargetMode::Combo, "默认合击顺序");
        assert_eq!(pick_enemy(&b, TargetMode::Combo), Some(18), "先打前排最上面那个");

        // 前排最上面倒下 → 顺延到前排下一个；前排清空才轮到后排。
        let b = with_enemies(&[(18, 0), (16, 100), (13, 100)]);
        assert_eq!(pick_enemy(&b, TargetMode::Combo), Some(16));
        let b = with_enemies(&[(18, 0), (16, 0), (13, 100), (10, 100)]);
        assert_eq!(pick_enemy(&b, TargetMode::Combo), Some(13));
    }

    /// 人物和战宠都取同一个目标，才可能合成合击。
    #[test]
    fn player_and_pet_attack_the_same_target_in_combo_mode() {
        let b = with_enemies(&[(18, 100), (16, 100)]);
        let w = world(&[]);
        let c = AutoProfile::default();
        let player = plan(&b, &w, Actor::Player, &c);
        let pet = plan(&b, &w, Actor::Pet, &c);
        assert_eq!((player.act, player.target), (Act::Attack, Some(18)));
        assert_eq!((pet.act, pet.target), (Act::Attack, Some(18)));
    }

    /// 「随机目标」只在活着的敌人里挑，而且不是每次都同一个。
    #[test]
    fn random_mode_picks_among_living_enemies() {
        let b = with_enemies(&[(18, 100), (16, 100), (13, 100)]);
        let mut seen = std::collections::BTreeSet::new();
        for _ in 0..40 {
            if let Some(no) = pick_enemy(&b, TargetMode::Random) {
                seen.insert(no);
            }
        }
        assert!(seen.len() > 1, "随机应该会换目标：{seen:?}");
        assert!(seen.iter().all(|no| [18, 16, 13].contains(no)), "只在活着的敌人里挑：{seen:?}");

        // 只剩一个活敌时只能打它。
        let b = with_enemies(&[(18, 0), (16, 100), (13, 0)]);
        for _ in 0..10 {
            assert_eq!(pick_enemy(&b, TargetMode::Random), Some(16));
        }
    }

    /// STAGE-173：「跳过」档下回合结算不播动画，直接推进到下一回合。
    #[test]
    fn skip_mode_ends_the_replay_at_once() {
        use crate::app::{Options, Screen};
        let mut a = App::new(Options::default());
        a.screen = Screen::World;
        let mut w = WorldState::new(crate::proto::WorldEntry::default());
        let mut b = BattleState::from_start(crate::proto::BattleStart::default());
        b.on_round(crate::proto::BattleRound { turn: 1, ..Default::default() }, Instant::now());
        assert_eq!(b.phase, Phase::Replay);
        w.battle = Some(b);
        a.world = Some(w);
        a.skip_replay = true;

        a.battle_tick(Instant::now());
        let b = a.world.as_ref().unwrap().battle.as_ref().unwrap();
        assert_eq!(b.phase, Phase::Waiting, "跳过档不播回放：直接进等待/下一回合");
    }

    /// STAGE-173：托管中不再让人手动选招，所以「打输 → 托管自动关」改在这里直接测推送，
    /// 不再依赖“边托管边手动防御”的场景用例。
    #[test]
    fn losing_the_battle_turns_auto_off() {
        use crate::app::{Options, Screen};
        let mut a = App::new(Options::default());
        a.screen = Screen::World;
        let mut w = WorldState::new(crate::proto::WorldEntry::default());
        let mut b = BattleState::from_start(crate::proto::BattleStart::default());
        b.phase = Phase::Input(Actor::Player);
        w.battle = Some(b);
        w.auto_battle = true;
        a.world = Some(w);

        a.on_battle_push("battle_end", serde_json::json!({"result": "lose"}));
        assert!(!a.world.as_ref().unwrap().auto_battle, "打输就不再自动出招");
        assert!(
            // STAGE-265：默认开着「打输后提示并停止」，挂机开着打输走新的停止文案（旧文案见 loss.rs 的开关关闭测试）。
            a.log.all().iter().any(|m| m.text == crate::app::battle::loss::LOSE_NOTE),
            "{:?}",
            a.log.all().iter().map(|m| m.text.clone()).collect::<Vec<_>>()
        );
    }


    /// 默认方案：攻击、补血门槛 30%、宠物补血开。
    fn dflt() -> AutoProfile {
        AutoProfile::default()
    }
    use crate::proto::{BattleStart, BattleUnit, InventorySlot, MagicView, TargetType};

    fn battle() -> BattleState {
        let unit = |no: u8, name: &str, pct: u32| BattleUnit {
            no,
            name: name.into(),
            hp_pct: pct,
            ..BattleUnit::default()
        };
        let mut b = BattleState::from_start(BattleStart {
            scene: "野外".into(),
            you: 0,
            units: vec![unit(0, "勇者", 100), unit(5, "乌力", 100), unit(15, "野猪", 100), unit(16, "野猪", 0)],
            ..BattleStart::default()
        });
        b.phase = Phase::Input(Actor::Player);
        b
    }

    fn world(items: &[(&str, u8)]) -> WorldState {
        let mut w = WorldState::new(Default::default());
        for (name, slot) in items {
            let effect = match *name {
                "肉" => "耐久力20前後回复",
                "大肉" => "耐100回复(1体)",
                "气药" => "气30回复(1人)",
                "斧头" => "攻 +9 防 -3 敏 -3",
                _ => "",
            };
            // 斧头是装备，战斗中不能用；其余是要选己方一个的回复道具。
            let target_type = if *name == "斧头" { TargetType::None } else { TargetType::Ally };
            w.inventory.slots.push(InventorySlot {
                slot: *slot,
                name: (*name).into(),
                effect: effect.into(),
                target_type,
                ..InventorySlot::default()
            });
        }
        w
    }

    #[test]
    fn plan_attacks_the_lowest_living_enemy() {
        let b = battle();
        let w = world(&[]);
        let mv = plan(&b, &w, Actor::Player, &dflt());
        assert_eq!((mv.act, mv.target), (Act::Attack, Some(15)), "16 号已倒下，打 15");
        let mv = plan(&b, &w, Actor::Pet, &dflt());
        assert_eq!((mv.act, mv.target), (Act::Attack, Some(15)), "宠物打同一个");
    }

    #[test]
    fn plan_guards_when_no_enemy_lives() {
        let mut b = battle();
        b.units.get_mut(&15).unwrap().hp_pct = 0;
        let w = world(&[]);
        let mv = plan(&b, &w, Actor::Player, &dflt());
        assert_eq!((mv.act, mv.target), (Act::Guard, None));
    }

    #[test]
    fn plan_heals_with_the_best_fitting_item_when_low() {
        let mut b = battle();
        b.units.get_mut(&0).unwrap().hp = Some(20);
        b.units.get_mut(&0).unwrap().max_hp = Some(100);
        // 缺 80：肉（20）不够补、大肉（100）够补，选够补的里最小的——大肉。
        let w = world(&[("肉", 5), ("大肉", 6)]);
        let mv = plan(&b, &w, Actor::Player, &dflt());
        assert_eq!((mv.act, mv.arg, mv.target), (Act::Item, Some(6), Some(0)));
        assert!(mv.reason.contains("大肉"));
        // 没有大肉时用最大的那个凑合；气药和装备不参与。
        let w = world(&[("气药", 5), ("斧头", 6), ("肉", 7)]);
        let mv = plan(&b, &w, Actor::Player, &dflt());
        assert_eq!(mv.arg, Some(7));
    }

    #[test]
    fn plan_heals_with_magic_when_no_item() {
        let mut b = battle();
        b.units.get_mut(&0).unwrap().hp = Some(10);
        b.units.get_mut(&0).unwrap().max_hp = Some(100);
        let mut w = world(&[("气药", 5)]);
        w.entry.status.mp = 50;
        w.entry.status.magic = vec![
            MagicView { slot: 1, name: "猛毒的精灵 Lv1".into(), mp: 10, ..MagicView::default() },
            MagicView {
                slot: 3,
                name: "治愈的精灵 Lv2".into(),
                mp: 20,
                target_type: TargetType::SelfOnly,
                ..MagicView::default()
            },
        ];
        let mv = plan(&b, &w, Actor::Player, &dflt());
        assert_eq!((mv.act, mv.arg, mv.target), (Act::Magic, Some(3), None), "自愈不用选目标");
        // 治疗他人的精灵要选己方一个，托管选自己。
        w.entry.status.magic[1].target_type = TargetType::Ally;
        assert_eq!(plan(&b, &w, Actor::Player, &dflt()).target, Some(0));
        // MP 不够就回去攻击。
        w.entry.status.mp = 5;
        assert_eq!(plan(&b, &w, Actor::Player, &dflt()).act, Act::Attack);
    }

    #[test]
    fn plan_ignores_blacklisted_slots_and_attacks() {
        let mut b = battle();
        b.units.get_mut(&0).unwrap().hp = Some(20);
        b.units.get_mut(&0).unwrap().max_hp = Some(100);
        b.bad_slots.insert(5);
        let w = world(&[("肉", 5)]);
        assert_eq!(plan(&b, &w, Actor::Player, &dflt()).act, Act::Attack, "唯一的药被拉黑，回去攻击");
    }

    #[test]
    fn plan_skips_items_that_cannot_be_used_in_battle() {
        let mut b = battle();
        b.units.get_mut(&0).unwrap().hp = Some(20);
        b.units.get_mut(&0).unwrap().max_hp = Some(100);
        let mut w = world(&[("肉", 5)]);
        w.inventory.slots[0].target_type = TargetType::None;
        assert_eq!(plan(&b, &w, Actor::Player, &dflt()).act, Act::Attack, "战斗中不能用的回复道具不选");
    }

    #[test]
    fn healthy_player_attacks_without_healing() {
        let b = battle();
        let w = world(&[("肉", 5)]);
        assert_eq!(plan(&b, &w, Actor::Player, &dflt()).act, Act::Attack);
        // 恰好在阈值上（30%）也加血：hp*100 == max*30 不大于。
        let mut low = battle();
        low.units.get_mut(&0).unwrap().hp = Some(30);
        low.units.get_mut(&0).unwrap().max_hp = Some(100);
        assert_eq!(plan(&low, &w, Actor::Player, &dflt()).act, Act::Item);
    }

    #[test]
    fn configured_guard_replaces_attack() {
        let b = battle();
        let w = world(&[]);
        let mut c = AutoProfile::default();
        c.player.act = ActKind::Guard;
        let mv = plan(&b, &w, Actor::Player, &c);
        assert_eq!((mv.act, mv.target), (Act::Guard, None));
        // 宠物也能配防御。
        c.pet.act = PetActKind::Guard;
        assert_eq!(plan(&b, &w, Actor::Pet, &c).act, Act::Guard);
    }

    #[test]
    fn configured_magic_uses_the_saved_slot_and_target_rule() {
        let b = battle();
        let mut w = world(&[]);
        w.entry.status.mp = 50;
        w.entry.status.magic = vec![
            crate::proto::MagicView { slot: 2, name: "火球".into(), mp: 10, target_type: TargetType::Enemy, ..Default::default() },
            crate::proto::MagicView { slot: 4, name: "治愈".into(), mp: 10, target_type: TargetType::Ally, ..Default::default() },
        ];
        let mut c = AutoProfile::default();
        c.player.act = ActKind::Magic;
        c.player.magic_slot = 2;
        c.player.magic_name = "火球".into();
        let mv = plan(&b, &w, Actor::Player, &c);
        assert_eq!((mv.act, mv.arg, mv.target), (Act::Magic, Some(2), Some(15)), "敌方精灵打编号最小的活敌");
        // 己方类精灵选自己，且不带目标时服务端默认自己；这里显式带上。
        c.player.magic_slot = 4;
        c.player.magic_name = "治愈".into();
        assert_eq!(plan(&b, &w, Actor::Player, &c).target, Some(0));
    }

    #[test]
    fn stale_magic_reference_falls_back_to_attack() {
        let b = battle();
        let mut w = world(&[]);
        w.entry.status.mp = 50;
        w.entry.status.magic = vec![crate::proto::MagicView {
            slot: 2,
            name: "冰锥".into(),
            mp: 10,
            target_type: TargetType::Enemy,
            ..Default::default()
        }];
        let mut c = AutoProfile::default();
        c.player.act = ActKind::Magic;
        c.player.magic_slot = 2;
        c.player.magic_name = "火球".into(); // 换过精灵：栏位还在，名字对不上
        let mv = plan(&b, &w, Actor::Player, &c);
        assert_eq!(mv.act, Act::Attack);
        assert!(mv.reason.contains("精灵已失效"), "{}", mv.reason);
    }

    #[test]
    fn not_enough_mp_falls_back_to_attack() {
        let b = battle();
        let mut w = world(&[]);
        w.entry.status.mp = 3;
        w.entry.status.magic =
            vec![crate::proto::MagicView { slot: 1, name: "火球".into(), mp: 10, target_type: TargetType::Enemy, ..Default::default() }];
        let mut c = AutoProfile::default();
        c.player.act = ActKind::Magic;
        c.player.magic_slot = 1;
        c.player.magic_name = "火球".into();
        assert_eq!(plan(&b, &w, Actor::Player, &c).act, Act::Attack);
    }

    #[test]
    fn first_turn_action_is_used_once_and_only_when_enabled() {
        let mut b = battle();
        let w = world(&[]);
        let mut c = AutoProfile::default();
        c.first.enabled = true;
        c.first.player.act = ActKind::Guard;
        b.turn = 1;
        assert_eq!(plan(&b, &w, Actor::Player, &c).act, Act::Guard, "第 1 回合用首回合动作");
        b.turn = 2;
        assert_eq!(plan(&b, &w, Actor::Player, &c).act, Act::Attack, "第 2 回合回到常规动作");
        b.turn = 1;
        c.first.enabled = false;
        assert_eq!(plan(&b, &w, Actor::Player, &c).act, Act::Attack, "关掉首回合就用常规动作");
    }

    #[test]
    fn heal_threshold_comes_from_the_profile() {
        let mut b = battle();
        let w = world(&[("肉", 5)]);
        b.units.get_mut(&0).unwrap().hp = Some(60);
        b.units.get_mut(&0).unwrap().max_hp = Some(100);
        let mut c = AutoProfile::default();
        assert_eq!(plan(&b, &w, Actor::Player, &c).act, Act::Attack, "60% 不低于默认 30%");
        c.recover.player_heal_pct = 70;
        assert_eq!(plan(&b, &w, Actor::Player, &c).act, Act::Item, "门槛调到 70% 就补");
        c.recover.player_heal = false;
        assert_eq!(plan(&b, &w, Actor::Player, &c).act, Act::Attack, "关掉自动补血就不补");
    }

    #[test]
    fn low_pet_is_healed_by_the_player() {
        let mut b = battle();
        b.units.get_mut(&5).unwrap().hp = Some(10);
        b.units.get_mut(&5).unwrap().max_hp = Some(100);
        let w = world(&[("肉", 5)]);
        let c = AutoProfile::default();
        let mv = plan(&b, &w, Actor::Player, &c);
        assert_eq!((mv.act, mv.arg, mv.target), (Act::Item, Some(5), Some(5)), "人物给战宠喂药，目标是被喂的宠");
        assert!(mv.reason.contains("回复 乌力"), "{}", mv.reason);
        // 关掉战宠补血就回去攻击。
        let mut off = c.clone();
        off.recover.pet_heal = false;
        assert_eq!(plan(&b, &w, Actor::Player, &off).act, Act::Attack);
    }

    #[test]
    fn pet_does_not_heal_itself() {
        let mut b = battle();
        b.units.get_mut(&5).unwrap().hp = Some(10);
        b.units.get_mut(&5).unwrap().max_hp = Some(100);
        let w = world(&[("肉", 5)]);
        assert_eq!(plan(&b, &w, Actor::Pet, &dflt()).act, Act::Attack, "宠物只能攻击/技能/防御");
    }

    #[test]
    fn magic_heal_is_used_when_no_item_is_left() {
        let mut b = battle();
        b.units.get_mut(&0).unwrap().hp = Some(10);
        b.units.get_mut(&0).unwrap().max_hp = Some(100);
        let mut w = world(&[]);
        w.entry.status.mp = 50;
        w.entry.status.magic =
            vec![crate::proto::MagicView { slot: 3, name: "治愈的精灵 Lv2".into(), mp: 20, target_type: TargetType::SelfOnly, ..Default::default() }];
        let mut c = AutoProfile::default();
        c.player.act = ActKind::Magic;
        c.player.magic_slot = 3;
        c.player.magic_name = "治愈的精灵 Lv2".into();
        let mv = plan(&b, &w, Actor::Player, &c);
        assert_eq!((mv.act, mv.arg), (Act::Magic, Some(3)), "没药时用治愈精灵");
    }

    /// 造一场带种类/等级/血量的战斗：`list` 是 (编号, 种类, 名字, 等级, 血量百分比)。
    fn battle_with(list: &[(u8, &str, &str, u32, u32)], escape: bool, capture: bool) -> BattleState {
        let mut units = vec![
            BattleUnit {
                no: 0,
                kind: "player".into(),
                name: "勇者".into(),
                level: 30,
                hp_pct: 100,
                ..BattleUnit::default()
            },
            BattleUnit { no: 5, kind: "pet".into(), name: "乌力".into(), level: 20, hp_pct: 100, ..BattleUnit::default() },
        ];
        units.extend(list.iter().map(|(no, kind, name, level, pct)| BattleUnit {
            no: *no,
            kind: (*kind).into(),
            name: (*name).into(),
            level: *level,
            hp_pct: *pct,
            ..BattleUnit::default()
        }));
        let mut b = BattleState::from_start(BattleStart {
            scene: "野外".into(),
            you: 0,
            allow: crate::proto::BattleAllow { escape, capture },
            units,
            ..BattleStart::default()
        });
        b.phase = Phase::Input(Actor::Player);
        b
    }

    /// 人物等级（捕捉要按“不超过自己 5 级”筛）。
    fn world_at(level: u32) -> WorldState {
        let mut w = world(&[]);
        w.entry.status.level = level;
        w
    }

    /// STAGE-173 M10：血量到门槛或敌人命中名单就逃；本场不许逃时不选。
    #[test]
    fn flee_triggers_on_low_hp_or_a_named_enemy() {
        let mut c = AutoProfile::default();
        c.flee.enabled = true;
        c.flee.hp_pct = 20;
        let b = battle_with(&[(18, "monster", "野猪", 10, 100)], true, true);

        // 满血、名单为空：不跑。
        assert!(flee_move(&b, &c).is_none());
        // 血量掉到门槛：跑。
        let mut low = battle_with(&[(18, "monster", "野猪", 10, 100)], true, true);
        low.units.get_mut(&0).unwrap().hp_pct = 15;
        let mv = flee_move(&low, &c).expect("血量到门槛要逃");
        assert_eq!(mv.act, Act::Escape);
        assert!(mv.reason.contains("20%"), "{}", mv.reason);
        // 名单命中：满血也跑。
        c.flee.names = vec!["野猪".into()];
        assert_eq!(flee_move(&b, &c).map(|m| m.act), Some(Act::Escape));
        // 名字对不上就不跑。
        c.flee.names = vec!["乌力".into()];
        assert!(flee_move(&b, &c).is_none());
        // 本场禁止逃跑：不选，落到普攻（硬发只会被服务端拒）。
        c.flee.names = vec!["野猪".into()];
        let no_escape = battle_with(&[(18, "monster", "野猪", 10, 100)], false, true);
        assert!(flee_move(&no_escape, &c).is_none());
        assert_eq!(plan(&no_escape, &world_at(30), Actor::Player, &c).act, Act::Attack);
        // 逃跑排在补血前面：又低血又名单命中时先跑。
        let mut c2 = c.clone();
        c2.recover.player_heal = true;
        c2.recover.player_heal_pct = 90;
        assert_eq!(plan(&low, &world_at(30), Actor::Player, &c2).act, Act::Escape);
    }

    /// STAGE-173 M13：名称与等级先筛，血量高于门槛先削血，到了门槛才捕捉。
    #[test]
    fn capture_screens_then_thins_then_catches() {
        let mut c = AutoProfile::default();
        c.capture.enabled = true;
        c.capture.hp_pct = 20;
        c.capture.level_min = 5;
        c.capture.level_max = 40;
        let w = world_at(30);

        // 名字筛「乌力」：野猪满血也不打，直接抓低血的乌力。
        c.capture.names = vec!["乌力".into()];
        let b = battle_with(&[(18, "monster", "野猪", 10, 100), (16, "monster", "乌力", 10, 15)], true, true);
        let mv = capture_move(&b, &w, &c).expect("有合格目标");
        assert_eq!((mv.act, mv.target), (Act::Capture, Some(16)));

        // 名字不限：先削血（用单体普攻打同一个目标，不选群攻）。
        c.capture.names.clear();
        let b = battle_with(&[(18, "monster", "野猪", 10, 100)], true, true);
        let mv = capture_move(&b, &w, &c).expect("有合格目标");
        assert_eq!((mv.act, mv.target), (Act::Attack, Some(18)));
        assert!(mv.reason.contains("削"), "{}", mv.reason);

        // 等级超出配置区间 → 不参与捕捉。
        let b = battle_with(&[(18, "monster", "野猪", 90, 100)], true, true);
        assert!(capture_move(&b, &w, &c).is_none());
        // 超过自己 5 级也抓不了（服务端规则）。
        let b = battle_with(&[(18, "monster", "野猪", 40, 100)], true, true);
        assert!(capture_move(&b, &world_at(10), &c).is_none());
        // 这场不允许捕捉 → 不参与。
        let b = battle_with(&[(18, "monster", "野猪", 10, 100)], true, false);
        assert!(capture_move(&b, &w, &c).is_none());
        // 不是怪物（玩家/宠物）不抓。
        let b = battle_with(&[(18, "player", "对手", 10, 100)], true, true);
        assert!(capture_move(&b, &w, &c).is_none());
    }

    /// STAGE-285 逃跑抓：本场没有合格目标才逃；有目标照常削血 → 抓，且不逃。
    #[test]
    fn catch_mode_escape_flees_only_without_a_target() {
        let mut c = AutoProfile::default();
        c.capture.enabled = true;
        c.capture.mode = CaptureMode::Escape;
        c.capture.hp_pct = 20;
        let w = world_at(30);

        // 没有合格目标（等级超出「不超过自己 +5」）：直接逃，并标成策略性逃跑。
        let b = battle_with(&[(18, "monster", "野猪", 90, 100)], true, true);
        let mv = plan(&b, &w, Actor::Player, &c);
        assert_eq!(mv.act, Act::Escape);
        assert!(mv.reason.contains("没有捕捉目标"), "{}", mv.reason);
        assert!(is_catch_flee(&c), "策略性逃跑要保住遇敌与挂机");

        // 有目标：满血先削血，不逃。
        let b = battle_with(&[(18, "monster", "野猪", 10, 100)], true, true);
        let mv = plan(&b, &w, Actor::Player, &c);
        assert_eq!((mv.act, mv.target), (Act::Attack, Some(18)));
        // 逃跑抓是一个模式（不是逐场判断）：只要方案是逃跑抓，任何一次逃跑都不停挂机。
        assert!(is_catch_flee(&c), "逃跑抓模式下有目标的场次也保持挂机");

        // 有目标、血量到门槛：抓，不逃。
        let b = battle_with(&[(18, "monster", "野猪", 10, 15)], true, true);
        assert_eq!(plan(&b, &w, Actor::Player, &c).act, Act::Capture);

        // 「自动逃跑」敌人名单里有这只怪，但它正是目标：让位给抓宠。
        let mut c2 = c.clone();
        c2.flee.enabled = true;
        c2.flee.names = vec!["野猪".into()];
        let b = battle_with(&[(18, "monster", "野猪", 10, 100)], true, true);
        assert_ne!(plan(&b, &w, Actor::Player, &c2).act, Act::Escape, "有目标时不因名单逃跑");

        // 自己血量到门槛：保命优先，仍然逃；这一跑也算逃跑抓（跑完继续刷，血由战后吃肉补回来）。
        let mut low = battle_with(&[(18, "monster", "野猪", 10, 100)], true, true);
        low.units.get_mut(&0).unwrap().hp_pct = 10;
        assert_eq!(plan(&low, &w, Actor::Player, &c2).act, Act::Escape);
        assert!(is_catch_flee(&c2), "逃跑抓模式下保命逃跑也不停挂机");

        // 本场不许逃：不发逃跑，照常打。
        let b = battle_with(&[(18, "monster", "野猪", 90, 100)], false, true);
        assert_ne!(plan(&b, &w, Actor::Player, &c).act, Act::Escape);

        // 边战斗边抓（默认）：没有目标也照常打，与加这一项之前一致。
        c.capture.mode = CaptureMode::Fight;
        let b = battle_with(&[(18, "monster", "野猪", 90, 100)], true, true);
        assert_ne!(plan(&b, &w, Actor::Player, &c).act, Act::Escape);
        assert!(!is_catch_flee(&c));
    }

    /// 满血抓（STAGE-264）：捕捉 hp_pct 取 100 时，满血的合格目标不削血，直接发捕捉。
    #[test]
    fn full_hp_capture_catches_without_thinning() {
        let mut c = AutoProfile::default();
        c.capture.enabled = true;
        c.capture.hp_pct = 100;
        let w = world_at(30);
        let b = battle_with(&[(18, "monster", "野猪", 10, 100)], true, true);
        let mv = capture_move(&b, &w, &c).expect("满血的合格目标");
        assert_eq!(mv.act, Act::Capture);
        assert_eq!(mv.target, Some(18));
        // 门槛 20% 的老档位不变：满血先削血。
        c.capture.hp_pct = 20;
        assert_eq!(capture_move(&b, &w, &c).unwrap().act, Act::Attack);
    }

    /// 捕捉期间本机战宠防御，不参与把目标打死。
    #[test]
    fn capture_keeps_the_pet_on_guard() {
        let mut c = AutoProfile::default();
        c.capture.enabled = true;
        c.pet.act = PetActKind::Attack;
        c.first.enabled = true;
        c.first.pet.act = PetActKind::Attack;
        let w = world_at(30);
        let b = battle_with(&[(18, "monster", "野猪", 10, 100)], true, true);
        let mv = plan(&b, &w, Actor::Pet, &c);
        assert_eq!(mv.act, Act::Guard, "捕捉中战宠防御");
        assert!(mv.reason.contains("捕捉"), "{}", mv.reason);
        // 捕捉关掉就照常打。
        c.capture.enabled = false;
        assert_eq!(plan(&b, &w, Actor::Pet, &c).act, Act::Attack);
    }

    /// 停止保护用的 App：世界在，挂机开着，方案存在临时目录里。
    fn guard_app() -> App {
        use crate::app::{Options, Screen};
        let mut a = App::new(Options::default());
        a.screen = Screen::World;
        let mut w = WorldState::new(crate::proto::WorldEntry::default());
        w.auto_battle = true;
        a.world = Some(w);
        let dir = std::env::temp_dir().join(format!("stage173-guard-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        a.auto_store = Some(crate::app::auto_battle::store::Store::open_in(&dir, "测试服", "勇者"));
        a
    }

    /// STAGE-173 M11：同时命中的条件要全部列出来，没配的（0）不算命中。
    #[test]
    fn stop_guard_lists_every_hit_condition() {
        let mut a = guard_app();
        assert!(a.stop_guard_reasons().is_none(), "默认方案不该命中任何条件");
        {
            let p = a.auto_store.as_mut().unwrap().profiles.active_mut();
            p.stop.player_level = 50;
            p.stop.party_min = 3;
            p.stop.no_supply = true;
            p.recover.player_heal = true;
        }
        a.world.as_mut().unwrap().entry.status.level = 50;
        let why = a.stop_guard_reasons().expect("两条该命中");
        assert!(why.contains("人物已到 50 级"), "{why}");
        assert!(why.contains("队伍 1 人"), "{why}");
        assert!(!why.contains("补给用完"), "满血时不需要补，不算补给不足：{why}");

        // 战斗里掉到补血门槛、背包又是空的：补给不足也命中。
        let mut b = battle_with(&[(18, "monster", "野猪", 10, 100)], true, true);
        b.units.get_mut(&0).unwrap().hp_pct = 10;
        a.world.as_mut().unwrap().battle = Some(b);
        let why = a.stop_guard_reasons().expect("三条都该命中");
        assert!(why.contains("补给用完"), "低血又没有药：{why}");
    }

    /// 抓宠时宠位满 → 停止保护；宠位没满不停。
    #[test]
    fn stop_guard_stops_when_the_pet_slots_are_full() {
        let mut a = guard_app();
        a.auto_store.as_mut().unwrap().profiles.active_mut().capture.enabled = true;
        {
            let w = a.world.as_mut().unwrap();
            for _ in 0..PET_SLOTS - 1 {
                w.pets.pets.push(Default::default());
            }
        }
        assert!(!a.stop_guard_reasons().unwrap_or_default().contains("宠物栏位"));
        a.world.as_mut().unwrap().pets.pets.push(Default::default());
        let why = a.stop_guard_reasons().expect("满了该停");
        assert!(why.contains("宠物栏位已满"), "{why}");
        // 没开自动捕捉时不看宠位。
        a.auto_store.as_mut().unwrap().profiles.active_mut().capture.enabled = false;
        assert!(a.stop_guard_reasons().is_none());
    }

    /// 命中停止保护：挂机关掉、消息里写清原因，并且不自动重开。
    #[test]
    fn stop_guard_turns_the_auto_off_and_says_why() {
        let mut a = guard_app();
        a.auto_store.as_mut().unwrap().profiles.active_mut().stop.player_level = 1;
        a.world.as_mut().unwrap().entry.status.level = 10;
        a.auto_tick(Instant::now());
        let w = a.world.as_ref().unwrap();
        assert!(!w.auto_battle, "命中停止保护就不再自动出招");
        let texts: Vec<String> = a.log.all().iter().map(|m| m.text.clone()).collect();
        assert!(texts.iter().any(|t| t.contains("停止保护（人物已到 1 级）")), "{texts:?}");
    }
}
