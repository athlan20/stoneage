//! 自动战斗设置页（STAGE-173）的入口、按键分层与草稿保护。
use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::auto_battle::edit::Ask;
use stoneage_client::app::auto_battle::capture_log::{self, CaptureRecord, Quality};
use stoneage_client::app::auto_battle::store::Store;
use stoneage_client::app::system::SystemStep;
use stoneage_client::app::{AUTO_MENU, App, GAME_MENU, Layer, Options, SYSTEM_MENU, Screen, WorldState};
use stoneage_client::proto::{BattleStart, MagicView, WorldEntry};

fn app() -> App {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    a.world = Some(WorldState::new(WorldEntry::default()));
    let w = a.world.as_mut().unwrap();
    w.menu = SYSTEM_MENU;
    w.nav.show_page(SYSTEM_MENU);
    // 方案落到临时目录，不碰玩家真正的 %APPDATA%。
    let dir = std::env::temp_dir().join(format!("stage173-keys-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    a.auto_store = Some(Store::open_in(&dir, "测试服", "勇者"));
    a
}

/// 刚进游戏的场景页（没有打开任何菜单页）。
fn scene_app() -> App {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    a.world = Some(WorldState::new(WorldEntry::default()));
    let dir = std::env::temp_dir().join(format!("stage173-scene-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    a.auto_store = Some(Store::open_in(&dir, "测试服", "勇者"));
    a
}

fn key(a: &mut App, k: KeyCode) {
    a.on_key(KeyEvent::new(k, KeyModifiers::NONE));
}

fn editing(a: &App) -> bool {
    matches!(a.world.as_ref().unwrap().system.step, Some(SystemStep::AutoBattle))
}

#[test]
fn system_page_opens_the_same_editor_as_the_battle_panel() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    assert!(editing(&a));
    assert!(a.world.as_ref().unwrap().auto_edit.is_some());

    // 没有改动时 Esc 直接回系统行表，不弹确认。
    key(&mut a, KeyCode::Esc);
    assert!(!editing(&a));
    assert!(a.world.as_ref().unwrap().auto_edit.is_none());
    assert!(a.world.as_ref().unwrap().nav.is(&Layer::Page(SYSTEM_MENU)), "回到系统页而不是离开");

    // 战斗中：Tab 面板里同一项打开同一个编辑器。
    let mut b = app();
    let mut battle = stoneage_client::app::battle::BattleState::from_start(BattleStart::default());
    battle.phase = stoneage_client::app::battle::Phase::Input(stoneage_client::app::battle::Actor::Player);
    b.world.as_mut().unwrap().battle = Some(battle);
    key(&mut b, KeyCode::Tab);
    assert!(b.world.as_ref().unwrap().system.battle_open);
    b.world.as_mut().unwrap().system.index = 10;
    key(&mut b, KeyCode::Enter);
    assert!(editing(&b));
    // Esc 先退编辑回面板，面板还开着；再 Esc 才关面板。
    key(&mut b, KeyCode::Esc);
    assert!(!editing(&b));
    assert!(b.world.as_ref().unwrap().system.battle_open, "第一次 Esc 只退出设置页");
}

#[test]
fn tab_changes_columns_and_arrows_move_rows() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Tab);
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().column, 1, "Tab 到恢复栏");
    key(&mut a, KeyCode::BackTab);
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().column, 0);
    key(&mut a, KeyCode::Down);
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().row, 1);
    key(&mut a, KeyCode::Home);
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().row, 0);
}

#[test]
fn digits_do_not_switch_pages_while_editing() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Char('3'));
    assert!(editing(&a), "编辑器里数字键不切页");
    assert_eq!(a.world.as_ref().unwrap().menu, SYSTEM_MENU);
}

#[test]
fn edits_are_a_draft_until_saved() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    // 恢复栏第 2 行是人物补血门槛，→ 调高一档。
    key(&mut a, KeyCode::Tab);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Right);
    let ed = a.world.as_ref().unwrap().auto_edit.as_ref().unwrap();
    assert!(ed.dirty);
    assert_eq!(ed.draft.recover.player_heal_pct, 35);
    assert_eq!(a.auto_profile().recover.player_heal_pct, 30, "没保存前方案不变");

    // Esc 弹出三选一，默认「继续编辑」。
    key(&mut a, KeyCode::Esc);
    let ed = a.world.as_ref().unwrap().auto_edit.as_ref().unwrap();
    assert_eq!(ed.ask, Some(Ask::Exit));
    assert_eq!(ed.ask_choice, 0);
    key(&mut a, KeyCode::Enter);
    let ed = a.world.as_ref().unwrap().auto_edit.as_ref().unwrap();
    assert!(ed.ask.is_none(), "默认继续编辑，不退出也不保存");
    assert!(ed.dirty);

    // 保存并应用：方案立刻生效。
    key(&mut a, KeyCode::Enter); // 选中行仍是补血门槛，这里 Enter 只是提示
    assert!(a.auto_profile().recover.player_heal_pct == 30);
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Right); // 光标右移到「保存并应用」
    key(&mut a, KeyCode::Enter);
    assert_eq!(a.auto_profile().recover.player_heal_pct, 35);
    assert!(!a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().dirty);
}

