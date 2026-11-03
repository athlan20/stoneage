//! 宠物动作与角色加点（STAGE-133）：动作菜单、禁用原因、改名表单、技能列表、放生确认、加点表单。
//! 不连服务端：只走选择与校验，真正发出去的请求见 `scenarios_pet_manage.rs` / `scenarios_points.rs`。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::{
    App, BagStep, CHAR_MENU, CharStep, Layer, Options, PETS_MENU, PetStep, Screen, WorldState,
};
use stoneage_client::proto::{Pet, PetSkill, Pets, Status, TargetType, WorldEntry};

fn world_app(pets: Vec<Pet>, points: u32, menu: usize) -> App {
    let mut a = App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() });
    let mut entry = WorldEntry::default();
    entry.status = Status { skill_points: points, ..Status::default() };
    let mut w = WorldState::new(entry);
    w.menu = menu;
    w.nav.push(Layer::Page(menu));
    w.pets = Pets { max_slots: 5, pets };
    a.world = Some(w);
    a.screen = Screen::World;
    a
}

fn pet(slot: u8, name: &str, state: &str, hp: u32) -> Pet {
    Pet { slot, name: name.into(), state: state.into(), hp, max_hp: 40, level: 5, ..Pet::default() }
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn text(a: &mut App, s: &str) {
    for c in s.chars() {
        key(a, KeyCode::Char(c));
    }
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().expect("已进入世界")
}

fn logged(a: &App, s: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(s))
}

// ---- 宠物动作菜单 ----

#[test]
fn enter_opens_the_pet_action_menu() {
    let mut a = world_app(vec![pet(0, "乌力", "rest", 20)], 0, PETS_MENU);
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).pet_step, Some(PetStep::Actions { slot: 0, index: 0 })));
    assert!(world(&a).nav.is(&Layer::Action));
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).pet_step.is_none(), "Esc 回宠物页");
}

#[test]
fn state_actions_stay_visible_and_explain_why() {
    // 已经在出战中：再选「出战」只解释，不发请求。
    let mut a = world_app(vec![pet(0, "乌力", "battle", 20)], 0, PETS_MENU);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter); // 第一项是「出战」
    assert!(logged(&a, "已经在出战中"));
    assert!(matches!(world(&a).pet_step, Some(PetStep::Actions { .. })), "留在动作列表");
}

#[test]
fn fainted_pet_cannot_be_sent_out() {
    let mut a = world_app(vec![pet(0, "乌力", "rest", 0)], 0, PETS_MENU);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "倒下的宠物不能出战"));
}

#[test]
fn action_rows_cover_all_eight_actions() {
    let mut a = world_app(vec![pet(0, "乌力", "rest", 20)], 0, PETS_MENU);
    key(&mut a, KeyCode::Enter);
    assert_eq!(world(&a).pet_step_rows(), 8, "出战 / 待命 / 休息 / 骑乘 / 改名 / 技能 / 对话 / 放生");
}

// ---- 骑乘（STAGE-243）：本地拦下的原因、骑乘中其它动作灰掉 ----

/// 人物的骑乘等级由 `ride_level` 给定（进入世界后的状态，菜单本地判断用）。
fn world_app_with_ride_level(pets: Vec<Pet>, ride_level: u32) -> App {
    let mut a = world_app(pets, 0, PETS_MENU);
    a.world.as_mut().expect("已进入世界").entry.status.ride_level = ride_level;
    a
}

/// 在宠物页选中栏位上的宠物，打开动作菜单，再往下移 `downs` 次。
fn open_actions(a: &mut App, downs: usize) {
    key(a, KeyCode::Enter);
    for _ in 0..downs {
        key(a, KeyCode::Down);
    }
}

/// 骑乘在第 4 项（index 3）：出战、待命、休息之后。
const RIDE_DOWNS: usize = 3;

#[test]
fn ride_is_blocked_when_another_pet_is_ridden() {
    let mut a = world_app_with_ride_level(vec![pet(0, "乌力", "ride", 20), pet(1, "贝鲁", "rest", 20)], 40);
    key(&mut a, KeyCode::Down); // 默认选中骑着的 0 号，往下选 1 号
    open_actions(&mut a, RIDE_DOWNS);
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "已经在骑 0 号栏的宠物，先下骑"), "B 的骑乘灰掉并说明先下骑 A");
    assert!(matches!(world(&a).pet_step, Some(PetStep::Actions { slot: 1, index: 3 })), "留在动作列表");
}

#[test]
fn ride_is_blocked_while_the_pet_is_out_or_standing_by() {
    for state in ["battle", "standby"] {
        let mut a = world_app_with_ride_level(vec![pet(0, "乌力", state, 20)], 40);
        open_actions(&mut a, RIDE_DOWNS);
        key(&mut a, KeyCode::Enter);
        assert!(logged(&a, "出战或待命的宠物不能骑，先设为休息"), "{state}");
    }
}

