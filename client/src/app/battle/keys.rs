//! 战斗中的按键（STAGE-38，`docs/battle-ui.md`「指令输入」）：数字键、方向键、回车、Esc 的选择流程。
//! 三种方式效果相同、可以混用：数字键、方向键加回车、`/` 命令栏。非输入层不接收打字，
//! 战斗时数字键不切换菜单（`1`–`7` 在输入行为空时才是选择）。发出指令见 `issue.rs`。
//! 道具、精灵、技能选好以后，不用选目标的直接出招，要选的只能在能选的那一方里选（STAGE-78）。

use std::time::Instant;

use crossterm::event::{KeyCode, KeyEvent};

use super::command;
use super::cursor::move_cursor;
use super::grid::Dir;
use super::issue::{disabled_reason, target_kind};
use super::list;
use super::list_focus::{self, PAGE_STEP};
use super::slots;
use super::{Act, Actor, BattleState, MenuSlot, Phase, Pick};
use crate::app::App;
use crate::proto::TargetType;

impl App {
    /// 战斗中的按键。返回 false 表示这个键交给底部输入行（普通字符，或者不是战斗命令的文字命令）。
    pub(crate) fn battle_key(&mut self, key: KeyEvent) -> bool {
        if self.battle_open_bar(key){return true;}
        if self.world.as_ref().is_some_and(|w|!w.online){self.log.error("战斗快照已离线，Tab 打开系统重新连接");return true;}
        let Some(battle) = self.battle_ref() else {
            return false;
        };
        // 观察模式（agent 操控）下 Esc 打开游戏菜单查看背包、宠物等页面，战斗留在后台（STAGE-182）。
        // 自己操控时保持原样；结果窗口开着时 Esc 仍是关结果窗口。
        if key.code == KeyCode::Esc && battle.result.is_none() && self.world.as_ref().is_some_and(|w| w.observing()) {
            if let Some(w) = self.world.as_mut() {
                w.open_battle_veil();
            }
            return true;
        }
        if battle.result.is_some() {
            return self.result_key(key);
        }
        // 托管中（STAGE-173）：不再摆“请你选招”的流程，按键只留「8 停止」与 Tab 面板。
        // 分身面板不受托管锁限制（STAGE-186）：分身不是本机角色出招，托管中照样能指定它们的目标。
        if self.auto_locks_commands() && !clone_key_exempt(battle.pick, key.code) {
            return self.auto_locked_key(key);
        }
        let (phase, pick) = (battle.phase, battle.pick);
        let empty = true;
        // 托管中玩家动手选指令或目标：自动出招顺延，停手片刻托管接回（STAGE-42）。
        let manual = empty
            && matches!(phase, Phase::Input(_))
            && matches!(
                key.code,
                KeyCode::Char('0'..='9' | 'f') | KeyCode::Up | KeyCode::Down | KeyCode::Left | KeyCode::Right | KeyCode::Esc
            );
        if manual && self.battle_mut().is_some_and(|b| b.auto_due.is_some()) {
            let hold = self.auto_hold();
            self.battle_mut().map(|b| b.defer_auto(Instant::now(), hold));
        }
        match key.code {
            KeyCode::Char(c @ '0'..='9') if empty => {
                // 0 是第 10 个位置。观察模式下轮到下指令时拦下（STAGE-104）。
                if !self.battle_blocked() {
                    let digit = c.to_digit(10).map_or(0, |d| d as u8);
                    self.battle_digit(if digit == 0 { 10 } else { digit });
                }
                true
            }
            // 指定分身的攻击目标（STAGE-186）：只在指令条第一层、轮到下指令时打开分身面板。
            KeyCode::Char('f') if empty && matches!(phase, Phase::Input(_)) && pick == Pick::Action => {
                if !self.battle_blocked() {
                    self.open_clones();
                }
                true
            }
            KeyCode::Up | KeyCode::Down | KeyCode::Left | KeyCode::Right if empty => {
                self.battle_move(key.code);
                true
            }
            KeyCode::PageUp | KeyCode::PageDown if empty && pick.is_list() => {
                self.list_move(pick, PAGE_STEP, key.code == KeyCode::PageDown);
                true
            }
            // 输入行有字时 Esc 和普通输入一样，不改变选择（Esc 清空输入行）。
            KeyCode::Esc if empty && pick != Pick::Action => {
                self.battle_back();
                true
            }
            KeyCode::Esc if empty => {self.battle_revert();true},
            KeyCode::Enter => self.battle_enter(),
            _ => true,
        }
    }