#[test]
fn discard_restores_the_saved_profile() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Right); // 人物动作：攻击 → 精灵
    assert!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().dirty);
    key(&mut a, KeyCode::Esc);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right); // 继续编辑 → 保存 → 放弃
    key(&mut a, KeyCode::Enter);
    let ed = a.world.as_ref().unwrap().auto_edit.as_ref().unwrap();
    assert!(!ed.dirty);
    assert_eq!(ed.draft.player.act, stoneage_client::app::auto_battle::ActKind::Attack);
}

#[test]
fn settings_page_renders_at_the_minimum_size() {
    use ratatui::{Terminal, backend::TestBackend};
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    for (x, y) in [(80, 24), (120, 40)] {
        let mut t = Terminal::new(TestBackend::new(x, y)).unwrap();
        t.draw(|f| stoneage_client::ui::render(f, &mut a)).unwrap();
        let text = t.backend().buffer().content.iter().map(|c| c.symbol()).collect::<String>();
        for want in ["行动", "恢复", "挂机", "捕捉", "方案与播放", "人物常规动作"] {
            assert!(text.replace(' ', "").contains(want), "{x}×{y} 缺少 {want}");
        }
    }
}

#[test]
fn game_menu_opens_the_auto_battle_page_and_esc_goes_back() {
    let mut a = scene_app();
    key(&mut a, KeyCode::Esc); // 场景 → 游戏菜单
    assert!(a.world.as_ref().unwrap().nav.is(&Layer::Menu));
    let at = GAME_MENU.iter().position(|(name, _)| *name == "自动战斗").expect("游戏菜单里有「自动战斗」");
    a.world.as_mut().unwrap().menu_index = at;
    key(&mut a, KeyCode::Enter);
    let w = a.world.as_ref().unwrap();
    assert_eq!(w.menu, AUTO_MENU, "顶层第 10 页");
    assert!(w.auto_edit.is_some(), "进页就是编辑器");
    assert!(w.system.step.is_none(), "顶层页不靠 system.step");

    // 编辑器里的方向键归它自己，不当成切页或页面选择。
    key(&mut a, KeyCode::Tab);
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().column, 1);

    // 没有改动时 Esc 回游戏菜单，草稿丢掉。
    key(&mut a, KeyCode::Esc);
    let w = a.world.as_ref().unwrap();
    assert!(w.auto_edit.is_none());
    assert!(w.nav.is(&Layer::Menu), "回到游戏菜单，不是场景");
}

#[test]
fn delay_is_configurable_and_saved() {
    let mut a = scene_app();
    key(&mut a, KeyCode::Esc);
    let at = GAME_MENU.iter().position(|(name, _)| *name == "自动战斗").unwrap();
    a.world.as_mut().unwrap().menu_index = at;
    key(&mut a, KeyCode::Enter);
    // 切到「方案与播放」栏，走到「出招延迟」（第 8 项）。
    for _ in 0..4 {
        key(&mut a, KeyCode::Tab);
    }
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().column, 4);
    key(&mut a, KeyCode::End);
    key(&mut a, KeyCode::Up); // End 停在「手动让位」，上一行就是「出招延迟」
    let ed = a.world.as_ref().unwrap().auto_edit.as_ref().unwrap();
    assert_eq!(ed.field().label(), "出招延迟");
    let before = ed.draft.playback.delay_ms;
    assert_eq!(before, 500, "默认 500ms，比原来写死的 1500ms 快");

    key(&mut a, KeyCode::Right);
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().draft.playback.delay_ms, 1000);
    key(&mut a, KeyCode::Left);
    key(&mut a, KeyCode::Left);
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().draft.playback.delay_ms, 300);
    // 再往左一档是「即时」（0ms）：收到回合结算就出招，不再夹到 50ms。
    key(&mut a, KeyCode::Left);
    let ed = a.world.as_ref().unwrap().auto_edit.as_ref().unwrap();
    assert_eq!(ed.draft.playback.delay_ms, 0);
    assert_eq!(ed.field().value(&ed.draft, &a.auto_context()).0, "即时");

    // 保存并应用：方案里的延迟生效。
    key(&mut a, KeyCode::Home);
    for _ in 0..3 {
        key(&mut a, KeyCode::Down);
    }
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().field().label(), "保存并应用");
    key(&mut a, KeyCode::Enter);
    assert_eq!(a.auto_profile().playback.delay_ms, 0);
    assert!(a.auto_delay().is_zero(), "即时档不能留在 50ms");
}

