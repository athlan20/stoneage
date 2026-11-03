//! 战斗的客户端状态（STAGE-37 至 39）：战场上的单位、当前阶段、倒计时和战斗记录。
//! 站位见 `slots.rs`，指令见 `command.rs`，结算回放见 `playback.rs`，按键见 `keys.rs`，选目标的焦点见 `cursor.rs`，
//! 与服务端推送、回复的接入见 `net.rs`。界面设计见 `docs/battle-ui.md`。

mod auto;
pub mod brief;
mod auto_drop;
mod auto_heal;
pub(crate) mod auto_meat;
pub(crate) mod auto_post;
mod auto_revive;
mod caption;
mod capture_discard;
mod clone;
mod command;
mod cursor;
mod grid;
mod halt;
mod issue;
mod keys;
pub(crate) mod list;
pub(crate) mod list_focus;
mod list_keys;
mod loss;
mod menu;
mod glyph;
mod motion;
mod spell;
mod swap;
mod net;
mod navigation;
pub mod party;
mod playback;
mod record;
mod revise;
pub mod slots;
mod types;

use std::collections::{BTreeMap, BTreeSet, VecDeque};
use std::time::{Duration, Instant};

use crate::app::Kind;
use crate::proto::{
    BattleAllow, BattleEnd, BattleField, BattleNpc, BattleProgress, BattleRound, BattleSnapshot, BattleStart,
    BattleTurn, BattleUnit, Progress, Submitted, TargetType,
};

pub use motion::{Motions, Motion, Outcome, Style, contact_glyph, projectile_glyph};
pub use spell::{Elem, SpellFx, SpellKind, Spells, effect_glyph, element_glyph, status_glyph};
pub use swap::{SwapFx, Swaps};
pub use types::{Float, FloatKind, Fx, FxKind, HpMove, Part, Phase, Pick, Tone};

pub use clone::CloneRow;
pub use command::{Act, Actor, MenuSlot, PET_MENU, PLAYER_MENU, Typed, command_data, parse};
pub use grid::{Dir, Grid};
pub use halt::HaltState;
pub use playback::Playback;
pub use slots::Cell;

/// 战斗记录最多保留的行数；右侧栏只显示最近的几行。
const MAX_RECORDS: usize = 200;

/// 一次打飞（STAGE-167）：什么时候、朝哪个方向。方向由攻击者和受击者的站位决定。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Flown {
    pub at: Instant,
    /// 横向：+1 往右、-1 往左。纵向固定朝场外（己方往下、敌方往上）。
    pub dir: i8,
}

