//! 战斗按键的选择流程（STAGE-38，`docs/battle-ui.md`「三种操作方式」）：焦点、列表、Esc、
//! 文字命令的本地检查，以及按目标类型决定只能选哪一方（STAGE-78）。不连服务端，所以只走到“发出之前”；
//! 真正发出并结算的流程见 `scenarios_battle.rs`。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::battle::slots::Cell;
use stoneage_client::app::battle::{Act, BattleState, Phase, Pick};
use stoneage_client::app::{App, Kind, Options, Screen, WorldState};
use stoneage_client::proto::{
    BattleAllow, BattleStart, BattleTurn, BattleUnit, Inventory, InventorySlot, MagicView, Progress, TargetType,
    WorldEntry,
};

fn app() -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() })
}

fn unit(no: u8, kind: &str, name: &str, pct: u32) -> BattleUnit {
    BattleUnit { no, kind: kind.into(), name: name.into(), level: 3, hp_pct: pct, ..BattleUnit::default() }
}

/// 一场战斗：我方勇者和乌力，敌方两只野猪（16 号在敌方第 2 位，15 号在第 3 位）。
/// 背包里有一瓶回复药（5 号栏位，要选己方一个），两种逃跑、捕捉都允许，没有可以换上场的宠物。
fn battle_app(phase_needs: &[&str]) -> App {
    let units = vec![
        unit(0, "player", "勇者小明", 100),
        unit(5, "pet", "乌力", 100),
        unit(15, "monster", "野猪", 100),
        unit(16, "monster", "野猪", 100),
    ];
    battle_app_with(phase_needs, units)
}

fn battle_app_with(phase_needs: &[&str], units: Vec<BattleUnit>) -> App {
    let mut a = app();
    let start = BattleStart {
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow { escape: true, capture: true },
        units,
        ..BattleStart::default()
    };
    let mut battle = BattleState::from_start(start);
    battle.on_turn(
        BattleTurn {
                field: None,
            battle_id: "b_1".into(),
            turn: 1,
            deadline_ms: 1_030_000,
            server_time_ms: 1_000_000,
            needs: phase_needs.iter().map(|s| s.to_string()).collect(),
            optional: vec![],
            progress: Progress { done: 0, total: 1 },
        },
        std::time::Instant::now(),
    );
    let mut world = WorldState::new(WorldEntry::default());
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.inventory = Inventory {
        gold: 0,
        slots: vec![InventorySlot {
            slot: 5,
            item_id: 1,
            name: "回复药".into(),
            count: 3,
            target_type: TargetType::Ally,
            ..InventorySlot::default()
        }],
    };
    world.battle = Some(battle);
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn text(a: &mut App, s: &str) {
    if !a.world.as_ref().unwrap().nav.is(&stoneage_client::app::Layer::Command){key(a,KeyCode::Char('/'));}
    for c in s.chars() {
        key(a, KeyCode::Char(c));
    }
}

fn battle(a: &App) -> &BattleState {
    a.world.as_ref().unwrap().battle.as_ref().unwrap()
}

fn last_error(a: &App) -> String {
    a.log.all().iter().rev().find(|m| m.kind == Kind::Error).map(|m| m.text.clone()).unwrap_or_default()
}

/// STAGE-107：道具不直选编号，数字键在道具列表里不起作用；↑↓ 移动高亮，Enter 选中。
#[test]
fn items_are_picked_with_arrows_and_enter_not_digits() {
    let mut a = battle_app(&["player"]);
    key(&mut a, KeyCode::Char('3'));
    assert_eq!(battle(&a).pick, Pick::Item, "3 是道具");
    key(&mut a, KeyCode::Char('1'));
    assert_eq!(battle(&a).pick, Pick::Item, "道具列表里数字键不直选");
    assert!(last_error(&a).contains("↑↓ 选"), "{}", last_error(&a));
    key(&mut a, KeyCode::Enter);
    assert!(
        matches!(battle(&a).pick, Pick::Target { act: Act::Item, extra: Some(5), kind: TargetType::Ally }),
        "Enter 选中回复药（5 号栏位），要选己方一个"
    );
    assert_eq!(battle(&a).cursor.side, 0, "回复药默认对己方（自己）");
    key(&mut a, KeyCode::Esc);
    assert_eq!(battle(&a).pick, Pick::Item, "Esc 退回道具列表");
    key(&mut a, KeyCode::Esc);
    assert_eq!(battle(&a).pick, Pick::Action, "再按 Esc 回到指令条");
}

/// STAGE-78：能选的一方由目标类型决定，Tab 不再切换——回复药只能选己方，攻击只能选敌方。
#[test]
fn each_target_only_offers_the_side_it_can_hit() {
    let mut a = battle_app(&["player"]);
    key(&mut a, KeyCode::Char('3'));
    key(&mut a, KeyCode::Enter);
    assert_eq!(battle(&a).cursor.side, 0, "回复药的焦点在己方");
    // STAGE-153 起 Tab 是战斗控制面板，不再是“切换一方”：打开、关掉都不改选目标的一方。
    key(&mut a, KeyCode::Tab);
    assert!(a.world.as_ref().is_some_and(|w| w.system.battle_open), "Tab 打开战斗控制面板");
    key(&mut a, KeyCode::Esc);
    assert!(!a.world.as_ref().is_some_and(|w| w.system.battle_open), "Esc 关面板");
    assert_eq!(battle(&a).cursor.side, 0, "面板开关不切到敌方");
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Char('1'));
    assert!(matches!(battle(&a).pick, Pick::Target { act: Act::Attack, .. }));
    let side = battle(&a).cursor.side;
    assert_eq!(side, 1, "攻击的焦点在敌方");
    key(&mut a, KeyCode::Tab);
    key(&mut a, KeyCode::Esc);
    assert_eq!(battle(&a).cursor.side, side, "攻击只能选敌方，面板也不切换");
}