/// 画一屏，返回全部文字（用来验证中间区域画的是谁）。
fn draw(a: &mut App, w: u16, h: u16) -> String {
    use ratatui::{Terminal, backend::TestBackend};
    let mut t = Terminal::new(TestBackend::new(w, h)).unwrap();
    t.draw(|f| stoneage_client::ui::render(f, a)).unwrap();
    t.backend().buffer().content.iter().map(|c| c.symbol()).collect()
}

/// STAGE-173：设置页压在战斗上面——战斗在后台继续，退出设置后回到面板。
#[test]
fn settings_page_sits_on_top_of_the_battle() {
    use stoneage_client::app::battle::{Actor, BattleState, Phase};

    let mut a = app();
    let mut battle = BattleState::from_start(BattleStart::default());
    battle.phase = Phase::Input(Actor::Player);
    a.world.as_mut().unwrap().battle = Some(battle);

    // 没开自动战斗时，Tab 打开面板落在「自动挂机」上。
    key(&mut a, KeyCode::Tab);
    assert!(a.world.as_ref().unwrap().system.battle_open);
    assert_eq!(a.world.as_ref().unwrap().system.index, 6);

    // 进设置页：中间区域画的是设置，不是战场。
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    assert!(a.world.as_ref().unwrap().auto_edit.is_some());
    // 中文每格占两列，缓冲里字符之间会多出空格，比较前先去掉。
    let screen = draw(&mut a, 80, 24).replace(' ', "");
    assert!(screen.contains("自动战斗设置") && screen.contains("人物常规动作"), "{screen}");

    // Esc 退回面板（面板还开着，step 清空）。
    key(&mut a, KeyCode::Esc);
    let w = a.world.as_ref().unwrap();
    assert!(w.auto_edit.is_none());
    assert!(w.system.battle_open && w.system.step.is_none());
    // 再 Esc 关面板，战斗还在。
    key(&mut a, KeyCode::Esc);
    let w = a.world.as_ref().unwrap();
    assert!(!w.system.battle_open);
    assert!(w.battle.is_some(), "设置和面板都不影响战斗本身");
}

/// 自动战斗中按 Tab 直接落在「自动战斗设置」上（STAGE-173）。
#[test]
fn tab_lands_on_the_settings_item_while_auto_is_on() {
    use stoneage_client::app::battle::{Actor, BattleState, Phase};

    let mut a = app();
    let mut battle = BattleState::from_start(BattleStart::default());
    battle.phase = Phase::Input(Actor::Player);
    a.world.as_mut().unwrap().battle = Some(battle);
    a.world.as_mut().unwrap().auto_battle = true;
    key(&mut a, KeyCode::Tab);
    assert_eq!(a.world.as_ref().unwrap().system.index, 10, "托管中 Tab 直接到设置项");
}

/// 打进一串字符（名单输入框里用）。
fn type_text(a: &mut App, text: &str) {
    for c in text.chars() {
        key(a, KeyCode::Char(c));
    }
}

/// STAGE-173 M10：逃跑敌人名单在设置页里直接录入、删除、保存。
#[test]
fn flee_name_list_is_edited_in_place() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);

    // 挂机栏：前两行是只读的遇敌开关与区域，不能打开名单。
    for _ in 0..2 {
        key(&mut a, KeyCode::Tab);
    }
    let ed = a.world.as_ref().unwrap().auto_edit.as_ref().unwrap();
    assert_eq!(ed.column, 2, "第 3 栏是挂机");
    assert_eq!(ed.field().label(), "连续遇敌");
    key(&mut a, KeyCode::Enter);
    assert!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().names.is_none(), "只读行不打开名单");

    // 打开自动逃跑，走到「逃跑敌人名单」。
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().field().label(), "自动逃跑");
    key(&mut a, KeyCode::Right);
    assert!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().draft.flee.enabled);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().field().label(), "逃跑敌人名单");

    key(&mut a, KeyCode::Enter);
    assert!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().names.is_some(), "Enter 打开名单编辑");
    type_text(&mut a, "野猪");
    key(&mut a, KeyCode::Enter);
    let ed = a.world.as_ref().unwrap().auto_edit.as_ref().unwrap();
    assert_eq!(ed.draft.flee.names, vec!["野猪".to_string()]);
    assert!(ed.dirty, "加名单算改动");
    assert!(ed.names.is_some(), "加完还留在名单编辑里");
    assert!(!a.auto_profile().flee.names.iter().any(|n| n == "野猪"), "没保存前方案不变");

    // 再加一个，然后删掉第一个。
    type_text(&mut a, "乌力");
    key(&mut a, KeyCode::Enter);
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().draft.flee.names.len(), 2);
    key(&mut a, KeyCode::Up);
    key(&mut a, KeyCode::Delete);
    assert_eq!(
        a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().draft.flee.names,
        vec!["乌力".to_string()]
    );

    // Esc 退出名单编辑，回到行表，设置页还开着。
    key(&mut a, KeyCode::Esc);
    let ed = a.world.as_ref().unwrap().auto_edit.as_ref().unwrap();
    assert!(ed.names.is_none() && editing(&a));

    // 保存并应用后方案里就有这条名单。
    for _ in 0..5 {
        key(&mut a, KeyCode::Tab);
    }
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().column, 2, "Tab 循环回挂机栏");
}