#[test]
fn ride_is_blocked_before_anyone_learns_riding() {
    let mut a = world_app_with_ride_level(vec![pet(0, "乌力", "rest", 20)], 0);
    open_actions(&mut a, RIDE_DOWNS);
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "你还没有学过骑乘，先去庄园的骑乘训练师学习"));
}

#[test]
fn ride_is_blocked_above_the_ride_level_with_the_cap_shown() {
    let mut p = pet(0, "乌力", "rest", 20);
    p.level = 41;
    let mut a = world_app_with_ride_level(vec![p], 40);
    open_actions(&mut a, RIDE_DOWNS);
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "你目前只能骑乘等级小于等于 40 级的宠物"));
}

#[test]
fn ride_is_blocked_on_low_loyalty_and_too_many_transmigrations() {
    let mut low = pet(0, "乌力", "rest", 20);
    low.loyalty = 99;
    let mut a = world_app_with_ride_level(vec![low], 40);
    open_actions(&mut a, RIDE_DOWNS);
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "该骑宠的忠诚小于 100"));

    let mut trans = pet(0, "乌力", "rest", 20);
    trans.loyalty = 100;
    trans.transmigration = 3;
    let mut a = world_app_with_ride_level(vec![trans], 40);
    open_actions(&mut a, RIDE_DOWNS);
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "转生次数超过 2 次的宠物不能骑"));
}

#[test]
fn riding_pet_greys_out_the_state_actions_and_drop() {
    let mut a = world_app_with_ride_level(vec![pet(0, "乌力", "ride", 20)], 40);
    open_actions(&mut a, 0);
    key(&mut a, KeyCode::Enter); // 出战
    assert!(logged(&a, "骑乘中的宠物不能改状态，先下骑"));
    assert!(matches!(world(&a).pet_step, Some(PetStep::Actions { .. })), "留在列表");

    // 放生（第 8 项）灰掉并说明，不进确认。
    for _ in 0..7 {
        key(&mut a, KeyCode::Down);
    }
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "骑乘中的宠物不能放生，先下骑"));
    assert!(world(&a).pet_step.is_some(), "没有进入放生确认");
}

#[test]
fn riding_pet_keeps_rename_and_skills_usable() {
    let mut a = world_app_with_ride_level(vec![pet(0, "乌力", "ride", 20)], 40);
    open_actions(&mut a, 4); // 改名
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).pet_step, Some(PetStep::Rename { .. })), "改名可用");
    key(&mut a, KeyCode::Esc); // 退回列表，光标回到第 1 项
    for _ in 0..5 {
        key(&mut a, KeyCode::Down);
    }
    key(&mut a, KeyCode::Enter); // 技能
    assert!(matches!(world(&a).pet_step, Some(PetStep::Skills { .. })), "技能可用");
}

// ---- 改名表单 ----

#[test]
fn rename_form_prefills_the_current_name_and_esc_keeps_it() {
    let mut a = world_app(vec![pet(0, "乌力", "rest", 20)], 0, PETS_MENU);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down); // 到「改名」
    key(&mut a, KeyCode::Enter);
    let Some(PetStep::Rename { text, .. }) = world(&a).pet_step.as_ref() else {
        panic!("应当进入改名表单");
    };
    assert_eq!(text.text(), "乌力", "表单里先填上现在的名字");
    key(&mut a, KeyCode::Esc);
    assert!(matches!(world(&a).pet_step, Some(PetStep::Actions { .. })), "退回动作列表，没改名");
}

#[test]
fn rename_form_refuses_whitespace_and_keeps_the_input() {
    let mut a = world_app(vec![pet(0, "乌力", "rest", 20)], 0, PETS_MENU);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down); // 到「改名」
    key(&mut a, KeyCode::Enter);
    // 清掉预填的名字，改成一个带空格的名字。
    for _ in 0..2 {
        key(&mut a, KeyCode::Backspace);
    }
    text(&mut a, "小 白");
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "宠物名字中间不能有空白"));
    assert!(matches!(world(&a).pet_step, Some(PetStep::Rename { .. })), "错误留在表单里");
}

// ---- 技能 ----

#[test]
fn skills_step_lists_the_pet_skills() {
    let mut skills = vec![
        PetSkill { slot: 1, name: "攻击".into(), target_type: TargetType::Enemy, ..Default::default() },
        PetSkill { slot: 2, name: "防御".into(), target_type: TargetType::None, ..Default::default() },
    ];
    let mut p = pet(0, "乌力", "rest", 20);
    p.skills = std::mem::take(&mut skills);
    let mut a = world_app(vec![p], 0, PETS_MENU);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down); // 到「技能」（STAGE-243 起在骑乘之后）
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).pet_step, Some(PetStep::Skills { slot: 0, index: 0 })));
    assert_eq!(world(&a).pet_step_rows(), 2);
}