#[test]
fn arrows_move_between_enabled_actions_and_across_the_target_grid() {
    let mut a = battle_app(&["player"]);
    // 人物指令：精灵不能用、没有可换的宠物，焦点按右移时跳过它们。
    key(&mut a, KeyCode::Right);
    assert_eq!(battle(&a).action_focus, 1, "不可用动作仍可聚焦查看原因");
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    assert_eq!(battle(&a).action_focus, 3, "第一行最后一格是防御");
    key(&mut a, KeyCode::Right);
    assert_eq!(battle(&a).action_focus, 3, "行末停住，不横移到第二行");
    // STAGE-153：上下按画面跨行，防御下面同一列是第二行最后一格的挂机管理项。
    key(&mut a, KeyCode::Down);
    assert_eq!(battle(&a).action_focus, 7, "防御 ↓ 自动挂机");
    key(&mut a, KeyCode::Up);
    assert_eq!(battle(&a).action_focus, 3, "↑ 回防御");
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Up);
    // 选攻击，焦点落在第一个活着的敌人（16 号，敌方第 2 位）；下移一格到 15 号（第 3 位）。
    key(&mut a, KeyCode::Left);
    key(&mut a, KeyCode::Left);
    key(&mut a, KeyCode::Left);
    key(&mut a, KeyCode::Enter);
    assert!(matches!(battle(&a).pick, Pick::Target { act: Act::Attack, .. }));
    let before = battle(&a).cursor;
    key(&mut a, KeyCode::Down);
    assert_eq!(battle(&a).cursor.col, before.col + 1, "下移一个站位");
    // 敌方后列没有单位，左移不动（STAGE-108：那一排没有能选的就不动）。
    key(&mut a, KeyCode::Left);
    assert!(battle(&a).cursor.front, "敌方后列是空的，左移不动");
    key(&mut a, KeyCode::Right);
    assert!(battle(&a).cursor.front, "右边已经是前列，不动");
}