/// STAGE-173 M13：捕捉目标名称同样在设置页里录入。
#[test]
fn capture_name_list_is_edited_in_place() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    for _ in 0..3 {
        key(&mut a, KeyCode::Tab);
    }
    let ed = a.world.as_ref().unwrap().auto_edit.as_ref().unwrap();
    assert_eq!(ed.column, 3, "第 4 栏是捕捉");
    key(&mut a, KeyCode::Right); // 自动捕捉 开
    assert!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().draft.capture.enabled);
    key(&mut a, KeyCode::Down); // 抓宠方式
    key(&mut a, KeyCode::Down); // 削血到
    key(&mut a, KeyCode::Down); // 捕捉目标名称
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().field().label(), "捕捉目标名称");
    key(&mut a, KeyCode::Enter);
    type_text(&mut a, "乌力");
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Esc);
    assert_eq!(
        a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().draft.capture.names,
        vec!["乌力".to_string()]
    );
}

/// STAGE-173：设置页开着时按 `8` 也要停自动战斗（原来按键被设置页整个吃掉了）。
#[test]
fn stop_key_works_while_the_settings_page_is_open() {
    use stoneage_client::app::auto_battle::edit::Origin;
    use stoneage_client::app::battle::{Actor, BattleState, Phase};

    let mut a = app();
    let mut battle = BattleState::from_start(BattleStart::default());
    battle.phase = Phase::Input(Actor::Player);
    a.world.as_mut().unwrap().battle = Some(battle);
    a.world.as_mut().unwrap().auto_battle = true;
    a.open_auto_settings(Origin::Panel);
    assert!(a.world.as_ref().unwrap().auto_edit.is_some());

    key(&mut a, KeyCode::Char('8'));
    assert!(!a.world.as_ref().unwrap().auto_battle, "设置页开着也能按 8 停");
    assert!(a.world.as_ref().unwrap().auto_edit.is_some(), "停挂机不关设置页");
}

/// 正在录名单时 `8` 是文字，不能当成停止键。
#[test]
fn digit_eight_is_text_while_typing_a_name() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    for _ in 0..2 {
        key(&mut a, KeyCode::Tab);
    }
    // 打开自动逃跑再进名单编辑。
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    assert!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().names.is_some());

    // 给一个战斗和一个开着的托管，确认 8 没被抢去停机。
    a.world.as_mut().unwrap().auto_battle = true;
    let mut battle = stoneage_client::app::battle::BattleState::from_start(BattleStart::default());
    battle.phase = stoneage_client::app::battle::Phase::Input(stoneage_client::app::battle::Actor::Player);
    a.world.as_mut().unwrap().battle = Some(battle);
    key(&mut a, KeyCode::Char('8'));
    assert!(a.world.as_ref().unwrap().auto_battle, "录名单时 8 是文字，不停挂机");
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().names.as_ref().unwrap().input.text(), "8");
}

/// STAGE-173：补血精灵在设置页里能手动选（恢复栏第 3 行），默认「自动」。
#[test]
fn heal_magic_is_selected_in_the_settings_page() {
    let mut a = app();
    a.world.as_mut().unwrap().entry.status.magic = vec![
        MagicView { slot: 0, name: "火球的精灵 Lv1".into(), mp: 5, ..MagicView::default() },
        MagicView { slot: 3, name: "治愈的精灵 Lv2".into(), mp: 12, ..MagicView::default() },
    ];
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Tab); // 恢复栏
    assert_eq!(a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().column, 1);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    let ed = a.world.as_ref().unwrap().auto_edit.as_ref().unwrap();
    assert_eq!(ed.field().label(), "补血精灵");
    assert!(ed.draft.recover.heal_magic_name.is_empty(), "默认自动");

    // 往右两下到「治愈的精灵 Lv2」，方案里记下栏位与名字。
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    let p = &a.world.as_ref().unwrap().auto_edit.as_ref().unwrap().draft;
    assert_eq!((p.recover.heal_magic_slot, p.recover.heal_magic_name.as_str()), (3, "治愈的精灵 Lv2"));
}