/// 进入战斗后客户端的全部状态。服务端给的数据在 `units`、`turn` 等字段里，
/// 本机的选择和动画状态在 `pick`、`cursor`、`acting`、`floats` 里。
#[derive(Debug)]
pub struct BattleState {
    pub prepared_row: Option<(Pick, list::Row)>,
    pub prepared_epoch: u64,
    pub scene: String,
    /// 自己（人物）的单位号；`you / 10` 是己方。
    pub you: u8,
    pub allow: BattleAllow,
    /// 本场出现过的所有单位（退场的也留着，回放里还要用到名字）。
    pub units: BTreeMap<u8, BattleUnit>,
    /// 当前在场的单位。
    pub here: BTreeSet<u8>,
    /// 被打飞过的单位（STAGE-167）：打飞和普通倒下要分开画，卡片上留一个离场标记，
    /// 并在打飞那一下沿攻击方向移出场外。值里记着时刻和方向。
    pub flown: BTreeMap<u8, Flown>,
    pub turn: u32,
    pub phase: Phase,
    /// 本回合出招的截止时间（本机时钟，由服务端的剩余毫秒换算）。
    pub deadline: Option<Instant>,
    /// 一回合的出招时限（截止时间减去回合开始的时刻），倒计时条按它算比例。
    pub turn_window: Duration,
    pub progress: Progress,
    pub pick: Pick,
    /// 指令条上动作的焦点（`Pick::Action` 时）。
    pub action_focus: usize,
    /// 列表面板里高亮的行（列表选择时，见 `list.rs`）。
    pub list_focus: usize,
    /// 本场上一次发出去的道具槽位：打开道具列表时默认高亮它（`docs/battle-ui.md`「编号规则」）。
    pub last_item: Option<u8>,
    /// 选目标时的焦点格子。
    pub cursor: Cell,
    /// 这回合可以由玩家指定攻击目标的分身（`clone:N` / `clone_pet:N`，STAGE-186，`clone.rs`）。
    pub optional: Vec<String>,
    /// 当前场地（STAGE-200）：整场战斗一个，双方共用；无属性时是 None。
    pub field: Option<BattleField>,
    /// 真人队同场（`kind` 为 `party`，STAGE-275，10.14）：队友是真人，倒计时、等待和离场按 10.14 处理。
    pub party: bool,
    /// 决斗（`kind` 为 `duel`，STAGE-307）：标题写「决斗 · 地图名」，对方是真人，只给血量百分比。
    pub duel: bool,
    /// 队员表（`battle_start.party.members`）：队长标记用；中途加入时没有，为空。
    pub members: Vec<crate::proto::BattleMember>,
    /// 这回合已经指定的分身目标：分身单位号 → 敌方单位号；新回合清空。
    pub clone_targets: BTreeMap<u8, u8>,
    /// 分身面板里高亮的行。
    pub clone_focus: usize,
    /// 指令已经发出、还没收到回复，这时不接受新的指令。
    pub submitting: bool,
    /// 人物指令已经交上、宠物指令还没交：这时 Esc 可以退回人物指令重选（STAGE-109，`revise.rs`）。
    pub revisable: bool,
    /// 正在重选人物指令（从宠物指令退回来的）：指令条写“人物指令（改）”。
    pub revising: bool,
    /// 回放速度：1 或 2。
    pub speed: u32,
    /// 正在播放的结算。
    pub playback: Option<Playback>,
    /// 回放期间收到的战斗结束，回放播完后再显示结果。
    pub end_after_replay: Option<BattleEnd>,
    /// 这一场的逃跑是「逃跑抓」的策略性逃跑（STAGE-285）：跑完还要继续刷目标，
    /// 所以 `battle_end` 收到 `escaped` 时不停挂机、日志也不写「已关闭」。
    pub catch_flee: bool,
    /// 结果窗口的内容；有值时显示窗口，Enter、Space、Esc 或打字键关闭（见 keys.rs 的 result_key）。
    pub result: Option<BattleEnd>,
    /// 战斗记录（右侧栏）。
    pub records: VecDeque<String>,
    /// 正在行动的单位及高亮的截止时间。
    pub acting: Option<(u8, Instant)>,
    /// 合击里除起头以外的出手者：和 `acting` 一起亮、一起灭（STAGE-126）。
    pub acting_with: Vec<u8>,
    pub floats: Vec<Float>,
    /// 命中拍的效果（闪、抖、让位）和血条移动（残影、回复涨血），到时间就去掉（STAGE-110，`docs/battle-ui.md`「卡片效果」）。
    pub fx: Vec<Fx>,
    pub hp_moves: Vec<HpMove>,
    /// 出手动作（STAGE-167，`motion.rs`）：谁从哪移到哪、什么时候接触，渲染据此画位移、
    /// 飞行物和接触标记。
    pub motions: Motions,
    /// 精灵表现（STAGE-167，`spell.rs`）：起手聚集、真实范围、生效形态。
    pub spells: Spells,
    /// 换宠动作链（STAGE-167，`swap.rs`）：旧宠物收回主人身上，新宠物再从主人处放出。
    pub swaps: Swaps,
    /// 中线字幕：正在播的事件写在两方前排之间（回放时才有，`docs/battle-ui.md`「中线字幕」）。
    pub caption: Vec<Part>,
    /// 正在播的事件在战斗记录里的行范围（左闭右开），右侧栏据此反色高亮。
    pub current_lines: Option<(usize, usize)>,
    /// 本次操作新增的消息，按发生顺序排队，取走后写进消息日志（`MessageLog`），战斗日志区从那里显示（STAGE-248）。
    /// 包括战斗文字（`Battle`）、装备耐久（STAGE-225）和分身倒下的队伍记录（STAGE-162，`Party`）。
    pending: Vec<(Kind, String)>,
    /// 简略日志档的状态（观察模式默认开，`brief.rs`）。
    pub brief: brief::BriefLog,
    /// 托管（STAGE-42）：下一次自动出招的时刻；None 表示这一阶段不自动出招。
    pub auto_due: Option<Instant>,
    /// 托管：这一阶段指令被拒后的重试次数，防止反复被拒空转。
    auto_retries: u32,
    /// 托管：本场被服务端拒绝过的道具槽位，不再选用。
    pub bad_slots: BTreeSet<u8>,
    /// 托管：刚由托管发出的指令（动作和参数），被拒时用来拉黑道具或重试。
    pub last_auto: Option<(Act, Option<i64>)>,
    /// 托管自动复活（STAGE-262 B 段）：本场每只宠的复活试数、被拒的精灵栏位。
    pub revive: auto_revive::ReviveLog,
    /// 结果窗口弹出的时刻；托管中到了 `auto::RESULT_HOLD` 自动关闭，不用按键。
    pub result_at: Option<Instant>,
    /// 托管「随机目标」用的随机源（STAGE-173）：每场按时间起一个种子，回合之间递推。
    pub rng: std::cell::Cell<u32>,
}

