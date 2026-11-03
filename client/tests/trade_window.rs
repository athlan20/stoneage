//! STAGE-295 交易窗口：80×24 渲染（标题、两块报价、按钮、提示）、默认焦点在「取消交易」、锁定与取消的按键、
//! Esc 的二次确认、对方改报价的提示与焦点回位、石币输入的本地拒绝。直接构造状态，不连服务端。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::{backend::TestBackend, Terminal};
use stoneage_client::app::trade::TradeOp;
use stoneage_client::app::trade_keys::{handle, Outcome};
use stoneage_client::app::trade_keys::item_candidates;
use stoneage_client::app::trade_state::{Focus, TradeWindow};
use stoneage_client::app::{App, LoadState, Options, Screen, WorldState};
use stoneage_client::proto::{Trade, TradeItem, TradeLastError, TradePet, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app_with(trade: Trade) -> App {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    let mut w = WorldState::new(WorldEntry::default());
    w.online = true;
    w.entry.character.name = "勇者小明".into();
    w.scene.exit_load = LoadState::Ready;
    w.scene.npc_load = LoadState::Ready;
    w.scene.area_load = LoadState::Ready;
    w.scene.players.load = LoadState::Ready;
    w.trade = Some(TradeWindow::new(trade));
    a.world = Some(w);
    a
}

fn base_trade() -> Trade {
    Trade {
        id: "t-17".into(),
        rev: 2,
        idle_expires_in: 300,
        me: stoneage_client::proto::TradeSide {
            name: "勇者小明".into(),
            items: vec![TradeItem {
                slot: Some(5),
                item_id: 20,
                name: "小斧头".into(),
                count: 1,
                ..Default::default()
            }],
            gold: 5000,
            ..Default::default()
        },
        peer: stoneage_client::proto::TradeSide {
            name: "路人乙".into(),
            items: vec![TradeItem {
                item_id: 7,
                name: "回复药".into(),
                count: 5,
                ..Default::default()
            }],
            ..Default::default()
        },
        last_error: None,
    }
}

fn press(a: &mut App, code: KeyCode) -> Outcome {
    let w = a.world.as_mut().unwrap();
    handle(w, KeyEvent::new(code, KeyModifiers::NONE))
}

fn win(a: &mut App) -> &mut TradeWindow {
    a.world.as_mut().unwrap().trade.as_mut().unwrap()
}

fn draw(a: &mut App, width: u16, height: u16) -> String {
    let mut t = Terminal::new(TestBackend::new(width, height)).unwrap();
    t.draw(|f| ui::render(f, a)).unwrap();
    let b = t.backend().buffer();
    (0..height)
        .map(|y| {
            let mut s = String::new();
            let mut x = 0;
            while x < width {
                let v = b[(x, y)].symbol();
                s.push_str(v);
                x += UnicodeWidthStr::width(v).max(1) as u16;
            }
            s
        })
        .collect::<Vec<_>>()
        .join("\n")
}

#[test]
fn window_renders_fully_at_80x24_with_cancel_focused() {
    let mut a = app_with(base_trade());
    let screen = draw(&mut a, 80, 24);
    for want in [
        "交易 · 路人乙",
        "我的报价",
        "对方的报价",
        "小斧头 x1",
        "回复药 x5",
        "5,000",
        "[取消交易]",
        "Esc 取消交易",
    ] {
        assert!(screen.contains(want), "缺少 {want}:\n{screen}");
    }
    assert_eq!(win(&mut a).focus, Focus::Cancel);
}

#[test]
fn lock_then_cancel_keys_send_the_right_requests() {
    let mut a = app_with(base_trade());
    // 焦点从「取消交易」往上走一格到「锁定」，Enter 锁定，带当前的 rev。
    press(&mut a, KeyCode::Up);
    assert_eq!(win(&mut a).focus, Focus::Lock);
    let out = press(&mut a, KeyCode::Enter);
    assert_eq!(
        out.ops,
        vec![TradeOp::Lock {
            id: "t-17".into(),
            rev: 2
        }]
    );
}

#[test]
fn esc_with_offer_asks_first_and_defaults_to_continue() {
    let mut a = app_with(base_trade());
    let out = press(&mut a, KeyCode::Esc);
    assert!(out.ops.is_empty(), "有报价时 Esc 先问，不直接取消");
    assert!(win(&mut a).ask_cancel);
    // 默认「继续交易」，直接 Enter 不取消。
    let out = press(&mut a, KeyCode::Enter);
    assert!(out.ops.is_empty());
    assert!(!win(&mut a).ask_cancel);
    // 再 Esc，切到「取消交易」后 Enter 才取消。
    press(&mut a, KeyCode::Esc);
    press(&mut a, KeyCode::Right);
    let out = press(&mut a, KeyCode::Enter);
    assert_eq!(out.ops, vec![TradeOp::Cancel { id: "t-17".into() }]);
}

#[test]
fn esc_with_empty_offer_cancels_at_once() {
    let mut t = base_trade();
    t.me.items.clear();
    t.me.gold = 0;
    let mut a = app_with(t);
    let out = press(&mut a, KeyCode::Esc);
    assert_eq!(out.ops, vec![TradeOp::Cancel { id: "t-17".into() }]);
}

#[test]
fn peer_offer_change_shows_notice_and_resets_focus() {
    let mut a = app_with(base_trade());
    a.unicode = true;
    press(&mut a, KeyCode::Up);
    assert_eq!(win(&mut a).focus, Focus::Lock);
    let mut next = base_trade();
    next.rev = 3;
    next.peer.gold = 300;
    next.peer.pet = Some(TradePet {
        name: "乌力".into(),
        level: 5,
        ..Default::default()
    });
    let w = a.world.as_mut().unwrap();
    assert_eq!(w.trade.as_mut().unwrap().apply(next), Some(true));
    assert_eq!(win(&mut a).focus, Focus::Cancel, "对方改价后焦点回到取消交易");
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("对方改动了报价，双方锁定已取消"), "{screen}");
    assert!(screen.contains("●"), "变了的行要有标记");
    // 自己的下一次按键清掉提示。
    press(&mut a, KeyCode::Down);
    assert!(!win(&mut a).peer_changed);
}