fn editor(a: &App) -> &stoneage_client::app::auto_battle::edit::Editor {
    a.world.as_ref().unwrap().auto_edit.as_ref().unwrap()
}

/// 进设置页、走到捕捉栏并打开「自动捕捉」。
fn open_capture(a: &mut App) {
    a.world.as_mut().unwrap().system.index = 10;
    key(a, KeyCode::Enter);
    for _ in 0..3 {
        key(a, KeyCode::Tab);
    }
    assert_eq!(editor(a).column, 3, "第 4 栏是捕捉");
    key(a, KeyCode::Right);
    assert!(editor(a).draft.capture.enabled);
}

/// 从捕捉栏第一行往下走到指定标签（行表增删项时不用改次数）。
fn goto_row(a: &mut App, label: &str) {
    for _ in 0..24 {
        if editor(a).field().label() == label {
            return;
        }
        key(a, KeyCode::Down);
    }
    panic!("这一栏没有「{label}」");
}

/// 按行取屏幕文字，去掉中文字符之间的空格（宽字符的占位格），方便整行比较。
fn screen_rows(a: &mut App, w: u16, h: u16) -> Vec<String> {
    use ratatui::{Terminal, backend::TestBackend};
    let mut t = Terminal::new(TestBackend::new(w, h)).unwrap();
    t.draw(|f| stoneage_client::ui::render(f, a)).unwrap();
    let buf = t.backend().buffer();
    buf.content
        .chunks(w as usize)
        .map(|row| row.iter().map(|c| c.symbol()).collect::<String>().replace(' ', ""))
        .collect()
}

/// STAGE-285：捕捉栏第二行是「抓宠方式」，80×24 下能看见，`←→` 在两种方式之间切。
#[test]
fn capture_mode_row_shows_and_cycles() {
    let mut a = app();
    open_capture(&mut a);
    goto_row(&mut a, "抓宠方式");
    let rows = screen_rows(&mut a, 80, 24);
    assert!(
        rows.iter().any(|r| r.contains("抓宠方式：边战斗边抓") && r.contains("每场都打")),
        "默认边战斗边抓：{rows:?}"
    );
    key(&mut a, KeyCode::Right);
    assert_eq!(editor(&a).draft.capture.mode, stoneage_client::app::auto_battle::CaptureMode::Escape);
    let rows = screen_rows(&mut a, 80, 24);
    assert!(rows.iter().any(|r| r.contains("逃跑抓") && r.contains("没有目标就逃")), "切换后写逃跑抓：{rows:?}");

    // 自动捕捉关掉以后这一行变灰，说明原因。
    key(&mut a, KeyCode::Up); // 回到「自动捕捉」
    assert_eq!(editor(&a).field().label(), "自动捕捉");
    key(&mut a, KeyCode::Right);
    assert!(!editor(&a).draft.capture.enabled);
    goto_row(&mut a, "抓宠方式");
    let rows = screen_rows(&mut a, 80, 24);
    assert!(
        rows.iter().any(|r| r.contains("抓宠方式") && r.contains("自动捕捉关着")),
        "关着时要说明原因：{rows:?}"
    );
}

/// STAGE-264 第 1 条：削血档位多一档「满血抓」，在 80×24 下整行显示。
#[test]
fn capture_hp_offers_full_hp_capture() {
    let mut a = app();
    open_capture(&mut a);
    goto_row(&mut a, "削血到");
    assert_eq!(editor(&a).field().label(), "削血到");
    assert_eq!(editor(&a).draft.capture.hp_pct, 20);
    for _ in 0..16 {
        key(&mut a, KeyCode::Right);
    }
    assert_eq!(editor(&a).draft.capture.hp_pct, 100, "20 → 95 → 满血抓");
    key(&mut a, KeyCode::Right);
    assert_eq!(editor(&a).draft.capture.hp_pct, 100, "100 是最上一档");

    let rows = screen_rows(&mut a, 80, 24);
    let row = rows.iter().find(|r| r.contains("削血到")).expect("捕捉栏有削血行");
    assert!(row.contains("满血抓") && row.contains("不削血，直接抓；高血量目标几乎抓不到"), "{row}");
    assert!(rows.iter().any(|r| r.contains("捕捉目标名称")), "下面的行没被挤掉");

    key(&mut a, KeyCode::Left);
    assert_eq!(editor(&a).draft.capture.hp_pct, 95, "往下回到 95%");
}