impl Phase {
    /// 按这一回合要下的指令决定阶段：先人物，再宠物；都已经交过或不需要，就是等待。
    fn for_needs(needs: &[String], submitted: Submitted) -> Self {
        let has = |k: &str| needs.iter().any(|n| n == k);
        if has("player") && !submitted.player {
            Self::Input(Actor::Player)
        } else if has("pet") && !submitted.pet {
            Self::Input(Actor::Pet)
        } else {
            Self::Waiting
        }
    }
}

/// 托管「随机目标」的种子：每场战斗按时间起一个，不同场次不会完全一样。
fn random_seed() -> u32 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.subsec_nanos() ^ d.as_secs() as u32)
        .unwrap_or(0x9E37_79B9)
}

/// 一回合的出招时限：截止时间减去服务端发出回合时的时间。
fn window_from(deadline_ms: u64, server_time_ms: u64) -> Duration {
    Duration::from_millis(deadline_ms.saturating_sub(server_time_ms))
}

/// 服务端给的剩余毫秒换成本机的截止时刻；没有给截止时间（0）时返回 None。
fn deadline_from(deadline_ms: u64, server_time_ms: u64, now: Instant) -> Option<Instant> {
    (deadline_ms > 0).then(|| now + Duration::from_millis(deadline_ms.saturating_sub(server_time_ms)))
}

/// NPC 说的话写进战斗记录的一行：`里昂蛙王：看我怎么修理你们这些人类！`（10.3、10.9 的 `npc.say`）。
fn npc_line(npc: &BattleNpc) -> String {
    format!("{}：{}", npc.name, npc.say)
}

impl BattleState {
    /// 开局（`battle_start`）：单位全部在场，还没有回合。
    pub fn from_start(start: BattleStart) -> Self {
        let party = start.kind == "party";
        let duel = start.kind == "duel";
        let members = start.party.map(|p| p.members).unwrap_or_default();
        let opening = start.npc.as_ref().filter(|n| !n.say.is_empty()).map(npc_line);
        let field = start.field.filter(BattleField::active);
        let units = start.units.into_iter().map(|u| (u.no, u)).collect();
        let mut s = Self::empty(start.scene, start.you, start.allow, units);
        s.field = field;
        s.party = party;
        s.duel = duel;
        s.members = members;
        if let Some(line) = opening {
            s.note(line); // NPC 发起的战斗：开战台词排在第一行（STAGE-96）
        }
        s.note(format!("进入战斗：{}", s.scene));
        s
    }

