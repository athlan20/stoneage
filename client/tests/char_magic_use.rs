//! STAGE-231 角色页精灵「使用」的按键与界面测试：`↑↓` 选精灵，`Enter` 打开动作，滋润选对象（自己或宠物）；
//! MP 不够在消息栏说明且不发请求；动作和对象列表在 80×24 不越界，`--unicode false` 下仍可读。
//! 发出的请求形状和服务端用例 `magic_use_heal` 一致（见 `docs/protocol.md` 5.51），这里只看界面状态。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, CharStep, Layer, Options, Screen, WorldState};
use stoneage_client::proto::{Pet, Status, TargetType, MagicView, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app(unicode: bool) -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode, ..Options::default() })
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn draw(a: &mut App, w: u16, h: u16) -> String {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, a)).unwrap();
    // 宽字符（中文）占两格，第二格是空的：跳过它，才能按原文搜索。
    let buf = term.backend().buffer();
    (0..h)
        .map(|y| {
            let mut line = String::new();
            let mut x = 0;
            while x < w {
                let sym = buf[(x, y)].symbol();
                line.push_str(sym);
                x += UnicodeWidthStr::width(sym).max(1) as u16;
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

fn spirit(slot: u8, id: u32, name: &str, mp: u32, target: TargetType, effect: &str) -> MagicView {
    MagicView { slot, id, name: name.into(), mp, target_type: target, effect: effect.into() }
}

/// 角色页：精灵在 1 号（滋润，要选对象）、3 号（治愈，直接发）；`mp` 和宠物由参数决定。
fn char_page(unicode: bool, mp: u32, pets: &[(u8, &str, u32)]) -> App {
    let mut a = app(unicode);
    let mut entry = WorldEntry::default();
    entry.status = Status {
        mp,
        max_mp: 100,
        magic: vec![
            spirit(1, 10, "滋润的精灵 Lv1", 6, TargetType::Ally, "回复己方一个 HP 65"),
            spirit(3, 0, "治愈的精灵 Lv1", 6, TargetType::SelfOnly, "回复自己 HP 80"),
        ],
        ..Status::default()
    };
    let mut w = WorldState::new(entry);
    w.nav.push(Layer::Page(0));
    for (slot, name, hp) in pets {
        w.pets.pets.push(Pet { slot: *slot, name: (*name).into(), hp: *hp, ..Default::default() });
    }
    a.world = Some(w);
    a.screen = Screen::World;
    key(&mut a, KeyCode::Char('2'));
    a
}

fn step(a: &App) -> Option<&CharStep> {
    a.world.as_ref().and_then(|w| w.char_step.as_ref())
}

fn logged(a: &App, text: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(text))
}

#[test]
fn pick_spirit_then_target_list_shows_self_and_pets() {
    let mut a = char_page(true, 100, &[(0, "凯比", 51), (1, "加美", 0)]);
    // 焦点在 1 号精灵（滋润）：Enter 打开动作，动作面板写「使用」。
    key(&mut a, KeyCode::Enter);
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("使用") && s.contains("Enter 执行"), "动作面板\n{s}");
    // 「使用」后进入对象列表：自己、0 号凯比、1 号加美（倒下的写「倒」），队友不列。
    key(&mut a, KeyCode::Enter);
    assert!(matches!(step(&a), Some(CharStep::MagicTarget { slot: 1, index: 0 })));
    let s = draw(&mut a, 120, 40);
    assert!(s.contains("对谁用") && s.contains("自己") && s.contains("0 凯比") && s.contains("1 加美·倒"), "对象列表\n{s}");
    assert!(!s.contains("队友"), "一期没有队友，不列");
    // ↓ 选到 1 号宠物；Esc 回动作列表，再 Esc 整个流程退掉。
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    assert!(matches!(step(&a), Some(CharStep::MagicTarget { index: 2, .. })), "到头停住");
    key(&mut a, KeyCode::Esc);
    assert!(matches!(step(&a), Some(CharStep::MagicAction { slot: 1 })));
    key(&mut a, KeyCode::Esc);
    assert!(step(&a).is_none());
}

#[test]
fn not_enough_mp_explains_and_sends_nothing() {
    let mut a = char_page(true, 5, &[(0, "凯比", 51)]);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Enter);
    assert!(matches!(step(&a), Some(CharStep::MagicAction { slot: 1 })), "MP 不够不进对象列表");
    assert!(logged(&a, "MP 不够，滋润的精灵 Lv1需要 MP 6（现有 5）"), "消息栏写明需要的 MP");
}

#[test]
fn action_and_target_panels_fit_the_smallest_window() {
    for unicode in [true, false] {
        let mut a = char_page(unicode, 100, &[(0, "凯比", 51), (1, "加美", 0), (2, "很长很长的宠物名字", 30)]);
        key(&mut a, KeyCode::Enter);
        let s = draw(&mut a, 80, 24);
        assert!(s.contains("使用"), "unicode={unicode}\n{s}");
        key(&mut a, KeyCode::Enter);
        let s = draw(&mut a, 80, 24);
        assert!(s.contains("对谁用") && s.contains("自己"), "unicode={unicode}\n{s}");
        // 很长的宠物名在窗口外截掉，不越界也不 panic。
        let _ = draw(&mut a, 40, 12);
    }
}

#[test]
fn healing_spirit_goes_straight_to_the_request() {
    // 治愈只回自己：不进对象列表，直接发（这里只看步骤已经结束）。
    let mut a = char_page(true, 100, &[(0, "凯比", 51)]);
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Down);
    assert!(a.world.as_ref().unwrap().char_step.is_none());
    key(&mut a, KeyCode::Enter);
    assert!(matches!(step(&a), Some(CharStep::MagicAction { slot: 3 })));
    key(&mut a, KeyCode::Enter);
    assert!(step(&a).is_none(), "治愈不选对象，动作直接结束");
}
