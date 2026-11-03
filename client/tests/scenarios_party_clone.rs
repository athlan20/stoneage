//! STAGE-186 验收（场景）：模拟服务端按 `ui_party_battle_clone` 用例回放（严格模式）。带分身队开战后，
//! `battle_turn.optional` 列出可指定目标的分身；按 `f` 打开分身面板，用数字键给分身 1 指定目标、用文字命令
//! 给分身 2 的宠物指定目标（都发出 `battle_command`，actor 为 `clone:1` / `clone_pet:2`），分身卡片写出
//! “自动→目标号”；队长再交人物和宠物两条指令就结算。

#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Down, Enter, Esc, Tab};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::{Actor, BattleState, Phase, Pick};
use stoneage_client::app::{App, Layer, SOCIAL_MENU, Screen};
use stoneage_client::ui;
use support::{Harness, opts};
use unicode_width::UnicodeWidthStr;

fn draw(h: &mut Harness, w: u16, hgt: u16) -> String {
    let mut term = Terminal::new(TestBackend::new(w, hgt)).unwrap();
    term.draw(|f| ui::render(f, &mut h.app)).unwrap();
    let buf = term.backend().buffer().clone();
    (0..hgt)
        .map(|y| {
            let (mut line, mut x) = (String::new(), 0);
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

fn enter_world(case: &str) -> Harness {
    let mut h = Harness::new(case, opts);
    h.text("alice");
    h.key(Tab);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
    h
}

fn open_social(h: &mut Harness) {
    while !h.app.world.as_ref().unwrap().at_scene() {
        h.key(Esc);
    }
    h.key(Esc);
    for _ in 0..5 {
        h.key(Down);
    }
    h.key(Enter);
    let w = h.app.world.as_ref().unwrap();
    assert_eq!(w.menu, SOCIAL_MENU);
    assert!(w.nav.is(&Layer::Page(SOCIAL_MENU)));
}

fn battle(a: &App) -> Option<&BattleState> {
    a.world.as_ref()?.battle.as_ref()
}

fn input(a: &App, turn: u32, actor: Actor) -> bool {
    battle(a).is_some_and(|b| b.turn == turn && b.phase == Phase::Input(actor))
}

#[test]
fn clone_targets_are_sent_before_the_leader_commands() {
    let mut h = enter_world("ui_party_battle_clone");
    open_social(&mut h);
    h.key(Enter); // 组成分身队
    h.wait("第 1 回合轮到人物出招", 5, |a| input(a, 1, Actor::Player));
    assert_eq!(battle(&h.app).unwrap().optional.len(), 8, "8 个分身人宠都能指定");
    let s = draw(&mut h, 80, 24);
    assert!(s.contains("f 分身"), "指令条提示分身入口：\n{s}");

    // f 打开分身面板，没指定的行都写服务端默认目标：合击顺序的第一个（敌方 2 号 = 16 号）。
    h.key(Char('f'));
    assert_eq!(battle(&h.app).unwrap().pick, Pick::Clones);
    let s = draw(&mut h, 80, 24);
    assert!(s.contains("分身 ·") && s.contains("默认→ 敌 2"), "{s}");
    // 数字键 1 选分身 1，再按 2 选敌方 2 号（16 号）：发出 clone:1。
    h.keys(&[Char('1'), Char('2')]);
    h.wait("分身 1 的目标记下", 5, |a| battle(a).is_some_and(|b| b.clone_targets.get(&1) == Some(&16)));
    assert_eq!(battle(&h.app).unwrap().pick, Pick::Clones, "选完回到面板");
    // 文字命令给分身 2 的宠物（单位 7）指定敌方 2 号。
    h.key(Esc);
    h.command("clone p2 e2");
    h.wait("分身宠物的目标记下", 5, |a| battle(a).is_some_and(|b| b.clone_targets.get(&7) == Some(&16)));
    h.key(Esc);
    let b = battle(&h.app).unwrap();
    assert_eq!(b.pick, Pick::Action);
    assert!(input(&h.app, 1, Actor::Player), "分身指令不改变队长的阶段");
    let s = draw(&mut h, 80, 24);
    assert!(s.contains("自→2"), "指定了目标的分身卡片写出目标号：\n{s}");
    let s = draw(&mut h, 120, 50);
    assert!(s.contains("自动→2"), "{s}");

    // 队长照常交两条指令，交齐就结算。
    h.keys(&[Char('1'), Char('3')]);
    h.wait("宠物轮到出招", 5, |a| input(a, 1, Actor::Pet));
    h.keys(&[Char('1'), Char('3')]);
    h.wait("结果窗口弹出", 10, |a| battle(a).is_some_and(|b| b.result.is_some()));
    h.finish();
}
