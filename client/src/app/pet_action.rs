//! 宠物的对象动作（STAGE-133，`docs/cli-ui-redesign.md`「对象、动作、参数」）：
//! 宠物页选中一只后 `Enter` 打开动作菜单（出战、待命、休息、改名、技能、放生）。
//! 状态类动作直接提交；改名走专用表单（16 字上限，与服务端一致）；技能是只读详情；
//! 放生进入默认取消的确认。请求调用 `pets_manage.rs` 里的同一批函数，与 `pet` 命令共用校验。
//!
//! 和背包一样，一次流程只在导航栈压**一层** `Layer::Action`，子步骤靠 `PetStep` 前后走。

use crossterm::event::{KeyCode, KeyEvent};

use crate::proto::{Pet, PetStatus};

use super::input::TextInput;
use super::pet_merge::MergeReport;
use super::pet_ride::{RideContext, is_riding, ride_reason};
use super::pets_manage::MAX_PET_NAME_CHARS;
use super::world::WorldState;
use super::{App, Layer};

/// 宠物动作流程当前停在哪一步。
#[derive(Debug)]
pub enum PetStep {
    /// 动作列表：`slot` 是宠物栏位，`index` 是选中的动作。
    Actions { slot: u8, index: usize },
    /// 改名表单。
    Rename { slot: u8, text: TextInput },
    /// 技能详情（只读）；加工、料理按 `Enter` 进入选材料（STAGE-208）。
    Skills { slot: u8, index: usize },
    /// 合成选材料（STAGE-208）：`skill` 是宠物页的技能栏位，`cursor` 是材料列表里的光标，
    /// `picked` 是已勾选的背包格号（按勾选顺序，第一件决定食物还是道具），`sending` 表示请求已发出、等响应。
    Merge { slot: u8, skill: u8, cursor: usize, picked: Vec<u8>, sending: bool },
    /// 合成结果页（STAGE-208）。
    MergeResult { slot: u8, skill: u8, report: MergeReport },
}

/// 这一步是哪一类。
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum PetStepKind {
    Actions,
    Rename,
    Skills,
    Merge,
    MergeResult,
}

impl PetStep {
    pub fn kind(&self) -> PetStepKind {
        match self {
            PetStep::Actions { .. } => PetStepKind::Actions,
            PetStep::Rename { .. } => PetStepKind::Rename,
            PetStep::Skills { .. } => PetStepKind::Skills,
            PetStep::Merge { .. } => PetStepKind::Merge,
            PetStep::MergeResult { .. } => PetStepKind::MergeResult,
        }
    }

    pub fn slot(&self) -> u8 {
        match self {
            PetStep::Actions { slot, .. }
            | PetStep::Rename { slot, .. }
            | PetStep::Skills { slot, .. }
            | PetStep::Merge { slot, .. }
            | PetStep::MergeResult { slot, .. } => *slot,
        }
    }

    /// 这一步的标题（渲染用）。
    pub fn title(&self, name: &str) -> String {
        match self {
            PetStep::Actions { .. } => format!("动作 · {name}"),
            PetStep::Rename { .. } => format!("给 {name} 改名"),
            PetStep::Skills { .. } => format!("{name} 的技能"),
            PetStep::Merge { .. } | PetStep::MergeResult { .. } => format!("合成 · {name}"),
        }
    }
}

/// 宠物能做的动作。
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum PetAct {
    Battle,
    Standby,
    Rest,
    /// 骑乘（STAGE-243）：没骑着就骑上，骑着这只时标签是「下骑」，见 `label_for`。
    Ride,
    Rename,
    Skills,
    /// 和宠物对话（STAGE-239，协议 5.61）。
    Talk,
    Drop,
}

impl PetAct {
    pub fn label(self) -> &'static str {
        match self {
            PetAct::Battle => "出战",
            PetAct::Standby => "待命",
            PetAct::Rest => "休息",
            PetAct::Ride => "骑乘",
            PetAct::Rename => "改名",
            PetAct::Skills => "技能",
            PetAct::Talk => "对话",
            PetAct::Drop => "放生",
        }
    }

    /// 菜单上显示的文字：骑乘项随这只宠物的状态变，骑着它时写「下骑」（STAGE-243）。
    pub fn label_for(self, pet: Option<&Pet>) -> &'static str {
        match (self, pet) {
            (PetAct::Ride, Some(p)) if is_riding(p) => "下骑",
            _ => self.label(),
        }
    }
}

/// 宠物菜单里的全部动作（顺序就是显示顺序）。
pub const PET_ACTS: [PetAct; 8] = [
    PetAct::Battle,
    PetAct::Standby,
    PetAct::Rest,
    PetAct::Ride,
    PetAct::Rename,
    PetAct::Skills,
    PetAct::Talk,
    PetAct::Drop,
];

