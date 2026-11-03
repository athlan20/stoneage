//! 结算回放的界面（STAGE-110，`docs/battle-ui.md`「结算回放」）：中线字幕写在两方前排之间的空行上；
//! 指令条和顶部信息条写 `结算中 3/7`。前排之间没有空行时字幕写在指令条上，见 `ui/battle/bar.rs` 的单元测试。效果（闪色、抖动、残影）的时间相关部分在 `ui/battle/fx.rs` 的单元测试里。
//! 界面渲染读的是真实时钟，所以这里只测不随时间变化的内容，时间相关的用 `BattleState::advance` 推到想要的那一拍后再画。

use std::time::{Duration, Instant};

use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::BattleState;
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{
    Appearance, BattleAllow, BattleEvent, BattleRound, BattleStart, BattleTurn, BattleUnit, Character, ElementPoints,
    Location, Progress, WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app(unicode: bool) -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode, ..Options::default() })
}

fn draw(app: &mut App, w: u16, h: u16) -> String {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
    let buf = term.backend().buffer().clone();
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

fn unit(no: u8, kind: &str, name: &str, level: u32, pct: u32) -> BattleUnit {
    BattleUnit { no, kind: kind.into(), name: name.into(), level, hp_pct: pct, ..BattleUnit::default() }
}

/// 一场战斗：我方勇者和乌力，敌方两只野猪（协议号 15、16）。
fn battle() -> BattleState {
    let mut hero = unit(0, "player", "勇者小明", 12, 75);
    hero.hp = Some(45);
    hero.max_hp = Some(60);
    hero.elements = ElementPoints { earth: 60, water: 40, fire: 0, wind: 0 };
    let start = BattleStart { party: None,
            field: None,
        battle_id: "b_1".into(),
        kind: "wild".into(),
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow { escape: true, capture: true },
        units: vec![
            hero,
            unit(5, "pet", "乌力", 5, 100),
            unit(15, "monster", "野猪", 3, 100),
            unit(16, "monster", "野猪", 3, 40),
        ],
        npc: None,
    };
    let mut b = BattleState::from_start(start);
    let turn = BattleTurn {
            field: None,
        battle_id: "b_1".into(),
        turn: 1,
        deadline_ms: 1_030_000,
        server_time_ms: 1_000_000,
        needs: vec!["player".into()],
        optional: vec![],
        progress: Progress { done: 0, total: 1 },
    };
    b.on_turn(turn, Instant::now());
    b
}

/// 我方勇者攻击敌 3 号野猪，打掉 23 点（命中拍）。
fn hit_round() -> BattleRound {
    let ev = BattleEvent {
        kind: "attack".into(),
        actor: Some(0),
        effects: vec![stoneage_client::proto::BattleEffect {
            target: 15,
            result: "hit".into(),
            delta: -23,
            hp_pct: Some(70),
            ..Default::default()
        }],
        ..BattleEvent::default()
    };
    BattleRound { turn: 1, events: vec![ev], ..BattleRound::default() }
}

/// 进入世界并挂上战斗。
fn battle_app(unicode: bool, battle: BattleState) -> App {
    let mut a = app(unicode);
    let entry = WorldEntry {
        character: Character {
            slot: 0,
            name: "勇者小明".into(),
            level: 12,
            appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
            location: Location { floor: 1000, map_name: "萨姆吉尔村".into() },
        },
        ..WorldEntry::default()
    };
    let mut world = WorldState::new(entry);
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.battle = Some(battle);
    a.world = Some(world);
    a.screen = Screen::World;
    // STAGE-248：战斗记录先排队，刷新时写进消息日志；界面的战斗日志区从消息日志读，所以这里和真实界面一样先写一次。
    a.flush_battle_log();
    a
}

#[test]
fn action_text_only_appears_in_the_battle_log_in_all_sizes_and_modes() {
    for (w,h) in [(80,24),(120,40),(200,60)] {
        for unicode in [true,false] {
            let t0=Instant::now();let mut b=battle();
            b.on_round(hit_round(),t0);assert!(!b.advance(t0));
            assert!(!b.advance(t0+Duration::from_millis(250)));
            let s=draw(&mut battle_app(unicode,b),w,h);
            assert!(!s.contains("━━ 攻击") && !s.contains("-- 攻击 -->"),"不在中场或指令区重复播报\n{s}");
            assert!(s.contains("战斗日志") && s.contains("勇者小明 攻击 野猪 -23"),"日志保留完整动作结果\n{s}");
            assert!(s.contains("结算中 1/1") && !s.contains("空格 跳过"),"只留进度，跳过去系统页设\n{s}");
        }
    }
}

#[test]
fn finishing_replay_does_not_restore_a_caption_or_drop_the_log() {
    let t0=Instant::now();let mut b=battle();b.on_round(hit_round(),t0);
    b.advance(t0);b.advance(t0+Duration::from_millis(250));
    assert!(b.advance(t0+Duration::from_millis(700)));
    let s=draw(&mut battle_app(true,b),120,40);
    assert!(!s.contains("━━ 攻击"));
    assert!(s.contains("勇者小明 攻击 野猪 -23"));
}