/// STAGE-108：方向键只落在能选的单位上。敌方前排第 1、4 位是空的，第 2 位（16 号）已经倒下，
/// 后排第 5 位（14 号）还活着。
#[test]
fn arrows_skip_empty_cells_and_fainted_units() {
    let units = vec![
        unit(0, "player", "勇者小明", 100),
        unit(14, "monster", "野猪", 100),
        unit(15, "monster", "野猪", 100),
        unit(16, "monster", "野猪", 0),
        unit(19, "monster", "野猪", 100),
    ];
    let mut a = battle_app_with(&["player"], units);
    key(&mut a, KeyCode::Char('1'));
    assert_eq!(battle(&a).cursor, Cell { side: 1, front: true, col: 2 }, "默认落在第一个活着的敌人（15 号）");
    key(&mut a, KeyCode::Down);
    assert_eq!(battle(&a).cursor, Cell { side: 1, front: true, col: 4 }, "下移跳过第 4 位的空位，到 19 号");
    key(&mut a, KeyCode::Down);
    assert_eq!(battle(&a).cursor.col, 4, "下边再没有能选的，停在原地");
    key(&mut a, KeyCode::Up);
    assert_eq!(battle(&a).cursor, Cell { side: 1, front: true, col: 2 }, "上移跳过空位，回到 15 号");
    key(&mut a, KeyCode::Up);
    assert_eq!(battle(&a).cursor, Cell { side: 1, front: true, col: 2 }, "上边第 2 位是倒下的，第 1 位是空的，停在原地");
    key(&mut a, KeyCode::Left);
    assert_eq!(battle(&a).cursor, Cell { side: 1, front: false, col: 4 }, "左到后列，落在离当前列最近的能选单位（14 号）上");
    key(&mut a, KeyCode::Right);
    assert_eq!(battle(&a).cursor, Cell { side: 1, front: true, col: 4 }, "右回前列，离第 5 列最近的是 19 号");
}

#[test]
fn enter_on_an_empty_cell_explains_and_keeps_the_choice() {
    let mut a = battle_app(&["player"]);
    key(&mut a, KeyCode::Char('1'));
    // 方向键不会停在空位上（STAGE-108），这里直接把焦点放到敌方前排第 1 位（不存在的 18 号）上，
    // 模拟选目标的途中单位离开战场：不能攻击，留在选目标里。
    let empty = Cell { side: 1, front: true, col: 0 };
    a.world.as_mut().unwrap().battle.as_mut().unwrap().cursor = empty;
    let pick = battle(&a).pick;
    key(&mut a, KeyCode::Enter);
    assert!(last_error(&a).contains("那个位置没有单位"), "{}", last_error(&a));
    assert_eq!(battle(&a).pick, pick, "选择不变，可以重新选");
}

#[test]
fn typed_commands_are_checked_before_they_are_sent() {
    let mut a = battle_app(&["player"]);
    // 我方宠物在我方 3 号（协议 5 号），攻击不能选己方。
    text(&mut a, "a m3");
    key(&mut a, KeyCode::Enter);
    assert!(last_error(&a).contains("攻击只能选敌方"), "{}", last_error(&a));
    text(&mut a, "item 4");
    key(&mut a, KeyCode::Enter);
    assert!(last_error(&a).contains("道具槽位要在 5–19"), "{}", last_error(&a));
    text(&mut a, "skill 1");
    key(&mut a, KeyCode::Enter);
    assert!(last_error(&a).contains("人物指令里没有技能"), "{}", last_error(&a));
    assert!(a.world.as_ref().unwrap().input.is_empty(), "命令出错后输入行清空");
}

#[test]
fn digits_typed_into_the_input_line_are_plain_text() {
    let mut a = battle_app(&["player"]);
    text(&mut a, "a");
    key(&mut a, KeyCode::Char('1'));
    assert_eq!(battle(&a).pick, Pick::Action, "输入行有内容时数字不选指令");
    assert_eq!(a.world.as_ref().unwrap().input.text(), "a1");
    assert_eq!(a.world.as_ref().unwrap().menu, 0, "也不切换菜单");
}

#[test]
fn magic_is_greyed_out_without_learned_magic() {
    // 没有学会精灵时，精灵动作仍是灰色（STAGE-25）。
    let mut a = battle_app(&["player"]);
    key(&mut a, KeyCode::Char('2'));
    assert!(last_error(&a).contains("还没有学会精灵"), "{}", last_error(&a));
    assert_eq!(battle(&a).pick, Pick::Action);
}

