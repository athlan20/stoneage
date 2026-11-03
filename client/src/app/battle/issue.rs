//! 把指令发给服务端（STAGE-38，`docs/battle-ui.md`「指令输入」「命令表」）：目标的合法性、
//! 换宠、文字命令的解析结果，以及 `battle_command` 的发送。按键的选择流程在 `keys.rs`。
//! 精灵、道具、技能要不要选目标、能选哪一方，看它们的目标类型（STAGE-78，`target_kind`）。

use super::{BattleState, Phase};
use crate::app::auto_battle::capture_keep::{CaptureKeep, attach_keep};
use super::command::{Act, Actor, Typed, command_data};
use super::slots::{self, Cell};
use crate::app::session::Pending;
use crate::app::{App, WorldState};
use crate::proto::{Pet, TargetType};

impl App {
    /// 在格子上确认目标：不合法的（没有单位、倒下、一方不对）留在选择里，在消息栏说明原因。
    pub(super) fn confirm_cell(&mut self, actor: Actor, act: Act, extra: Option<i64>, kind: TargetType, cell: Cell) {
        let no = slots::no_at(cell);
        let check = match self.battle_ref() {
            Some(b) => check_target(b, kind, act, no),
            None => return,
        };
        match check {
            Ok(()) => {
                self.send_command(actor, act, Some(no), extra);
            }
            Err(msg) => self.log.error(msg),
        }
    }

    /// 文字命令（`attack e3`、`g`…）：按当前阶段的角色下指令，参数在这里检查。没写目标时：
    /// 要选敌方一个的必须写，己方一个缺省是自己，不用选的不带目标（同按键选择）。
    pub(super) fn run_typed(&mut self, typed: Typed) {
        if let Some(b)=self.battle_mut(){b.prepared_row=None;}
        let Some(actor) = self.input_actor() else {
            self.log.error("现在不用下指令");
            return;
        };
        if !typed.act.allowed_for(actor) {
            self.log.error(format!("{}里没有{}", actor.title(), typed.act.label()));
            return;
        }
        let kind = self.world.as_ref().map_or(TargetType::None, |w| target_kind(w, typed.act, typed.arg));
        let Some(battle) = self.battle_ref() else {
            return;
        };
        let target = match typed.target {
            Some((side, number)) => {
                // 数字前没写 m/e 时，要选敌方的动作是敌方，其余是我方。
                // 属性反转（any）没写 m/e 时按敌方，与默认落点一致。
                let mine = side.unwrap_or(kind != TargetType::Enemy && kind != TargetType::Any);
                let side = if mine { battle.me_side() } else { slots::other_side(battle.me_side()) };
                match slots::cell_of_number(side, number).map(slots::no_at) {
                    Some(no) => match check_target(battle, kind, typed.act, no) {
                        Ok(()) => Some(no),
                        Err(msg) => {
                            self.log.error(msg);
                            return;
                        }
                    },
                    None => None,
                }
            }
            None if kind == TargetType::Enemy || kind == TargetType::Any => {
                self.log.error(format!("{}要写目标，例如 e3（敌方 3 号）", typed.act.label()));
                return;
            }
            None => None,
        };
        self.send_command(actor, typed.act, target, typed.arg);
    }

    /// 发出一条指令；返回是否真的发出去了。上一条还没有回复时不再发，避免同一个指令发两次。
    pub(super) fn send_command(&mut self, actor: Actor, act: Act, target: Option<u8>, arg: Option<i64>) -> bool {
        self.send_command_keep(actor, act, target, arg, None)
    }

    /// 同上，`keep` 是自动捕捉的资质筛选（STAGE-264，协议 10.5 的 `capture.keep`）；手动出招不带。
    pub(super) fn send_command_keep(
        &mut self,
        actor: Actor,
        act: Act,
        target: Option<u8>,
        arg: Option<i64>,
        keep: Option<&CaptureKeep>,
    ) -> bool {
        if self.observer_blocks(){return false;}
        if let (Some(w),Some(b))=(self.world.as_ref(),self.battle_ref()){
            if act==Act::Item && b.prepared_row.is_some() && b.prepared_epoch!=w.inventory_epoch{self.log.error("背包已更新，请重新选择道具");return false;}
            if !b.enabled(w,actor,act){self.log.error(disabled_reason(act));return false;}
            let pick=match act{Act::Item=>Some(super::Pick::Item),Act::Magic=>Some(super::Pick::Magic),Act::SwapPet=>Some(super::Pick::Pet),Act::Skill=>Some(super::Pick::Skill),_=>None};
            if let Some(pick)=pick{
                let row=super::list::rows(w,pick).into_iter().find(|r|Some(r.key)==arg);
                let Some(row)=row else{self.log.error("对象已不存在，请重新选择");return false;};
                if let Some(reason)=row.blocked.as_ref(){self.log.error(reason.clone());return false;}
                if b.prepared_row.as_ref().is_some_and(|(p,old)|*p==pick && *old!=row){self.log.error("对象已变化，请重新选择");return false;}
            }
        }
        // 逃跑抓（STAGE-285）：手动按逃跑也按方案记账，战斗结束时据此决定停不停挂机。
        let catch_flee = act == Act::Escape && super::auto::is_catch_flee(self.auto_profile());
        let Some(b) = self.battle_mut() else {
            return false;
        };
        // STAGE-282：战斗已经结束（结果窗口显示中）就不再发任何指令，托管的自动出招也一样。
        if b.submitting || b.phase == Phase::Ended {
            return false;
        }
        if act == Act::Escape {
            b.catch_flee = catch_flee;
        }
        let mut data = command_data(b.turn, actor, act, target, arg);
        if let Some(keep) = keep {
            attach_keep(&mut data, keep);
        }
        b.submitting = true;
        b.last_auto = None;
        if act == Act::Item {
            b.last_item = arg.and_then(|slot| u8::try_from(slot).ok());
        }
        let turn=b.turn;
        self.send("battle_command", data, Pending::BattleCommand { turn })
    }
}

