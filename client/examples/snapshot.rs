//! 把某一页画成纯文本打印出来，方便不开终端也能看界面：
//! `cargo run --example snapshot -- <login|register|characters|create1|create2|create3|world|battle> [宽 高]`

use std::time::Instant;

use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::BattleState;
use stoneage_client::app::{App, CreateStep, Options, Screen, WorldState};
use stoneage_client::proto::{
    Appearance, BattleAllow, BattleStart, BattleTurn, BattleUnit, Character, CharacterList, ElementPoints, Location, Progress,
    WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn main() {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let page = args.first().map_or("login", String::as_str);
    let w: u16 = args.get(1).and_then(|s| s.parse().ok()).unwrap_or(100);
    let h: u16 = args.get(2).and_then(|s| s.parse().ok()).unwrap_or(30);
    let mut app = App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() });
    let ch = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 12,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 1006, map_name: "萨姆吉尔村长之家".into() },
    };
    match page {
        "register" => app.screen = Screen::Register,
        "characters" => {
            app.screen = Screen::Characters;
            app.chars.list = CharacterList { max_slots: 2, characters: vec![ch] };
        }
        "create1" | "create2" | "create3" => {
            app.screen = Screen::Create;
            app.create.step = [CreateStep::Look, CreateStep::Stats, CreateStep::Home][usize::from(page.as_bytes()[6] - b'1')];
            app.create.name.set("勇者小明");
        }
        "world" => {
            let mut entry = WorldEntry { character: ch, ..WorldEntry::default() };
            entry.position.floor = 1006;
            entry.position.map_name = "萨姆吉尔村长之家".into();
            entry.position.x = 15;
            entry.position.y = 22;
            entry.status.level = 12;
            entry.status.hp = 180;
            entry.status.max_hp = 220;
            entry.status.mp = 30;
            entry.status.max_mp = 100;
            app.world = Some(WorldState::new(entry));
            app.screen = Screen::World;
        }
        "battle" => {
            let unit = |no, kind: &str, name: &str, level, pct| BattleUnit {
                no,
                kind: kind.into(),
                name: name.into(),
                level,
                hp_pct: pct,
                elements: ElementPoints { earth: 60, water: 40, fire: 0, wind: 0 },
                ..BattleUnit::default()
            };
            let mut hero = unit(0, "player", "勇者小明", 12, 75);
            hero.hp = Some(45);
            hero.max_hp = Some(60);
            let mut pet = unit(5, "pet", "乌力", 5, 100);
            pet.hp = Some(35);
            pet.max_hp = Some(35);
            let mut boar = unit(15, "monster", "野猪", 3, 100);
            boar.status = vec!["poison".into()];
            let start = BattleStart {
                    field: None,
                battle_id: "b_1".into(),
                kind: "wild".into(),
                scene: "野外".into(),
                you: 0,
                allow: BattleAllow { escape: true, capture: true },
                units: vec![hero, pet, boar, unit(16, "monster", "野猪", 3, 40)],
                npc: None,
            };
            let mut battle = BattleState::from_start(start);
            let turn = BattleTurn {
                    field: None,
                battle_id: "b_1".into(),
                turn: 3,
                deadline_ms: 1_030_000,
                server_time_ms: 1_000_000,
                needs: vec!["player".into(), "pet".into()],
                optional: vec![],
                progress: Progress { done: 0, total: 1 },
            };
            battle.on_turn(turn, Instant::now());
            let mut entry = WorldEntry { character: ch, ..WorldEntry::default() };
            entry.position.map_name = "萨姆吉尔村".into();
            let mut world = WorldState::new(entry);
            world.battle = Some(battle);
            app.world = Some(world);
            app.screen = Screen::World;
        }
        _ => {}
    }
    app.log.system("正在连接服务端 127.0.0.1:7065…");
    app.log.error("账号或密码错误");
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, &mut app)).unwrap();
    let buf = term.backend().buffer().clone();
    for y in 0..h {
        let mut line = String::new();
        let mut x = 0;
        while x < w {
            let sym = buf[(x, y)].symbol();
            line.push_str(sym);
            x += UnicodeWidthStr::width(sym).max(1) as u16;
        }
        println!("{}", line.trim_end());
    }
}