/// STAGE-264 第 2 条：名单输入框用 `|` 或 `｜` 一次加多个，重复和空项不进名单。
#[test]
fn capture_names_take_several_at_once() {
    let mut a = app();
    open_capture(&mut a);
    goto_row(&mut a, "捕捉目标名称");
    key(&mut a, KeyCode::Enter);

    type_text(&mut a, "乌力|凯比｜克克尔");
    key(&mut a, KeyCode::Enter);
    assert_eq!(editor(&a).draft.capture.names, vec!["乌力", "凯比", "克克尔"]);
    assert!(editor(&a).names.is_some(), "加完还留在名单编辑里");

    // 全是已有的名字：名单不变，输入框保留，提示说明。
    type_text(&mut a, "乌力|凯比|");
    key(&mut a, KeyCode::Enter);
    assert_eq!(editor(&a).draft.capture.names.len(), 3);
    assert!(editor(&a).hint.as_deref().unwrap_or("").contains("都已经在名单里"));

    // 清空输入后：首尾空白、空项、重复都丢掉，只加新的「野猪」。
    for _ in 0..20 {
        key(&mut a, KeyCode::Backspace);
    }
    type_text(&mut a, " | 野猪 | ｜ 野猪 ");
    key(&mut a, KeyCode::Enter);
    assert_eq!(editor(&a).draft.capture.names, vec!["乌力", "凯比", "克克尔", "野猪"]);

    let rows = screen_rows(&mut a, 80, 24);
    assert!(rows.iter().any(|r| r.contains("可用|一次输入多个")), "说明行写明批量输入");
    assert!(rows.iter().any(|r| r.contains("新名字")), "输入框在屏上");

    key(&mut a, KeyCode::Esc);
    assert!(editor(&a).names.is_none());
}

/// STAGE-264 第 3 条：等级上下限可以手输，1–4 级也能设；非法与 min>max 被拒绝、原值保持。
#[test]
fn capture_levels_can_be_typed() {
    let mut a = app();
    open_capture(&mut a);
    goto_row(&mut a, "捕捉等级下限");
    key(&mut a, KeyCode::Enter);
    assert!(editor(&a).level.is_some(), "Enter 打开手输");
    type_text(&mut a, "1");
    key(&mut a, KeyCode::Enter);
    assert_eq!(editor(&a).draft.capture.level_min, 1, "手输 1 级");
    assert!(editor(&a).level.is_none() && editor(&a).dirty);

    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    type_text(&mut a, "5");
    key(&mut a, KeyCode::Enter);
    assert_eq!(editor(&a).draft.capture.level_max, 5);

    // min > max：不接受，原值保持，输入框留着。
    key(&mut a, KeyCode::Up);
    key(&mut a, KeyCode::Enter);
    type_text(&mut a, "9");
    key(&mut a, KeyCode::Enter);
    assert_eq!(editor(&a).draft.capture.level_min, 1, "下限 9 > 上限 5，不接受");
    assert!(editor(&a).level.is_some(), "输入框还在");
    assert!(editor(&a).hint.as_deref().unwrap_or("").contains("大于上限"));

    // 负数、超范围、Esc 取消。
    for _ in 0..3 {
        key(&mut a, KeyCode::Backspace);
    }
    type_text(&mut a, "-3");
    key(&mut a, KeyCode::Enter);
    assert!(editor(&a).hint.as_deref().unwrap_or("").contains("负数"));
    for _ in 0..2 {
        key(&mut a, KeyCode::Backspace);
    }
    type_text(&mut a, "201");
    key(&mut a, KeyCode::Enter);
    assert!(editor(&a).hint.as_deref().unwrap_or("").contains("超出范围"));
    key(&mut a, KeyCode::Esc);
    assert!(editor(&a).level.is_none());
    assert_eq!(editor(&a).draft.capture.level_min, 1);

    // 上限手输 0 就是不限。
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    type_text(&mut a, "0");
    key(&mut a, KeyCode::Enter);
    assert_eq!(editor(&a).draft.capture.level_max, 0);

    let rows = screen_rows(&mut a, 80, 24);
    assert!(rows.iter().any(|r| r.contains("捕捉等级上限")), "等级行没被挤掉");
}