/// 动作不能做的原因（保留显示并解释，STAGE-133 验收标准 2）。`ride` 是骑乘判断用的人物与栏位信息（STAGE-243）。
pub fn disabled_reason(act: PetAct, pet: &Pet, ride: RideContext) -> Option<String> {
    // 骑乘中的宠物不能改状态、不能放生（服务端回 PET_RIDING，STAGE-243）。
    if is_riding(pet) {
        match act {
            PetAct::Battle | PetAct::Standby | PetAct::Rest => return Some("骑乘中的宠物不能改状态，先下骑".to_string()),
            PetAct::Drop => return Some("骑乘中的宠物不能放生，先下骑".to_string()),
            _ => {}
        }
    }
    let reason = match act {
        PetAct::Battle => match pet.status() {
            PetStatus::Battle => "已经在出战中",
            PetStatus::Fainted => "倒下的宠物不能出战",
            _ => return None,
        },
        PetAct::Standby => return (pet.status() == PetStatus::Standby).then(|| "已经在待命".to_string()),
        PetAct::Rest => return (pet.status() == PetStatus::Rest).then(|| "已经在休息".to_string()),
        PetAct::Ride => return ride_reason(pet, ride),
        // 原版只有在地图上（出战、待命）的宠物会被面向说话（STAGE-239）。
        PetAct::Talk => match pet.status() {
            PetStatus::Rest => "休息中的宠物不在身边，先设为待命或出战",
            PetStatus::Ride => "骑乘中的宠物不能对话",
            _ => return None,
        },
        _ => return None,
    };
    Some(reason.to_string())
}

impl WorldState {
    /// 宠物动作流程的当前一步。
    pub fn pet_at(&self, slot: u8) -> Option<Pet> {
        self.pets.pets.iter().find(|p| p.slot == slot).cloned()
    }

    /// 当前步骤要选中项的项数（改名表单没有列表，返回 0）。
    pub fn pet_step_rows(&self) -> usize {
        match self.pet_step.as_ref() {
            Some(PetStep::Actions { .. }) => PET_ACTS.len(),
            Some(PetStep::Skills { slot, .. }) => self.pet_at(*slot).map_or(0, |p| p.skills.len()),
            Some(PetStep::Merge { .. }) => self.merge_materials().len(),
            Some(PetStep::Rename { .. }) | Some(PetStep::MergeResult { .. }) | None => 0,
        }
    }
}

impl App {
    /// 宠物栏位上的宠物（动作流程里反复用到）。
    fn pet_at(&self, slot: u8) -> Option<Pet> {
        self.world.as_ref()?.pet_at(slot)
    }

    /// 宠物页 `Enter`：打开选中宠物的动作菜单。没有选中宠物时返回 false。
    pub(super) fn pet_open_actions(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        let Some(slot) = world.selected_pet_slot() else {
            return false;
        };
        let slot = slot as u8;
        if world.pet_at(slot).is_none() {
            return false;
        }
        if let Some(world) = self.world.as_mut() {
            world.pet_revision=world.pets_epoch;
            world.pet_step = Some(PetStep::Actions { slot, index: 0 });
            world.nav.push(Layer::Action);
        }
        true
    }

    /// 宠物动作流程里的按键。返回 true 表示已处理。
    pub(super) fn pet_action_key(&mut self, key: KeyEvent) -> bool {
        if self.world.as_ref().is_some_and(|w|w.pet_step.is_some()&&w.pet_revision!=w.pets_epoch){self.close_pet_step();self.log.error("宠物资料已变化，请重新选择");return true;}
        let Some(kind) = self.world.as_ref().and_then(|w| w.pet_step.as_ref()).map(PetStep::kind) else {
            return false;
        };
        // 改名表单里的 `/` 是文字；动作和技能列表里 `/` 照常打开命令栏。
        if key.code == KeyCode::Char('/') && kind != PetStepKind::Rename {
            if let Some(world) = self.world.as_mut() {
                world.nav.push(Layer::Command);
            }
            return true;
        }
        match kind {
            PetStepKind::Actions => self.pet_actions_key(key),
            PetStepKind::Rename => self.pet_rename_key(key),
            PetStepKind::Skills => self.pet_skills_key(key),
            PetStepKind::Merge => self.pet_merge_key(key),
            PetStepKind::MergeResult => self.pet_merge_result_key(key),
        }
    }