    /// 中途加入（`enter_world` 或 `battle` 查询带回的现状）。`phase` 为 `replay` 时由调用方补发确认。
    pub fn resume(snap: BattleSnapshot, now: Instant) -> Self {
        let party = snap.kind == "party";
        let duel = snap.kind == "duel";
        let units = snap.units.into_iter().map(|u| (u.no, u)).collect();
        let mut s = Self::empty(snap.scene, snap.you, snap.allow, units);
        s.turn = snap.turn;
        s.progress = snap.progress;
        s.field = snap.field.filter(BattleField::active);
        s.optional = snap.optional;
        s.party = party;
        s.duel = duel;
        s.deadline = deadline_from(snap.deadline_ms, snap.server_time_ms, now);
        s.turn_window = window_from(snap.deadline_ms, snap.server_time_ms);
        s.phase = if snap.phase == "replay" { Phase::Replay } else { Phase::for_needs(&snap.needs, snap.submitted) };
        s.note(format!("回到战斗：第 {} 回合", snap.turn));
        s
    }

    fn empty(scene: String, you: u8, allow: BattleAllow, units: BTreeMap<u8, BattleUnit>) -> Self {
        let here = units.keys().copied().collect();
        Self {
            scene,
            you,
            allow,
            units,
            here,
            flown: BTreeMap::new(),
            turn: 0,
            phase: Phase::Waiting,
            deadline: None,
            turn_window: Duration::ZERO,
            progress: Progress::default(),
            pick: Pick::Action,
            action_focus: 0,
            prepared_row: None,
            prepared_epoch: 0,
            list_focus: 0,
            last_item: None,
            cursor: slots::cell_of(you),
            optional: vec![],
            field: None,
            clone_targets: BTreeMap::new(),
            clone_focus: 0,
            submitting: false,
            revisable: false,
            revising: false,
            speed: 1,
            party: false,
            duel: false,
            members: vec![],
            rng: std::cell::Cell::new(random_seed()),
            playback: None,
            end_after_replay: None,
            catch_flee: false,
            result: None,
            records: VecDeque::new(),
            acting: None,
            acting_with: vec![],
            floats: vec![],
            fx: vec![],
            hp_moves: vec![],
            motions: Motions::default(),
            spells: Spells::default(),
            swaps: Swaps::default(),
            caption: vec![],
            current_lines: None,
            pending: vec![],
            brief: brief::BriefLog::default(),
            auto_due: None,
            auto_retries: 0,
            bad_slots: BTreeSet::new(),
            last_auto: None,
            revive: auto_revive::ReviveLog::default(),
            result_at: None,
        }
    }

    /// 托管中轮到下指令（`battle_turn`、人物指令后的宠物指令）：安排一次自动出招，
    /// 从此刻起留出手动接管的窗口。窗口长度来自方案（STAGE-173：默认 500ms，原来写死 1500ms）。
    pub fn schedule_auto(&mut self, now: Instant, delay: Duration) {
        self.auto_due = Some(now + delay);
        self.auto_retries = 0;
    }

    /// 托管中玩家动了手（按键选择或在打命令）：这一阶段的自动出招顺延，停手片刻托管接回。
    pub fn defer_auto(&mut self, now: Instant, hold: Duration) {
        if self.auto_due.is_some() {
            self.auto_due = Some(now + hold);
        }
    }

