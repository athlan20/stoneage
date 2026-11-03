//! STAGE-167：离线验收。用正式 TUI 渲染器导出逐帧 HTML，既不连接服务端也不修改存档。
//! cargo run --example battle_preview -- --export ../build/battle-actors.html
use ratatui::{Terminal, backend::TestBackend, style::Color};
use std::{
    fs,
    time::{Duration, Instant},
};
use stoneage_client::{
    app::battle::BattleState,
    app::{App, Options, Screen, WorldState},
    proto::*,
    ui,
};
use unicode_width::UnicodeWidthStr;

#[path = "battle_preview/scenarios.rs"]
mod scenarios;
use scenarios::{Case, cases, unit};

fn app(case: &Case, elapsed: u64, unicode: bool, reduced: bool) -> App {
    let now = Instant::now();
    let start = now - Duration::from_millis(elapsed);
    let units = if case.full {
        (0..20)
            .map(|no| unit(no, if no % 10 < 5 { "勇者" } else { "乌力" }))
            .collect()
    } else {
        vec![
            unit(0, "勇者小明"),
            unit(5, "乌力"),
            unit(10, "对手"),
            unit(15, "野猪"),
            unit(16, "贝鲁卡"),
        ]
    };
    let mut b = BattleState::from_start(BattleStart {
        scene: "动作验收".into(),
        you: 0,
        units,
        ..Default::default()
    });
    if let Some(magic) = case.events.first().and_then(|ev| ev.magic.as_ref()) {
        if let Some(pet) = b.units.get_mut(&5) {
            match magic.func.as_str() {
                "revive" => {
                    pet.hp_pct = 0;
                    pet.hp = Some(0);
                }
                "heal" => {
                    pet.hp_pct = 40;
                    pet.hp = Some(40);
                }
                "cure" => {
                    pet.status = vec!["poison".into()];
                }
                _ => {}
            }
        }
    }
    b.on_round(
        BattleRound {
            turn: 1,
            events: case.events.clone(),
            ..Default::default()
        },
        start,
    );
    for ms in (0..=elapsed).step_by(10) {
        b.advance(start + Duration::from_millis(ms));
    }
    b.advance(now);
    let mut a = App::new(Options {
        unicode,
        reduce_motion: reduced,
        ..Default::default()
    });
    let mut world = WorldState::new(WorldEntry::default());
    world.battle = Some(b);
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

fn css(c: Color) -> &'static str {
    match c {
        Color::Black => "#000",
        Color::Red => "#cd3131",
        Color::Green => "#0dbc79",
        Color::Yellow => "#e5c07b",
        Color::Blue => "#2472c8",
        Color::Magenta => "#bc3fbc",
        Color::Cyan => "#11a8cd",
        Color::White => "#e5e5e5",
        Color::DarkGray => "#666",
        Color::Gray => "#aaa",
        Color::LightRed => "#f14c4c",
        Color::LightGreen => "#23d18b",
        Color::LightYellow => "#f5f543",
        Color::LightBlue => "#3b8eea",
        Color::LightMagenta => "#d670d6",
        Color::LightCyan => "#29b8db",
        _ => "#ddd",
    }
}

fn push_run(
    out: &mut Vec<(u16, u16, &'static str, String)>,
    x: u16,
    y: u16,
    color: Color,
    text: &str,
) {
    let shown = text.trim_matches(' ');
    if !shown.is_empty() {
        let leading = text.len() - text.trim_start_matches(' ').len();
        out.push((x + leading as u16, y, css(color), shown.to_string()));
    }
}

fn snapshot(a: &mut App, w: u16, h: u16) -> Vec<(u16, u16, &'static str, String)> {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, a)).unwrap();
    let buf = term.backend().buffer();
    let mut out = Vec::new();
    for y in 0..h {
        let mut x = 0;
        let mut from = 0;
        let mut color = buf[(0, y)].fg;
        let mut text = String::new();
        while x < w {
            let cell = &buf[(x, y)];
            if cell.fg != color {
                push_run(&mut out, from, y, color, &text);
                from = x;
                color = cell.fg;
                text.clear();
            }
            text.push_str(cell.symbol());
            x += UnicodeWidthStr::width(cell.symbol()).max(1) as u16;
        }
        push_run(&mut out, from, y, color, &text);
    }
    out
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let path = args
        .iter()
        .position(|s| s == "--export")
        .and_then(|i| args.get(i + 1))
        .expect("用 --export 指定输出 HTML 路径");
    let compact = args.iter().any(|s| s == "--compact");
    let unicode = !args.iter().any(|s| s == "--ascii");
    let reduced = args.iter().any(|s| s == "--reduce-motion");
    let (w, h) = if compact { (80, 24) } else { (120, 40) };
    let mut data = vec![];
    for case in cases() {
        let duration = if case.events.len() > 1 { 1400 } else { 700 };
        let frames: Vec<_> = (0..duration)
            .step_by(33)
            .map(|t| snapshot(&mut app(&case, t, unicode, reduced), w, h))
            .collect();
        data.push(serde_json::json!({"name":case.name,"width":w,"height":h,"frames":frames}));
    }
    fs::write(path, render_page(&data)).unwrap();
    println!(
        "已导出 {} 个场景：{}（正式终端渲染器逐帧输出）",
        data.len(),
        path
    );
}

// 模板使用 .html.in，避免被误当成能直接打开的成品页面。
fn render_page(data: &[serde_json::Value]) -> String {
    let frames = serde_json::to_string(data).unwrap().replace('<', "\\u003c");
    include_str!("battle_preview.html.in")
        .replace("__PLAYER__", include_str!("battle_preview.js"))
        .replace("__FRAMES__", &frames)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn exported_page_has_the_player_and_data_instead_of_template_tokens() {
        let html = render_page(&[serde_json::json!({"name":"测试","frames":[]})]);
        assert!(!html.contains("__PLAYER__"));
        assert!(!html.contains("__FRAMES__"));
        assert!(html.contains("JSON.parse(document.getElementById('frames').textContent)"));
        assert!(html.contains("测试"));
    }

    #[test]
    fn frame_data_cannot_end_its_script_container() {
        let html = render_page(&[serde_json::json!({"name":"</script><script>oops"})]);
        let data = html
            .split("type=\"application/json\">")
            .nth(1)
            .unwrap()
            .split("</script>")
            .next()
            .unwrap();
        let parsed: Vec<serde_json::Value> = serde_json::from_str(data).unwrap();
        assert_eq!(parsed[0]["name"], "</script><script>oops");
    }

    #[test]
    fn directly_opened_html_is_a_launcher_without_template_code() {
        let html = include_str!("battle_preview.html");
        assert!(html.contains("../../build/battle-actors.html"));
        assert!(!html.contains("__PLAYER__") && !html.contains("__FRAMES__"));
    }
}