#[test]
fn last_error_is_shown_in_red_notice_line() {
    let mut t = base_trade();
    t.last_error = Some(TradeLastError {
        code: "BAG_FULL".into(),
        who: "me".into(),
    });
    let mut a = app_with(t);
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("你物品栏已满，请整理后再交易"), "{screen}");
}

#[test]
fn gold_input_over_holdings_is_refused_locally() {
    let mut a = app_with(base_trade());
    a.world.as_mut().unwrap().inventory.gold = 100;
    // 焦点直接放到石币行（第 3 行），Enter 打开石币输入。
    win(&mut a).focus = Focus::Row(2);
    press(&mut a, KeyCode::Enter);
    for c in ['5', '0', '0'] {
        press(&mut a, KeyCode::Char(c));
    }
    let out = press(&mut a, KeyCode::Enter);
    assert!(out.ops.is_empty(), "超过持有量不发请求");
    assert_eq!(out.notice.as_deref(), Some("你身上的石币不够"));
}

#[test]
fn confirm_needs_both_locked() {
    let mut a = app_with(base_trade());
    assert!(!win(&mut a).focus_order().contains(&Focus::Confirm));
    let w = win(&mut a);
    w.trade.me.locked = true;
    w.trade.peer.locked = true;
    assert!(w.focus_order().contains(&Focus::Confirm));
    win(&mut a).focus = Focus::Confirm;
    let out = press(&mut a, KeyCode::Enter);
    assert_eq!(
        out.ops,
        vec![TradeOp::Confirm {
            id: "t-17".into(),
            rev: 2
        }]
    );
}