    /// 托管中轮到本机下指令：战斗键不再走选招（STAGE-173「一开托管就不再让人选行动」）。
    fn auto_locks_commands(&self) -> bool {
        let Some(b) = self.battle_ref() else {
            return false;
        };
        matches!(b.phase, Phase::Input(_))
            && !b.submitting
            && self.world.as_ref().is_some_and(|w| w.auto_battle)
    }

    /// 托管锁住选招时的提示；这回合有分身可指定目标的，补一句 `f`（STAGE-186）。
    fn auto_lock_hint(&self) -> String {
        let clones = self.battle_ref().is_some_and(|b| !b.clone_rows().is_empty());
        let tail = if clones { "；f 指定分身的攻击目标" } else { "" };
        format!("自动挂机中：按 8 停止自动挂机后可以手动出招；按 7 只停自动遇敌；Tab 打开控制面板改设置{tail}")
    }

    /// 托管中按下的战斗键：`8` 是“停止自动挂机”（人物第 8 格、宠物第 4 格的管理项编号），
    /// 其余选招键只给一句提示，不改选择、不发指令。
    fn auto_locked_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Char('8') => {
                self.stop_auto_all("战斗菜单");
            }
            KeyCode::Char(c) if c.is_ascii_digit() => {
                let hint = self.auto_lock_hint();
                self.log.system(hint);
            }
            KeyCode::Up | KeyCode::Down | KeyCode::Left | KeyCode::Right | KeyCode::Enter | KeyCode::Esc => {
                let hint = self.auto_lock_hint();
                self.log.system(hint);
            }
            _ => {}
        }
        true
    }

    /// 自动战斗中的两个全局快捷键（STAGE-173）：
    /// `8` 停止自动战斗（连带关后续遇敌），`7` 只停自动遇敌、自动出招继续。
    /// 只要自动战斗开着、战斗还在、又没在打字，任何阶段（下指令/结算/等待）都认。
    pub(crate) fn auto_hotkey(&mut self, code: KeyCode) -> bool {
        let Some(w) = self.world.as_ref() else {
            return false;
        };
        if !w.auto_battle || w.battle.is_none() {
            return false;
        }
        // 正在打字时 8 是文字：命令栏、底部输入行有字、暗号窗口、确认框、方案命名、名单录入与等级手输。
        let typing = !w.input.is_empty()
            || w.nav.is(&crate::app::Layer::Command)
            || w.npc_window.as_ref().is_some_and(|n| n.window_id.is_empty())
            || (w.confirm.is_some() && !w.confirm_native)
            || w.auto_edit.as_ref().is_some_and(|e| e.naming.is_some() || e.names.is_some() || e.level.is_some());
        if typing {
            return false;
        }
        match code {
            KeyCode::Char('8') => self.stop_auto_all("快捷键 8"),
            KeyCode::Char('7') => {
                // 只关后续遇敌：这一场照常打完，托管的出招也不停。
                self.log.system("按 7 停止自动遇敌：自动战斗继续，之后不再开新的一场");
                self.stop_encounter_only();
            }
            _ => return false,
        }
        true
    }

    /// 结果窗口只由可见继续键关闭，普通字符不输入、不关闭。
    fn result_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Enter | KeyCode::Esc => self.close_battle(),
            KeyCode::Char(' ') => self.close_battle(),
            _ => return true,
        }
        true
    }

    pub(super) fn battle_ref(&self) -> Option<&BattleState> {
        self.world.as_ref()?.battle.as_ref()
    }

    pub(super) fn battle_mut(&mut self) -> Option<&mut BattleState> {
        self.world.as_mut()?.battle.as_mut()
    }

    /// 结果窗口关闭：战斗结束，菜单停在进入战斗前的页面（战斗期间没有切换过页面）。
    /// 托管中到了 `auto::RESULT_HOLD` 也自动走这里（STAGE-42）。
    pub(super) fn close_battle(&mut self) {
        self.flush_battle_log();
        if let Some(w) = self.world.as_mut() {
            let lost=w.battle.as_ref().and_then(|b|b.result.as_ref()).is_some_and(|r|r.result=="lose");
            // 有结算、挂机开着：收起战斗后跑一轮战后补血与丢弃（STAGE-260）。
            let post = w.auto_battle && w.battle.as_ref().and_then(|b|b.result.as_ref()).is_some();
            w.battle = None;
            if post { w.post_battle.arm(); }
            if w.battle_veil { // 玩家正停在菜单页里，战斗结束不强行拉回（STAGE-182）
                w.battle_return = None;
            } else if lost {w.nav.reset();w.battle_return=None;} else if let Some(nav)=w.battle_return.take(){w.nav=nav;}
        }
    }

    /// 轮到哪个角色下指令；不在下指令的阶段返回 None。
    pub(super) fn input_actor(&self) -> Option<Actor> {
        match self.battle_ref()?.phase {
            Phase::Input(actor) => Some(actor),
            _ => None,
        }
    }

    /// 观察模式（agent 操控）下轮到下指令时，出招的按键（数字、回车、文字命令）被拦下（STAGE-104）：写一条提示，返回 true。
    /// 战斗中不回显命令（docs/interaction.md「快捷键与命令」），只提示。
    fn battle_blocked(&mut self) -> bool {
        self.input_actor().is_some() && self.observer_blocks()
    }

    /// 回车：输入行有内容时先当作文字命令（不是战斗命令就交给普通命令）；输入行为空时跳过回放或确认焦点。
    pub(crate) fn battle_enter(&mut self) -> bool {
        let line = self.world.as_ref().filter(|w|w.nav.is(&crate::app::Layer::Command)).map(|w| w.input.text().trim().to_string()).unwrap_or_default();
        if !line.is_empty() {
            if let Some(parsed) = super::clone::parse(&line) {
                if let Some(w) = self.world.as_mut() {
                    w.history.push(&line);
                    w.input.clear();
                }
                if self.battle_blocked() {
                    return true;
                }
                match parsed {
                    Ok((who, target)) => self.run_clone_typed(who, target),
                    Err(msg) => self.log.error(msg),
                }
                return true;
            }
            let Some(parsed) = command::parse(&line) else {
                return false;
            };
            if let Some(w) = self.world.as_mut() {
                w.history.push(&line);
                w.input.clear();
            }
            if self.battle_blocked() {
                return true;
            }
            // 托管中的文字出招同样拒绝（STAGE-173）：想手动先按 8 停止。
            if self.auto_locks_commands() {
                self.log.system("自动挂机中：按 8 停止自动挂机后可以手动出招；按 7 只停自动遇敌");
                return true;
            }
            match parsed {
                Ok(typed) => self.run_typed(typed),
                Err(msg) => self.log.error(msg),
            }
            return true;
        }
        if self.battle_blocked() {
            return true;
        }
        let Some(battle) = self.battle_ref() else {
            return true;
        };
        let (phase, pick) = (battle.phase, battle.pick);
        match (phase, pick) {
            (Phase::Input(actor), Pick::Action) => {
                let focus = battle.action_focus;
                if let Some(&slot) = battle.menu(actor).get(focus) {
                    self.choose_slot(actor, slot);
                }
            }
            (Phase::Input(actor), Pick::Target { act, extra, kind }) => {
                let cell = battle.cursor;
                self.confirm_cell(actor, act, extra, kind, cell);
            }
            (Phase::Input(_), Pick::Clones) => self.clones_enter(),
            (Phase::Input(_), Pick::CloneAim { who }) => {
                let cell = battle.cursor;
                self.confirm_clone_cell(who, cell);
            }
            (Phase::Input(actor), pick) if pick.is_list() => self.list_enter(actor, pick),
            _ => {}
        }
        true
    }

    /// 数字键：按当前的选择（指令条、列表或目标）选中第几个。列表的号见 `list_keys.rs`。
    fn battle_digit(&mut self, number: u8) {
        let Some(battle) = self.battle_ref() else {
            return;
        };
        let Phase::Input(actor) = battle.phase else {
            return;
        };
        let pick = battle.pick;
        match pick {
            Pick::Action => {
                if let Some(&slot) = battle.menu(actor).get(usize::from(number) - 1) {
                    self.choose_slot(actor, slot);
                }
            }
            Pick::Target { act, extra, kind } => {
                if let Some(cell) = slots::cell_of_number(battle.cursor.side, number) {
                    self.confirm_cell(actor, act, extra, kind, cell);
                }
            }
            Pick::Clones => self.clones_digit(number),
            Pick::CloneAim { who } => {
                if let Some(cell) = slots::cell_of_number(slots::other_side(battle.me_side()), number) {
                    self.confirm_clone_cell(who, cell);
                }
            }
            _ => self.list_digit(actor, pick, number),
        }
    }

    /// 指令条上选中一个格子：普通动作走出招流程，管理项开关自动挂机（STAGE-153）。
    /// 管理项不发 `battle_command`、不消耗回合，也不把焦点挪到别的格子。
    fn choose_slot(&mut self, actor: Actor, slot: MenuSlot) {
        match slot {
            MenuSlot::Act(act) => self.choose_action(actor, act),
            MenuSlot::AutoToggle => self.toggle_auto_manage(),
        }
    }

    /// 管理项：开着就停（联动关遇敌），关着就开（只开自动出招，不碰遇敌）。长按的 Repeat
    /// 事件在 `on_key` 里已经被丢掉，这里只处理真正按下的那一次。
    fn toggle_auto_manage(&mut self) {
        let Some(on) = self.world.as_ref().map(|w| w.auto_battle) else {
            return;
        };
        if on {
            self.stop_auto_all("战斗菜单");
        } else {
            self.start_auto("战斗菜单");
        }
    }

    /// 选中一个动作：需要目标的进入选目标，换宠、道具进入对应的列表，防御和逃跑直接下指令。
    fn choose_action(&mut self, actor: Actor, act: Act) {
        let enabled = match (self.battle_ref(), self.world.as_ref()) {
            (Some(b), Some(w)) => b.enabled(w, actor, act),
            _ => false,
        };
        if !enabled {
            self.log.error(disabled_reason(act));
            return;
        }
        match act {
            Act::Attack | Act::Capture => self.start_target(act, None),
            Act::Item => self.set_pick(Pick::Item),
            Act::Skill => self.set_pick(Pick::Skill),
            Act::SwapPet => self.set_pick(Pick::Pet),
            Act::Guard | Act::Escape => {
                self.send_command(actor, act, None, None);
            }
            Act::Magic => self.set_pick(Pick::Magic),
        }
    }

    /// 切换到另一个选择；列表打开时高亮落在默认的一行上（道具优先本场上次用过的）。
    fn set_pick(&mut self, pick: Pick) {
        let last = self.battle_ref().and_then(|b| b.last_item);
        let focus = self.world.as_ref().map_or(0, |w| list_focus::default_focus(&list::rows(w, pick), pick, last));
        if let Some(b) = self.battle_mut() {
            b.pick = pick;
            b.list_focus = focus;
        }
    }

    /// 选好了道具、精灵或技能的编号：不用选目标的（自愈、全体、只能对自己的）直接出招，要选的进入选目标。
    pub(super) fn choose_thing(&mut self, actor: Actor, act: Act, slot: i64) {
        let kind = self.world.as_ref().map_or(TargetType::None, |w| target_kind(w, act, Some(slot)));
        if kind.picks_unit() {
            self.start_target(act, Some(slot));
        } else {
            self.send_command(actor, act, None, Some(slot));
        }
    }

    /// 进入选目标：焦点落在能选的那一方的默认位置上（己方一个在自己身上，敌方一个在第一个活着的敌人上）。
    /// 只能选的那一方由目标类型决定，之后方向键和数字键都只在这一方里移动，不用 Tab 切换。
    fn start_target(&mut self, act: Act, extra: Option<i64>) {
        let kind = self.world.as_ref().map_or(TargetType::Enemy, |w| target_kind(w, act, extra));
        let Some(b) = self.battle_mut() else {
            return;
        };
        b.cursor = b.default_cursor(kind);
        b.pick = Pick::Target { act, extra, kind };
    }

    /// Esc：退回上一步（道具和技能的目标退回列表，其余退回指令条）。
    fn battle_back(&mut self) {
        let Some(b) = self.battle_mut() else {
            return;
        };
        let next = match b.pick {
            Pick::CloneAim { .. } => Pick::Clones,
            Pick::Target { act: Act::Item, .. } => Pick::Item,
            Pick::Target { act: Act::Magic, .. } => Pick::Magic,
            Pick::Target { act: Act::Skill, .. } => Pick::Skill,
            _ => Pick::Action,
        };
        b.pick = next;
    }

    /// 宠物指令的第一层按 Esc：退回人物指令重选（STAGE-109）。不能退回时返回 false，Esc 交给输入行，什么也不做。
    fn battle_revert(&mut self) -> bool {
        if !self.battle_ref().is_some_and(BattleState::can_revert) {
            return false;
        }
        if !self.battle_blocked() {
            self.battle_mut().map(BattleState::revert_to_player);
        }
        true
    }

    /// 方向键：列表里上下移动高亮；指令条上按网格移动（左右换列、上下换行，STAGE-153）；
    /// 选目标时上下换排、左右换列。
    fn battle_move(&mut self, code: KeyCode) {
        let Some(actor) = self.input_actor() else {
            return;
        };
        let Some(pick) = self.battle_ref().map(|b| b.pick) else {
            return;
        };
        if pick.is_list() {
            if matches!(code, KeyCode::Up | KeyCode::Down) {
                self.list_move(pick, 1, code == KeyCode::Down);
            }
            return;
        }
        let dir = match code {
            KeyCode::Left => Some(Dir::Left),
            KeyCode::Right => Some(Dir::Right),
            KeyCode::Up => Some(Dir::Up),
            KeyCode::Down => Some(Dir::Down),
            _ => None,
        };
        let Some(b) = self.battle_mut() else {
            return;
        };
        match b.pick {
            // 网格位置和画面对应：左右只在当前行换列，上下到相邻行同一列，到边界停住。
            Pick::Action => {
                if let Some(dir) = dir {
                    b.action_focus = b.grid(actor).step(b.action_focus, dir);
                }
            }
            Pick::Target { .. } | Pick::CloneAim { .. } => move_cursor(b, code),
            Pick::Clones => {
                if matches!(code, KeyCode::Up | KeyCode::Down) {
                    b.clone_move(code == KeyCode::Down);
                }
            }
            _ => {}
        }
    }

    pub(crate) fn jump_replay(&mut self) {
        let done = self.battle_mut().is_some_and(|b| b.skip());
        if done {
            self.after_replay();
        }
    }
}

/// 托管锁放行的键（STAGE-186）：指令条第一层按 `f` 打开分身面板，以及分身面板和选目标界面里的所有键。
fn clone_key_exempt(pick: Pick, code: KeyCode) -> bool {
    matches!(pick, Pick::Clones | Pick::CloneAim { .. }) || (pick == Pick::Action && code == KeyCode::Char('f'))
}