    /// 新回合（`battle_turn`）：重置倒计时和指令条。
    pub fn on_turn(&mut self, t: BattleTurn, now: Instant) {
        // STAGE-282：战斗已经结束（结果窗口显示中），迟到的回合推送不再把界面拉回指令阶段。
        if self.phase == Phase::Ended {
            return;
        }
        self.turn = t.turn;
        self.deadline = deadline_from(t.deadline_ms, t.server_time_ms, now);
        self.turn_window = window_from(t.deadline_ms, t.server_time_ms);
        self.progress = t.progress;
        self.optional = t.optional;
        self.field = t.field.filter(BattleField::active);
        self.clone_targets.clear();
        self.phase = Phase::for_needs(&t.needs, Submitted::default());
        self.playback = None;
        self.clear_anim();
        self.submitting = false;
        self.revisable = false;
        self.revising = false;
        self.reset_pick();
        self.note(format!("第 {} 回合", t.turn));
    }

    /// 有玩家交完指令（`battle_progress`）。
    /// 真人队同场（STAGE-275）：第一条指令被接受时带截止时间，这时起算 120 秒倒计时。
    pub fn on_progress(&mut self, p: BattleProgress, now: Instant) {
        self.progress = Progress { done: p.done, total: p.total };
        if let Some(deadline) = deadline_from(p.deadline_ms, p.server_time_ms, now) {
            self.deadline = Some(deadline);
            self.turn_window = window_from(p.deadline_ms, p.server_time_ms);
        }
    }

    /// 一回合的结算（`battle_round`）：开始播放，播完再确认。
    pub fn on_round(&mut self, round: BattleRound, now: Instant) {
        // STAGE-282：同上，结束后收到的旧回合不重播、不确认，免得界面卡在“结算中”。
        if self.phase == Phase::Ended {
            return;
        }
        self.note_party_escapes(&round.events);
        self.phase = Phase::Replay;
        self.submitting = false;
        self.revisable = false;
        self.revising = false;
        self.reset_pick();
        self.playback = Some(Playback::new(round, now));
    }

    /// 战斗结束（`battle_end`）：正在回放就等播完再弹窗，否则马上弹。
    pub fn on_end(&mut self, end: BattleEnd) {
        if self.phase == Phase::Replay {
            self.end_after_replay = Some(end);
        } else {
            self.show_result(end);
        }
    }

    /// 显示结果窗口：写一行战斗记录，阶段进入结束，倒计时不再显示。NPC 赢了有结束台词的，先写台词一行（STAGE-96）。
    pub fn show_result(&mut self, end: BattleEnd) {
        if let Some(npc) = end.npc.as_ref().filter(|n| !n.say.is_empty()) {
            self.note(npc_line(npc));
        }
        // 分身倒下：战斗结束时还倒着的分身写一条队伍记录（战后自动恢复满血，STAGE-162）。
        let downed: Vec<&str> = self.units.values().filter(|u| u.auto && u.kind == "player" && u.hp_pct == 0).map(|u| u.name.as_str()).collect();
        if !downed.is_empty() {
            let text = format!("分身倒下：{}（战后自动恢复满血）", downed.join("、"));
            self.pending.push((Kind::Party, text));
        }
        let line = match end.result.as_str() {
            "win" => "战斗胜利",
            "lose" => "战斗失败",
            "escaped" => "你逃离了战斗",
            "forced_out" => "超时未下指令，被移出战斗",
            _ => "战斗结束，没有分出胜负",
        };
        self.note(line.to_string());
        self.phase = Phase::Ended;
        self.result = Some(end);
        self.result_at = Some(Instant::now());
        self.auto_due = None;
        self.deadline = None;
        self.clear_anim();
        self.reset_pick();
    }

    /// 取走本次新增的消息（按发生的顺序），写进消息日志（STAGE-248）。
    pub fn take_pending(&mut self) -> Vec<(Kind, String)> {
        std::mem::take(&mut self.pending)
    }

    /// 本场有没有分身队的单位（结果窗口据此注明“分身不领奖励”，STAGE-162）。
    pub fn has_clones(&self) -> bool {
        self.units.values().any(|u| u.auto)
    }