#[test]
fn long_item_name_is_cut_but_count_and_gold_survive_at_80x24() {
    let mut t = base_trade();
    t.me.items[0].name = "超级无敌大宝剑之神圣光辉传说".into();
    t.me.items[0].count = 12;
    t.me.gold = 1_234_567;
    let mut a = app_with(t);
    a.unicode = true;
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains("…"), "长名字要截断:\n{screen}");
    assert!(screen.contains(" x12"), "数量必须保留:\n{screen}");
    assert!(screen.contains("1,234,567"), "石币必须完整:\n{screen}");
    assert!(!screen.contains("超级无敌大宝剑之神圣光辉传说"), "整名不能画出来");
}

#[test]
fn long_item_name_degrades_to_ascii_when_unicode_is_off() {
    let mut t = base_trade();
    t.me.items[0].name = "超级无敌大宝剑之神圣光辉传说".into();
    t.me.items[0].count = 12;
    let mut a = app_with(t);
    a.unicode = false;
    let screen = draw(&mut a, 80, 24);
    assert!(screen.contains(".."), "非 Unicode 模式用两个点截断:\n{screen}");
    assert!(!screen.contains("…"), "非 Unicode 模式不画省略号:\n{screen}");
    assert!(screen.contains(" x12"), "数量必须保留:\n{screen}");
    assert!(screen.contains("Up/Down"), "提示行降级为 ASCII:\n{screen}");
}

fn bag_slot(n: u8, name: &str, count: u32) -> stoneage_client::proto::InventorySlot {
    stoneage_client::proto::InventorySlot {
        slot: n,
        item_id: 20,
        name: name.into(),
        count,
        ..Default::default()
    }
}

#[test]
fn item_picker_lists_bag_slots_5_to_19_and_sends_the_pick() {
    let mut t = base_trade();
    t.me.items.clear();
    t.me.gold = 0;
    let mut a = app_with(t);
    a.world.as_mut().unwrap().inventory.slots =
        vec![bag_slot(2, "头盔", 1), bag_slot(5, "小斧头", 1), bag_slot(21, "箱子", 1), bag_slot(7, "空", 0)];
    win(&mut a).focus = Focus::Row(0);
    assert!(press(&mut a, KeyCode::Enter).ops.is_empty());
    let w = a.world.as_ref().unwrap();
    assert_eq!(item_candidates(w, 0), vec![Some(5)], "只列 5–19 格、有数量的物品");
    let out = press(&mut a, KeyCode::Enter);
    assert_eq!(
        out.ops,
        vec![TradeOp::Offer { id: "t-17".into(), items: vec![5], gold: 0, pet: None }]
    );
    assert!(win(&mut a).picker.is_none(), "选完关掉选择层");
}

#[test]
fn item_picker_can_clear_a_filled_row_and_esc_closes_without_request() {
    let mut a = app_with(base_trade());
    a.world.as_mut().unwrap().inventory.slots = vec![bag_slot(5, "小斧头", 1)];
    win(&mut a).focus = Focus::Row(0);
    press(&mut a, KeyCode::Enter);
    let w = a.world.as_ref().unwrap();
    assert_eq!(item_candidates(w, 0), vec![None, Some(5)], "已经放了东西，第一项是清空");
    let out = press(&mut a, KeyCode::Esc);
    assert!(out.ops.is_empty() && win(&mut a).picker.is_none(), "Esc 只关选择层");
    press(&mut a, KeyCode::Enter);
    let out = press(&mut a, KeyCode::Enter);
    assert_eq!(
        out.ops,
        vec![TradeOp::Offer { id: "t-17".into(), items: vec![], gold: 5000, pet: None }]
    );
}

#[test]
fn gold_input_within_holdings_sends_the_offer_and_keeps_items() {
    let mut a = app_with(base_trade());
    a.world.as_mut().unwrap().inventory.gold = 10_000;
    win(&mut a).focus = Focus::Row(2);
    press(&mut a, KeyCode::Enter);
    for c in ['3', '0', '0'] {
        press(&mut a, KeyCode::Char(c));
    }
    press(&mut a, KeyCode::Backspace);
    press(&mut a, KeyCode::Char('0'));
    let out = press(&mut a, KeyCode::Enter);
    assert_eq!(
        out.ops,
        vec![TradeOp::Offer { id: "t-17".into(), items: vec![5], gold: 300, pet: None }]
    );
}