/// 动作不能选的原因，显示在消息栏。
pub(super) fn disabled_reason(act: Act) -> String {
    match act {
        Act::Magic => "还没有学会精灵".into(),
        Act::Capture => "这场战斗不允许捕捉".into(),
        Act::Escape => "这场战斗不允许逃跑".into(),
        Act::Item => "背包里没有可以用的道具".into(),
        Act::SwapPet => "没有可以换上场的宠物".into(),
        Act::Skill => "宠物没有可用的技能".into(),
        _ => "现在不能这样做".into(),
    }
}

/// 这个动作的目标类型（`docs/protocol.md` 10.5）：攻击和捕捉打敌方一个；道具、精灵、技能看所选的那一项的
/// `target_type`；防御、逃跑、换宠没有目标。所选的项找不到（栏位空了）时按 `none`，服务端会回复错误。
pub(super) fn target_kind(w: &WorldState, act: Act, extra: Option<i64>) -> TargetType {
    let n = extra.unwrap_or(-1);
    match act {
        Act::Attack | Act::Capture => TargetType::Enemy,
        Act::Item => w.inventory.slots.iter().find(|s| i64::from(s.slot) == n).map_or(TargetType::None, |s| s.target_type),
        Act::Magic => {
            w.entry.status.magic.iter().find(|m| i64::from(m.slot) == n).map_or(TargetType::None, |m| m.target_type)
        }
        Act::Skill => battle_pet(w)
            .and_then(|p| p.skills.iter().find(|s| i64::from(s.slot) == n))
            .map_or(TargetType::None, |s| s.target_type),
        Act::Guard | Act::Escape | Act::SwapPet => TargetType::None,
    }
}

/// 这一格的单位能不能被选为目标（界面的光标和按键共用；队友只能选回复、复活类，STAGE-275）。
impl BattleState {
    pub fn can_pick(&self, kind: TargetType, act: Act, no: u8) -> bool {
        check_target(self, kind, act, no).is_ok()
    }
}

/// 目标是否合法（本地先挡一下明显的错，服务端仍会校验）：单位要在场；要选的一方要对（敌方一个不能选己方，
/// 己方一个不能选敌方）；要选的单位不能已经倒下。不用选目标的类型（`none` 等）不看一方。
pub(super) fn check_target(b: &BattleState, kind: TargetType, act: Act, no: u8) -> Result<(), String> {
    if !b.here.contains(&no) {
        return Err("那个位置没有单位".into());
    }
    if b.is_mate(no) && !mate_support(kind, act) {
        return Err("队友由他自己指挥，不能给他下指令".into());
    }
    let mine = slots::cell_of(no).side == b.me_side();
    let unit = b.units.get(&no);
    let down = unit.is_some_and(|u| u.hp_pct == 0);
    match kind {
        TargetType::Enemy if mine => Err(format!("{}只能选敌方", act.label())),
        TargetType::Ally if !mine => Err(format!("{}只能选己方", act.label())),
        TargetType::Ally | TargetType::Enemy | TargetType::Any if down => Err("目标已经倒下".into()),
        // 复活：己方倒下的人或宠物，怪物不能复活（服务端同样校验）。
        TargetType::AllyDown if !mine || !down || unit.is_some_and(|u| u.kind == "monster") => {
            Err(format!("{}只能选倒下的己方（人或宠物）", act.label()))
        }
        _ => Ok(()),
    }
}

/// 能不能对队友（及他的宠物）下这个指令（STAGE-275）：只有道具和人物精灵的回复、复活类（目标是己方一个或
/// 倒下的己方）。攻击、捕捉、宠物技能和属性反转（`any`）都不能选队友；拿不准的一律不放开。
fn mate_support(kind: TargetType, act: Act) -> bool {
    matches!(act, Act::Item | Act::Magic) && matches!(kind, TargetType::Ally | TargetType::AllyDown)
}

/// 出战的宠物（`state` 是 `battle`）。
fn battle_pet(w: &WorldState) -> Option<&Pet> {
    w.pets.pets.iter().find(|p| p.state == "battle")
}