// ---- 放生 ----

#[test]
fn drop_asks_for_confirmation_with_the_real_name() {
    let mut a = world_app(vec![pet(2, "贝鲁卡", "rest", 20)], 0, PETS_MENU);
    key(&mut a, KeyCode::Enter);
    for _ in 0..7 {
        key(&mut a, KeyCode::Down);
    }
    key(&mut a, KeyCode::Enter); // 放生（STAGE-239 起排在对话之后，STAGE-243 起是第 8 项）
    assert!(logged(&a, "放生后不能恢复：贝鲁卡（2 号）"), "要写真实的名字和栏位");
    // STAGE-256：确认期间动作面板留着，光标停在「放生」，确认走全局的消息栏流程。
    assert!(matches!(world(&a).pet_step, Some(PetStep::Actions { index: 7, .. })), "面板留着，光标在放生");
    assert!(world(&a).nav.is(&Layer::Action));
}

/// 滚到「放生」（第 8 项）并回车，弹出放生确认。
fn ask_drop(a: &mut App) {
    key(a, KeyCode::Enter);
    for _ in 0..7 {
        key(a, KeyCode::Down);
    }
    key(a, KeyCode::Enter);
}

#[test]
fn drop_confirm_esc_keeps_the_action_panel_on_drop_row() {
    // STAGE-256 D2：Esc 只取消确认，动作面板仍开着，光标还在「放生」。
    let mut a = world_app(vec![pet(2, "贝鲁卡", "rest", 20)], 0, PETS_MENU);
    ask_drop(&mut a);
    assert!(world(&a).confirm.is_some(), "放生在等确认");
    key(&mut a, KeyCode::Esc);
    assert!(logged(&a, "已取消放生"), "Esc 取消确认，消息照旧");
    assert!(world(&a).confirm.is_none(), "确认已经取消");
    assert!(matches!(world(&a).pet_step, Some(PetStep::Actions { index: 7, .. })), "面板仍开，光标在放生");
    assert!(world(&a).nav.is(&Layer::Action), "动作层还在栈顶");
    assert_eq!(world(&a).pets.pets.len(), 1, "宠物还在");
    // 再按一次 Esc 才退出动作面板。
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).pet_step.is_none(), "第二次 Esc 关掉动作面板");
    assert!(world(&a).nav.is(&Layer::Page(PETS_MENU)));
}

#[test]
fn drop_confirm_yes_closes_the_action_panel() {
    // 确认选「确认」后动作面板收掉，行为和以前一样。
    let mut a = world_app(vec![pet(2, "贝鲁卡", "rest", 20)], 0, PETS_MENU);
    ask_drop(&mut a);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Enter);
    assert!(world(&a).pet_step.is_none(), "答 y 后动作面板收掉");
    assert!(!world(&a).nav.contains(&Layer::Action), "动作层也去掉");
}

// ---- 角色加点 ----

fn char_app(points: u32) -> App {
    world_app(vec![], points, CHAR_MENU)
}

#[test]
fn addpoint_form_checks_the_range() {
    let mut a = char_app(3);
    // 精灵栏是空的，焦点自动落在能力栏（不用 Tab）。
    key(&mut a, KeyCode::Enter);
    assert!(matches!(world(&a).char_step, Some(CharStep::AddPoint { stat: 0, .. })));

    key(&mut a, KeyCode::Char('0'));
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "要填 1–3 之间的整数"), "0 不合法");
    assert!(world(&a).char_step.is_some(), "错误留在表单");

    key(&mut a, KeyCode::Char('9'));
    key(&mut a, KeyCode::Enter);
    assert!(logged(&a, "要填 1–3 之间的整数"), "超过未分配点数也不合法");
}

#[test]
fn addpoint_esc_cancels_without_a_request() {
    let mut a = char_app(3);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Char('2'));
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).char_step.is_none(), "Esc 取消加点");
    assert!(world(&a).nav.is(&Layer::Page(CHAR_MENU)), "导航栈回到角色页");
    assert!(!logged(&a, "已加点"), "取消不发请求");
}

#[test]
fn bag_and_pet_steps_do_not_get_mixed_up() {
    // 两步状态各自独立：宠物动作开着时不会误判成背包动作。
    let mut a = world_app(vec![pet(0, "乌力", "rest", 20)], 0, PETS_MENU);
    key(&mut a, KeyCode::Enter);
    assert!(world(&a).bag_step.is_none());
    assert!(matches!(world(&a).pet_step, Some(PetStep::Actions { .. })));
    let _: Option<&BagStep> = world(&a).bag_step.as_ref();
}