/// STAGE-25：数字键按精灵编号（0–4）选精灵，再选目标；Esc 逐级退回；空栏位和越界编号留在选择里。
/// STAGE-78：攻击系的精灵只能选敌方（Tab 不切换），治愈类的精灵不用选目标。
#[test]
fn digits_pick_a_magic_then_a_target_and_esc_steps_back() {
    let mut a = battle_app(&["player"]);
    a.world.as_mut().unwrap().entry.status.mp = 50;
    a.world.as_mut().unwrap().entry.status.magic = vec![
        MagicView {
            slot: 0,
            id: 301,
            name: "岩石撞击".into(),
            mp: 10,
            target_type: TargetType::Enemy,
            ..Default::default()
        },
        MagicView {
            slot: 1,
            id: 0,
            name: "治愈的精灵 Lv1".into(),
            mp: 6,
            target_type: TargetType::SelfOnly,
            ..Default::default()
        },
    ];
    key(&mut a, KeyCode::Char('2'));
    assert_eq!(battle(&a).pick, Pick::Magic, "2 是精灵");
    // 3 号栏位没有精灵：说明原因，留在精灵列表里。
    key(&mut a, KeyCode::Char('3'));
    assert!(last_error(&a).contains("3 号栏位没有精灵"), "{}", last_error(&a));
    assert_eq!(battle(&a).pick, Pick::Magic);
    // 按 0 选 0 号精灵（岩石撞击）：要选敌方，焦点落在敌方第一个活着的单位上；Tab 不切到己方。
    key(&mut a, KeyCode::Char('0'));
    assert!(
        matches!(battle(&a).pick, Pick::Target { act: Act::Magic, extra: Some(0), kind: TargetType::Enemy }),
        "0 选中 0 号精灵"
    );
    assert_eq!(battle(&a).cursor.side, 1, "攻击精灵默认落在敌方");
    // Tab 是战斗控制面板（STAGE-153）：开一下再关掉，选目标的一方和当前选择都不变。
    key(&mut a, KeyCode::Tab);
    assert!(a.world.as_ref().is_some_and(|w| w.system.battle_open));
    key(&mut a, KeyCode::Esc);
    assert_eq!(battle(&a).cursor.side, 1, "只能选敌方，面板不切到己方");
    key(&mut a, KeyCode::Esc);
    assert_eq!(battle(&a).pick, Pick::Magic, "Esc 退回精灵列表");
    key(&mut a, KeyCode::Esc);
    assert_eq!(battle(&a).pick, Pick::Action, "再按 Esc 回到指令条");
}

/// STAGE-78：自愈的精灵（只能对自己用）不进选目标，选中就直接出招（`submitting` 表示已经发出去了）。
#[test]
fn magic_that_needs_no_target_is_sent_at_once() {
    let mut a = battle_app(&["player"]);
    a.world.as_mut().unwrap().entry.status.mp = 50;
    a.world.as_mut().unwrap().entry.status.magic = vec![MagicView {
        slot: 1,
        id: 0,
        name: "治愈的精灵 Lv1".into(),
        mp: 6,
        target_type: TargetType::SelfOnly,
        ..Default::default()
    }];
    key(&mut a, KeyCode::Char('2'));
    key(&mut a, KeyCode::Char('1'));
    assert_eq!(battle(&a).pick, Pick::Action, "无连接时取消旧目标");
    assert!(!battle(&a).submitting && !a.world.as_ref().unwrap().online, "无真实连接不能假装已提交，保留只读战场");
}

/// STAGE-173：回放速度和“跳过”只在系统页设，战斗里 f / Space 不再是快捷键。
#[test]
fn speed_and_skip_are_no_longer_battle_keys() {
    let mut a = battle_app(&["player"]);
    key(&mut a, KeyCode::Char('f'));
    assert_eq!(battle(&a).speed, 1, "f 不再是战斗快捷键");
    assert_eq!(a.world.as_ref().unwrap().input.text(), "", "非输入区字母不进隐藏命令栏");
    key(&mut a, KeyCode::Esc);
    // 进入结算（这里直接放一个空的回合）：f 和空格都不该改变什么。
    let round = stoneage_client::proto::BattleRound { turn: 1, ..Default::default() };
    a.world.as_mut().unwrap().battle.as_mut().unwrap().on_round(round, std::time::Instant::now());
    assert_eq!(battle(&a).phase, Phase::Replay);
    key(&mut a, KeyCode::Char('f'));
    assert_eq!(battle(&a).speed, 1, "回放里也不再切倍速");
    key(&mut a, KeyCode::Char(' '));
    assert_eq!(battle(&a).phase, Phase::Replay, "空格不再跳过回放（跳过在系统页设）");
    assert_eq!(a.world.as_ref().unwrap().input.text(), "", "f 不进输入行");
}