    /// 记一行战斗记录：进战斗记录（回放高亮用），同时排队写进消息日志（战斗日志区从那里显示，STAGE-248）。
    fn note(&mut self, text: String) {
        self.note_kind(Kind::Battle, text);
    }
    /// 同 `note`，排队写进消息日志时用给定的类别（队伍记录是绿色的 `Party`，STAGE-275）。
    fn note_kind(&mut self, kind: Kind, text: String) {
        self.note_record(text.clone());
        self.pending.push((kind, text));
    }
    /// 只进战斗记录（回放高亮用），不写消息日志（简略档里被汇总掉的事件）。
    fn note_record(&mut self, text: String) {
        self.records.push_back(text);
        while self.records.len() > MAX_RECORDS {
            self.records.pop_front();
            // 最早的记录被丢掉了，高亮的范围跟着往前挪；整段都丢掉就不再高亮。
            self.current_lines = self.current_lines.and_then(|(s, e)| (e > 1).then(|| (s.saturating_sub(1), e - 1)));
        }
    }

    pub fn reset_pick(&mut self) {
        self.prepared_row=None;
        self.pick = Pick::Action;
        self.action_focus = 0;
        self.list_focus = 0;
    }

    pub fn me_side(&self) -> u8 {
        self.you / 10
    }

    /// 正在下指令的单位号（STAGE-106）：人物指令是自己，宠物指令是出战宠物（主人的号码加 5）；
    /// 不在下指令时返回 None。界面用它高亮卡片。
    pub fn commander(&self) -> Option<u8> {
        match self.phase {
            Phase::Input(Actor::Player) => Some(self.you),
            Phase::Input(Actor::Pet) => Some(self.you + 5),
            _ => None,
        }
    }

    /// 单位的名字；没有见过的单位用协议号代替。
    pub fn name(&self, no: u8) -> String {
        self.units.get(&no).map_or_else(|| format!("#{no}"), |u| u.name.clone())
    }

    /// 格子里现在有没有单位（在场的）。
    pub fn unit_at(&self, cell: Cell) -> Option<&BattleUnit> {
        let no = slots::no_at(cell);
        if self.here.contains(&no) { self.units.get(&no) } else { None }
    }

    /// 格子里有单位，而且还活着（没有倒下）。
    pub fn alive_at(&self, cell: Cell) -> bool {
        self.unit_at(cell).is_some_and(|u| u.hp_pct > 0)
    }

    /// 选目标的起点：己方一个落在自己身上（回复类道具、精灵），复活落在己方第一个倒下的单位上，
    /// 敌方一个落在第一个还活着的敌人上。
    pub fn default_cursor(&self, kind: TargetType) -> Cell {
        if kind == TargetType::Ally {
            return slots::cell_of(self.you);
        }
        if kind == TargetType::AllyDown {
            let side = self.me_side();
            return self
                .cells_of(side)
                .find(|&cell| {
                    self.unit_at(cell).is_some_and(|u| u.hp_pct == 0 && u.kind != "monster")
                        && !self.is_mate(slots::no_at(cell))
                })
                .unwrap_or_else(|| slots::cell_of(self.you));
        }
        let side = slots::other_side(self.me_side());
        self.cells_of(side).find(|&cell| self.alive_at(cell)).unwrap_or(Cell { side, front: true, col: 0 })
    }

    /// 一方的全部格子，前排在前，各排从左到右。
    fn cells_of(&self, side: u8) -> impl Iterator<Item = Cell> {
        [true, false].into_iter().flat_map(move |front| (0..5).map(move |col| Cell { side, front, col }))
    }

    /// 倒计时还剩几秒（向上取整，刚开始时显示 30 而不是 29）；没有截止时间返回 None。
    pub fn remaining_secs(&self, now: Instant) -> Option<u64> {
        self.deadline.map(|d| (d.saturating_duration_since(now).as_millis().div_ceil(1000)) as u64)
    }
}

#[cfg(test)]
mod tests;