/// STAGE-264 第 4 条：四项资质档位在 不限 → 0 → −1 → −2 → −3 → 不限 之间循环，左右都能走。
#[test]
fn capture_keep_rows_cycle_through_the_tiers() {
    let mut a = app();
    open_capture(&mut a);
    goto_row(&mut a, "体力");
    assert_eq!(editor(&a).draft.capture.keep.vital, None, "缺省全不限");
    key(&mut a, KeyCode::Right);
    assert_eq!(editor(&a).draft.capture.keep.vital, Some(0), "不限 → 0（满）");
    for want in [-1, -2, -3] {
        key(&mut a, KeyCode::Right);
        assert_eq!(editor(&a).draft.capture.keep.vital, Some(want));
    }
    key(&mut a, KeyCode::Right);
    assert_eq!(editor(&a).draft.capture.keep.vital, None, "−3 再往右回到不限");
    key(&mut a, KeyCode::Left);
    assert_eq!(editor(&a).draft.capture.keep.vital, Some(-3), "不限再往左是 −3");

    key(&mut a, KeyCode::Down);
    assert_eq!(editor(&a).field().label(), "腕力");
    assert_eq!(editor(&a).draft.capture.keep.vital, Some(-3), "各项互不影响");
    key(&mut a, KeyCode::Right);
    assert_eq!(editor(&a).draft.capture.keep.strength, Some(0));

    let rows = screen_rows(&mut a, 80, 24);
    assert!(rows.iter().any(|r| r.contains("体力") && r.contains("−3档")), "体力行显示档位");
    assert!(rows.iter().any(|r| r.contains("腕力") && r.contains("0档（满）")), "0 写明是满");
    assert!(rows.iter().any(|r| r.contains("最差不低于−3档；0为满")), "说明写明语义");
    // 80×24 下捕捉栏十行放不下，光标往下走时列表跟着滚动，末尾的丢弃记录入口也能滚到。
    for _ in 0..3 {
        key(&mut a, KeyCode::Down);
    }
    assert_eq!(editor(&a).field().label(), "抓宠丢弃记录");
    let rows = screen_rows(&mut a, 80, 24);
    assert!(rows.iter().any(|r| r.contains("抓宠丢弃记录")), "末尾一行是丢弃记录入口");
    assert!(rows.iter().any(|r| r.contains("耐久力") && r.contains("不限")), "耐久力行滚到了");
    assert!(rows.iter().any(|r| r.contains("速度")), "速度行滚到了");
}

/// STAGE-264 第 4 条：自动捕捉关着时资质行不能改，提示写明原因（和削血、等级行一样）。
#[test]
fn capture_keep_rows_wait_for_auto_capture() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    for _ in 0..3 {
        key(&mut a, KeyCode::Tab);
    }
    for _ in 0..5 {
        key(&mut a, KeyCode::Down);
    }
    key(&mut a, KeyCode::Right);
    assert_eq!(editor(&a).draft.capture.keep.vital, None);
    assert!(editor(&a).hint.as_deref().unwrap_or("").contains("自动捕捉关着"));
}

/// STAGE-264 第 5 条：丢弃记录入口在捕捉栏末尾；Enter 打开只读列表，Esc 回设置页。
#[test]
fn capture_log_page_opens_from_the_last_row_and_esc_returns() {
    let mut a = app();
    open_capture(&mut a);
    goto_row(&mut a, "抓宠丢弃记录");
    key(&mut a, KeyCode::Enter);
    assert!(editor(&a).log.is_some());
    let rows = screen_rows(&mut a, 80, 24);
    assert!(rows.iter().any(|r| r.contains("抓宠丢弃记录·勇者")), "标题写明角色");
    assert!(rows.iter().any(|r| r.contains("没有记录")), "没有记录时直说");

    key(&mut a, KeyCode::Esc);
    assert!(editor(&a).log.is_none(), "Esc 只退回设置页");
    assert!(editing(&a), "还在设置页里");
    assert_eq!(editor(&a).column, 3, "仍在捕捉栏");
}