#[test]
fn pets_have_their_own_menu_when_it_is_their_turn() {
    let mut a = battle_app(&["pet"]);
    assert_eq!(battle(&a).phase, Phase::Input(stoneage_client::app::battle::Actor::Pet));
    key(&mut a, KeyCode::Char('2'));
    assert!(last_error(&a).contains("宠物没有可用的技能"), "宠物的 2 号是技能：{}", last_error(&a));
    // 防御会直接发出，要服务端；这里不走到发送。
}

#[test]
fn tab_and_esc_leave_the_choice_alone_while_text_is_typed() {
    let mut a = battle_app(&["player"]);
    key(&mut a, KeyCode::Char('3'));
    key(&mut a, KeyCode::Enter);
    let side = battle(&a).cursor.side;
    text(&mut a, "x");
    key(&mut a, KeyCode::Tab);
    assert_eq!(battle(&a).cursor.side, side, "输入行有字时 Tab 不换边");
    key(&mut a, KeyCode::Esc);
    assert!(matches!(battle(&a).pick, Pick::Target { .. }), "Esc 先清空输入，选择不变");
    assert_eq!(a.world.as_ref().unwrap().input.text(),"x", "关闭命令栏保留草稿");
}

fn battle_mut(a: &mut App) -> &mut BattleState {
    a.world.as_mut().unwrap().battle.as_mut().unwrap()
}

/// STAGE-109：人物指令交上、轮到宠物时，宠物指令的第一层按 Esc 退回人物指令重选；进了目标就先退回宠物指令条。
#[test]
fn esc_on_the_pet_command_goes_back_to_the_player_command() {
    let mut a = battle_app(&["player", "pet"]);
    battle_mut(&mut a).on_accepted("pet");
    key(&mut a, KeyCode::Char('1'));
    assert!(matches!(battle(&a).pick, Pick::Target { act: Act::Attack, .. }), "1 攻击，进入选目标");
    key(&mut a, KeyCode::Esc);
    assert_eq!(battle(&a).pick, Pick::Action, "选目标时 Esc 先退回宠物指令条");
    assert_eq!(battle(&a).phase, Phase::Input(stoneage_client::app::battle::Actor::Pet));
    key(&mut a, KeyCode::Esc);
    assert_eq!(battle(&a).phase, Phase::Input(stoneage_client::app::battle::Actor::Player), "再按 Esc 回到人物指令");
    assert!(battle(&a).revising, "指令条写“人物指令（改）”");
    assert_eq!(battle(&a).pick, Pick::Action);
}

/// 宠物指令交上以后（或者人物用了换宠），Esc 不起作用。
#[test]
fn esc_on_the_pet_command_does_nothing_once_it_cannot_be_revised() {
    let mut a = battle_app(&["player", "pet"]);
    battle_mut(&mut a).on_accepted("wait");
    key(&mut a, KeyCode::Esc);
    assert_eq!(battle(&a).phase, Phase::Waiting, "等待中按 Esc 没有效果");
    let mut b = battle_app(&["player", "pet"]);
    battle_mut(&mut b).on_accepted("pet");
    text(&mut b, "x");
    key(&mut b, KeyCode::Esc);
    assert_eq!(battle(&b).phase, Phase::Input(stoneage_client::app::battle::Actor::Pet), "输入行有字时 Esc 只清空输入");
    assert_eq!(b.world.as_ref().unwrap().input.text(),"x", "Esc 收起命令栏并保留草稿");
}