    fn pet_actions_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => self.pet_step_back(),
            KeyCode::Up | KeyCode::Down => {
                self.pet_move_index(key.code == KeyCode::Down);
                true
            }
            KeyCode::Enter => self.pet_run_selected(),
            _ => true,
        }
    }

    fn pet_rename_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => self.pet_step_back(),
            KeyCode::Enter => self.pet_rename_submit(),
            _ => {
                if let Some(world) = self.world.as_mut() {
                    if let Some(PetStep::Rename { text, .. }) = world.pet_step.as_mut() {
                        text.handle_key(&key);
                    }
                }
                true
            }
        }
    }

    fn pet_skills_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => self.pet_step_back(),
            KeyCode::Up | KeyCode::Down => {
                self.pet_move_index(key.code == KeyCode::Down);
                true
            }
            KeyCode::Enter => self.pet_merge_open(),
            _ => true,
        }
    }

    fn pet_move_index(&mut self, down: bool) {
        let rows = self.world.as_ref().map_or(0, WorldState::pet_step_rows);
        if rows == 0 {
            return;
        }
        let Some(world) = self.world.as_mut() else {
            return;
        };
        let index = match world.pet_step.as_ref() {
            Some(PetStep::Actions { index, .. }) | Some(PetStep::Skills { index, .. }) => *index,
            _ => return,
        };
        let next = if down { (index + 1).min(rows - 1) } else { index.saturating_sub(1) };
        match world.pet_step.as_mut() {
            Some(PetStep::Actions { index, .. }) | Some(PetStep::Skills { index, .. }) => *index = next,
            _ => {}
        }
    }

    /// `Esc`：退一步。退到底返回 false（调用方把导航栈上那一层弹掉）。
    pub(super) fn pet_step_back(&mut self) -> bool {
        let Some(world) = self.world.as_mut() else {
            return false;
        };
        match world.pet_step.take() {
            Some(PetStep::Actions { .. }) | None => false,
            Some(PetStep::Rename { slot, .. }) | Some(PetStep::Skills { slot, .. }) => {
                world.pet_step = Some(PetStep::Actions { slot, index: 0 });
                true
            }
            // 选材料和结果页都退回技能列表，光标停在刚才那一项（STAGE-208）。
            Some(PetStep::Merge { slot, skill, .. }) | Some(PetStep::MergeResult { slot, skill, .. }) => {
                let index = world.pet_at(slot).and_then(|p| p.skills.iter().position(|s| s.slot == skill)).unwrap_or(0);
                world.pet_step = Some(PetStep::Skills { slot, index });
                true
            }
        }
    }

    /// 关掉宠物动作流程（动作完成或宠物消失），并弹掉导航栈上那一层。
    pub(super) fn close_pet_step(&mut self) {
        if let Some(world) = self.world.as_mut() {
            world.pet_step = None;
            if matches!(world.nav.top(), Layer::Action) {
                world.nav.pop();
            }
        }
    }

    /// 执行动作列表里选中的那一项。
    fn pet_run_selected(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return true;
        };
        let Some(PetStep::Actions { slot, index }) = world.pet_step.as_ref() else {
            return true;
        };
        let (slot, index) = (*slot, *index);
        // 宠物消失（被放生、被换走）时动作作废，退回宠物页。
        let Some(pet) = self.pet_at(slot) else {
            self.close_pet_step();
            return true;
        };
        let Some(&act) = PET_ACTS.get(index) else {
            return true;
        };
        if act!=PetAct::Skills && self.observer_blocks(){return true;}
        let ride = self.world.as_ref().map(WorldState::ride_context).unwrap_or_default();
        if let Some(reason) = disabled_reason(act, &pet, ride) {
            self.log.system(reason);
            return true;
        }
        match act {
            PetAct::Battle => {
                self.set_pet_state_slot(slot, "battle", "出战");
                self.close_pet_step();
            }
            PetAct::Standby => {
                self.set_pet_state_slot(slot, "standby", "待命");
                self.close_pet_step();
            }
            PetAct::Rest => {
                self.set_pet_state_slot(slot, "rest", "休息");
                self.close_pet_step();
            }
            PetAct::Ride => {
                // 骑着这只就下骑，否则骑上（本地校验已在上面通过）；和 `pet ride` 命令共用请求函数。
                if is_riding(&pet) {
                    self.unride_pet();
                } else {
                    self.ride_pet_slot(slot);
                }
                self.close_pet_step();
            }
            PetAct::Rename => {
                if let Some(world) = self.world.as_mut() {
                    // 表单里先填上现在的名字，改起来直观（清空即恢复种族名）。
                    let text = TextInput::with_text(MAX_PET_NAME_CHARS, &pet.name);
                    world.pet_step = Some(PetStep::Rename { slot, text });
                }
            }
            PetAct::Skills => {
                if let Some(world) = self.world.as_mut() {
                    world.pet_step = Some(PetStep::Skills { slot, index: 0 });
                }
            }
            PetAct::Talk => {
                self.talk_pet_slot(slot);
                self.close_pet_step();
            }
            PetAct::Drop => {
                // STAGE-256：确认期间动作面板留着，光标停在「放生」；Esc 取消后还在这里，答 y 时才收掉（confirm.rs）。
                // 没问成（不能放生、栏位没有宠物）照常收起，原因已写进消息栏。
                self.ask_drop_pet(slot);
                if !self.drop_confirm_pending() {
                    self.close_pet_step();
                }
            }
        }
        true
    }

    /// 提交改名表单：留空表示恢复种族名；有空白字符时留在表单里解释。
    fn pet_rename_submit(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return true;
        };
        let Some(PetStep::Rename { slot, text }) = world.pet_step.as_ref() else {
            return true;
        };
        let (slot, raw) = (*slot, text.text().to_string());
        if self.pet_at(slot).is_none() {
            self.close_pet_step();
            return true;
        }
        if raw.chars().any(char::is_whitespace) {
            self.log.system("宠物名字中间不能有空白");
            return true;
        }
        self.rename_pet_slot(slot, raw);
        self.close_pet_step();
        true
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{PetSkill, WorldEntry};

    fn pet(state: &str, hp: u32) -> Pet {
        Pet { slot: 0, name: "乌力".into(), state: state.into(), hp, max_hp: 40, ..Pet::default() }
    }

    /// 没有骑乘相关的前提（没骑别的、骑乘等级 0 之外的判断不用）。
    fn no_ride() -> RideContext {
        RideContext { riding: None, ride_level: 40 }
    }

    #[test]
    fn battle_pet_cannot_be_sent_out_again() {
        assert_eq!(disabled_reason(PetAct::Battle, &pet("battle", 20), no_ride()).as_deref(), Some("已经在出战中"));
        assert_eq!(disabled_reason(PetAct::Battle, &pet("rest", 20), no_ride()), None);
    }

    #[test]
    fn fainted_pet_cannot_be_sent_out() {
        assert_eq!(disabled_reason(PetAct::Battle, &pet("rest", 0), no_ride()).as_deref(), Some("倒下的宠物不能出战"));
    }

    #[test]
    fn state_actions_are_disabled_when_already_in_them() {
        assert_eq!(disabled_reason(PetAct::Standby, &pet("standby", 20), no_ride()).as_deref(), Some("已经在待命"));
        assert_eq!(disabled_reason(PetAct::Rest, &pet("rest", 20), no_ride()).as_deref(), Some("已经在休息"));
        assert_eq!(disabled_reason(PetAct::Rename, &pet("rest", 20), no_ride()), None);
        assert_eq!(disabled_reason(PetAct::Drop, &pet("rest", 20), no_ride()), None);
    }

    #[test]
    fn ride_label_follows_the_pet_state() {
        assert_eq!(PetAct::Ride.label_for(Some(&pet("rest", 20))), "骑乘");
        assert_eq!(PetAct::Ride.label_for(Some(&pet("ride", 20))), "下骑");
        assert_eq!(PetAct::Ride.label_for(None), "骑乘");
        assert_eq!(PetAct::Talk.label_for(Some(&pet("ride", 20))), "对话", "只有骑乘项换标签");
    }

    #[test]
    fn riding_pet_greys_out_the_state_actions_and_drop() {
        let riding = pet("ride", 20);
        let ctx = RideContext { riding: Some(0), ride_level: 40 };
        for act in [PetAct::Battle, PetAct::Standby, PetAct::Rest] {
            assert_eq!(disabled_reason(act, &riding, ctx).as_deref(), Some("骑乘中的宠物不能改状态，先下骑"), "{act:?}");
        }
        assert_eq!(disabled_reason(PetAct::Drop, &riding, ctx).as_deref(), Some("骑乘中的宠物不能放生，先下骑"));
    }

    #[test]
    fn riding_pet_keeps_rename_skills_and_unride_enabled() {
        let riding = pet("ride", 20);
        let ctx = RideContext { riding: Some(0), ride_level: 40 };
        assert_eq!(disabled_reason(PetAct::Rename, &riding, ctx), None);
        assert_eq!(disabled_reason(PetAct::Skills, &riding, ctx), None);
        assert_eq!(disabled_reason(PetAct::Ride, &riding, ctx), None, "骑着这只时「下骑」可选");
    }

    #[test]
    fn skill_rows_follow_the_pet() {
        let mut w = WorldState::new(WorldEntry::default());
        w.pets.pets = vec![Pet {
            slot: 1,
            skills: vec![PetSkill::default(), PetSkill::default()],
            ..Default::default()
        }];
        w.pet_step = Some(PetStep::Skills { slot: 1, index: 0 });
        assert_eq!(w.pet_step_rows(), 2);
    }
}