/// STAGE-264 第 5 条：清空要确认，默认选「取消」；记录按新在上排，长行截断不错行。
#[test]
fn capture_log_lists_newest_first_and_clear_defaults_to_cancel() {
    let mut a = app();
    let path = capture_log::path(a.auto_store.as_ref().unwrap());
    let rec = |name: &str, reason: &str| CaptureRecord {
        at: "2026-10-21 04:12:33".into(),
        name: name.into(),
        level: 1,
        quality: vec![Quality { stat: "vital".into(), points: 21, tier: -1 }],
        reason: reason.into(),
    };
    capture_log::append(&path, "勇者", rec("乌力", "体力 −1 < 0")).unwrap();
    capture_log::append(&path, "勇者", rec("凯比", &"很长的原因".repeat(12))).unwrap();

    open_capture(&mut a);
    goto_row(&mut a, "抓宠丢弃记录");
    key(&mut a, KeyCode::Enter);
    let rows = screen_rows(&mut a, 80, 24);
    let pos = |name: &str| rows.iter().position(|r| r.contains(name) && r.contains("Lv1"));
    assert!(pos("凯比").unwrap() < pos("乌力").unwrap(), "最新的在上");
    assert!(rows.iter().any(|r| r.contains("体力21(−1)")), "资质一行");
    assert!(rows.iter().any(|r| r.contains("…")), "长原因截断加省略号");

    key(&mut a, KeyCode::Enter); // 选中「清空记录」→ 确认框
    assert!(editor(&a).log.as_ref().unwrap().ask);
    key(&mut a, KeyCode::Enter); // 默认「取消」
    assert!(!editor(&a).log.as_ref().unwrap().ask);
    assert_eq!(capture_log::load(&path, "勇者").len(), 2, "默认取消，记录没动");

    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Enter);
    assert!(capture_log::load(&path, "勇者").is_empty(), "选了清空才删");
    assert_eq!(editor(&a).log.as_ref().unwrap().hint.as_deref(), Some("已清空 2 条记录"));
}

/// STAGE-260：恢复栏末尾三行。默认吃肉开、门槛 90%、丢弃关；丢弃从关切到开要确认，默认取消。
#[test]
fn post_battle_rows_default_and_drop_asks_before_turning_on() {
    use stoneage_client::app::auto_battle::edit::Editor;
    fn edit(a: &App) -> &Editor {
        a.world.as_ref().unwrap().auto_edit.as_ref().unwrap()
    }
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Tab); // 恢复栏
    for _ in 0..12 {
        key(&mut a, KeyCode::Down);
    }
    assert_eq!(edit(&a).field().label(), "战后自动吃肉回血");
    assert!(edit(&a).draft.recover.post_heal_item, "默认开");

    key(&mut a, KeyCode::Down);
    assert_eq!(edit(&a).field().label(), "战后吃肉门槛");
    key(&mut a, KeyCode::Left);
    assert_eq!(edit(&a).draft.recover.post_heal_pct, 85);
    key(&mut a, KeyCode::Right);
    assert_eq!(edit(&a).draft.recover.post_heal_pct, 90, "默认 90，步进 5");

    key(&mut a, KeyCode::Down);
    assert_eq!(edit(&a).field().label(), "战后自动丢弃肉类道具");
    assert!(!edit(&a).draft.recover.post_drop_item, "默认关");
    // 关 → 开：先问一句，默认是「取消」。
    key(&mut a, KeyCode::Right);
    assert_eq!(edit(&a).ask, Some(Ask::DropOn));
    assert_eq!(edit(&a).ask_choice, 0);
    assert!(!edit(&a).draft.recover.post_drop_item, "没确认前不变");
    key(&mut a, KeyCode::Enter);
    assert_eq!(edit(&a).ask, None);
    assert!(!edit(&a).draft.recover.post_drop_item, "默认取消，保持关");

    // 再问一次，←→ 选到「开启」再 Enter 才写进草稿。
    key(&mut a, KeyCode::Right);
    key(&mut a, KeyCode::Right);
    assert_eq!(edit(&a).ask_choice, 1);
    key(&mut a, KeyCode::Enter);
    assert!(edit(&a).draft.recover.post_drop_item);
    assert!(edit(&a).dirty);
    assert_eq!(a.auto_profile().recover.post_drop_item, false, "没保存前方案不变");

    // 开 → 关不用问。
    key(&mut a, KeyCode::Right);
    assert_eq!(edit(&a).ask, None);
    assert!(!edit(&a).draft.recover.post_drop_item);

    // Esc 只关掉确认框，不退出编辑。
    key(&mut a, KeyCode::Right);
    assert_eq!(edit(&a).ask, Some(Ask::DropOn));
    key(&mut a, KeyCode::Esc);
    assert_eq!(edit(&a).ask, None);
    assert!(!edit(&a).draft.recover.post_drop_item);
    assert!(editing(&a));
}

/// STAGE-260：80×24 下恢复栏多了三行，选到最后一行时三行都要看得见（不错行、不被截掉）。
#[test]
fn post_battle_rows_fit_at_80_by_24() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 10;
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Tab); // 恢复栏
    for _ in 0..14 {
        key(&mut a, KeyCode::Down);
    }
    let text = draw(&mut a, 80, 24).replace(' ', "");
    for want in ["战后自动吃肉回血", "战后吃肉门槛", "战后自动丢弃肉类道具"] {
        assert!(text.contains(want), "80×24 缺少 {want}");
    }
}