/// STAGE-153：第 8 项是挂机管理项，不是战斗动作——按下去只开本机挂机，不发指令、不消耗回合，焦点不动。
#[test]
fn the_eighth_item_toggles_auto_without_sending_a_command() {
    let mut a = battle_app(&["player"]);
    // 数字 8 从动作层直接选管理项：焦点不动，也不能当成战斗指令。
    key(&mut a, KeyCode::Char('8'));
    assert!(a.world.as_ref().unwrap().auto_battle, "8 号是开始自动挂机");
    assert_eq!(battle(&a).action_focus, 0, "数字键不移动焦点");
    assert!(!battle(&a).submitting, "管理项不发 battle_command");
    assert_eq!(battle(&a).pick, Pick::Action, "仍在动作层");
    // 方向键走到管理项（第一行行末往下），Enter 开关焦点都不离开这一格。
    let mut b2 = battle_app(&["player"]);
    for _ in 0..3 {
        key(&mut b2, KeyCode::Right);
    }
    key(&mut b2, KeyCode::Down);
    assert_eq!(battle(&b2).action_focus, 7, "防御 ↓ 到挂机管理项");
    key(&mut b2, KeyCode::Enter);
    assert!(b2.world.as_ref().unwrap().auto_battle);
    assert_eq!(battle(&b2).action_focus, 7, "开启后焦点仍在管理项");
    assert!(!battle(&b2).submitting && battle(&b2).pick == Pick::Action);
    // 再按一次 8 是停止：本地挂机先关；遇敌本来就关着，就不再多发一个关闭请求。
    // （托管中 Enter / 数字 / 方向键不再操作动作网格，见下一个用例。）
    key(&mut b2, KeyCode::Char('8'));
    assert!(!b2.world.as_ref().unwrap().auto_battle, "再按一次停止挂机");
    assert_eq!(battle(&b2).action_focus, 7, "停止后焦点也不变");
    assert!(
        b2.log.all().iter().any(|m| m.text == "自动挂机已停止（快捷键 8）；自动遇敌本来就是关闭的"),
        "遇敌关着时按幂等处理：{:?}",
        b2.log.all().iter().map(|m| m.text.clone()).collect::<Vec<_>>()
    );

    // 遇敌开着（比如选区计划还在跑）时，主动停止要真的去关遇敌；没有连接就如实报告不能确认。
    let mut b3 = battle_app(&["player"]);
    b3.world.as_mut().unwrap().auto_battle = true;
    b3.world.as_mut().unwrap().entry.encounter = true;
    // 托管中方向键和 Enter 都不再操作网格，停止只有 8（STAGE-173）。
    key(&mut b3, KeyCode::Char('8'));
    assert!(!b3.world.as_ref().unwrap().auto_battle, "本地挂机先关掉");
    assert!(
        b3.log.all().iter().any(|m| m.kind == Kind::Error && m.text.contains("自动遇敌关闭失败")),
        "没有连接时说明清楚：{:?}",
        b3.log.all().iter().map(|m| m.text.clone()).collect::<Vec<_>>()
    );
}

#[test]
fn ally_target_arrows_match_front_left_and_rear_right() {
    let mut a = battle_app(&["player"]);
    key(&mut a, KeyCode::Char('3'));
    key(&mut a, KeyCode::Enter);
    assert_eq!(battle(&a).cursor, Cell { side: 0, front: false, col: 2 });
    key(&mut a, KeyCode::Right);
    assert!(!battle(&a).cursor.front, "我方后列已在最右，不绕回");
    key(&mut a, KeyCode::Left);
    assert_eq!(battle(&a).cursor, Cell { side: 0, front: true, col: 2 });
    key(&mut a, KeyCode::Left);
    assert!(battle(&a).cursor.front, "我方前列已在左，不绕回");
    key(&mut a, KeyCode::Right);
    assert!(!battle(&a).cursor.front, "右移回主人后列");
}

/// STAGE-173：托管中战斗画面不再请玩家选招——数字、方向、回车、Esc 都不改选择，
/// 只留 `8` 停止；停止后按键恢复成正常选招。
#[test]
fn auto_battle_locks_the_action_grid_until_stopped() {
    let mut a = battle_app(&["player"]);
    a.world.as_mut().unwrap().auto_battle = true;

    for k in [KeyCode::Char('1'), KeyCode::Char('3'), KeyCode::Right, KeyCode::Down, KeyCode::Enter, KeyCode::Esc] {
        key(&mut a, k);
        let b = battle(&a);
        assert_eq!(b.pick, Pick::Action, "{k:?} 不该进入列表或选目标");
        assert_eq!(b.action_focus, 0, "{k:?} 不该移动焦点");
        assert!(!b.submitting, "{k:?} 不该发指令");
    }
    assert!(
        a.log.all().iter().any(|m| m.text.contains("按 8 停止自动挂机后可以手动出招")),
        "被拦下的按键要说明怎么手动：{:?}",
        a.log.all().iter().map(|m| m.text.clone()).collect::<Vec<_>>()
    );

    // 8 就是那个人物第 8 格的“停止自动挂机”。
    key(&mut a, KeyCode::Char('8'));
    assert!(!a.world.as_ref().unwrap().auto_battle, "8 停止托管");
    // 停止之后回到正常的选招流程。
    key(&mut a, KeyCode::Char('3'));
    assert_eq!(battle(&a).pick, Pick::Item, "停止后数字键恢复选招");

    // 再开托管：Tab 仍然能打开战斗控制面板（改设置、停止的另一个入口）。
    a.world.as_mut().unwrap().auto_battle = true;
    key(&mut a, KeyCode::Tab);
    assert!(a.world.as_ref().unwrap().system.battle_open, "托管中 Tab 仍能开控制面板");
}

/// STAGE-172：战斗里 Backspace 不接管返回键：道具列表里不退回指令条，Esc 照旧退。
#[test]
fn backspace_does_not_back_out_of_the_battle_lists() {
    let mut a = battle_app(&["player"]);
    key(&mut a, KeyCode::Char('3'));
    assert_eq!(battle(&a).pick, Pick::Item, "3 是道具");
    key(&mut a, KeyCode::Backspace);
    assert_eq!(battle(&a).pick, Pick::Item, "道具列表里 Backspace 不退回指令条");
    assert!(a.world.as_ref().is_some_and(|w| w.battle.is_some()), "战斗没有结束");
    key(&mut a, KeyCode::Esc);
    assert_eq!(battle(&a).pick, Pick::Action, "Esc 照旧退回指令条");
}

/// STAGE-173：`8` 是全局停止键——结算阶段按下去也要停。
/// （原来只在“轮到本机下指令且没在发送”时才认 8，玩家在结算里按了没反应。）
#[test]
fn stop_key_also_works_during_the_replay() {
    let mut a = battle_app(&["player"]);
    a.world.as_mut().unwrap().auto_battle = true;
    let round = stoneage_client::proto::BattleRound { turn: 1, ..Default::default() };
    a.world.as_mut().unwrap().battle.as_mut().unwrap().on_round(round, std::time::Instant::now());
    assert_eq!(battle(&a).phase, Phase::Replay);

    key(&mut a, KeyCode::Char('8'));
    assert!(!a.world.as_ref().unwrap().auto_battle, "结算阶段按 8 也要停");

    // 指令在途（submitting）时同样认。
    let mut b = battle_app(&["player"]);
    b.world.as_mut().unwrap().auto_battle = true;
    b.world.as_mut().unwrap().battle.as_mut().unwrap().submitting = true;
    key(&mut b, KeyCode::Char('8'));
    assert!(!b.world.as_ref().unwrap().auto_battle, "发送中按 8 也要停");
}

/// STAGE-173：托管中按 `7` 只停自动遇敌，自动出招继续；按 `8` 才是全停。
#[test]
fn key_seven_only_stops_the_encounter() {
    let mut a = battle_app(&["player"]);
    a.world.as_mut().unwrap().auto_battle = true;
    a.world.as_mut().unwrap().entry.encounter = true;
    // 这个用例不连服务端：离线状态下 7 只写提示、不发请求，自动战斗必须保持开着。
    a.world.as_mut().unwrap().online = false;

    key(&mut a, KeyCode::Char('7'));
    assert!(a.world.as_ref().unwrap().auto_battle, "7 不停自动战斗");
    assert!(
        a.log.all().iter().any(|m| m.text.contains("按 7 停止自动遇敌")),
        "{:?}",
        a.log.all().iter().map(|m| m.text.clone()).collect::<Vec<_>>()
    );

    // 8 才是全停（托管 + 遇敌）。
    key(&mut a, KeyCode::Char('8'));
    assert!(!a.world.as_ref().unwrap().auto_battle, "8 停自动战斗");
}

/// 没开自动战斗时 7 还是普通的第 7 格（换宠），不是停止遇敌的快捷键。
#[test]
fn key_seven_is_a_normal_slot_without_auto_battle() {
    let mut a = battle_app(&["player"]);
    key(&mut a, KeyCode::Char('7'));
    assert!(a.log.all().iter().all(|m| !m.text.contains("按 7 停止自动遇敌")));
}
