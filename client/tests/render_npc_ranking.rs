//! 强者表排行窗口（STAGE-308，`docs/protocol.md` 5.75）：窗口正文是带黄色表头的表格（名次、决斗点数、等级、名字），
//! 自己那行名字前有 ★；表上方是页码和自己的名次，选项在表下面。

use ratatui::Terminal;
use ratatui::backend::TestBackend;
use ratatui::style::Color;
use stoneage_client::app::{App, Layer, Options, Screen, WorldState};
use stoneage_client::proto::{
    Appearance, Character, Location, NpcOption, NpcRankMe, NpcRankRow, NpcRanking, NpcWindow, WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

/// `window_id` 非空是服务端推的真实窗口（菜单式按键）；空串是旧式数字选择窗口。
fn app_with_ranking(window_id: &str) -> App {
    let mut a = App::new(Options { server: "127.0.0.1:7065".into(), unicode: true, ..Options::default() });
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 1,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 130, map_name: "竞技场".into() },
    };
    let mut world = WorldState::new(entry);
    world.nav.push(Layer::Page(0));
    world.npc_window = Some(NpcWindow {
        npc: 1802,
        window_id: window_id.into(),
        name: "强者表".into(),
        type_: "强者表".into(),
        text: "世界的强者们".into(),
        options: vec![NpcOption { no: 1, label: "下一页".into() }],
        ranking: Some(NpcRanking {
            top: vec![
                NpcRankRow { rank: 1, name: "勇者小明".into(), level: 80, duel_point: 2310, is_self: true },
                NpcRankRow { rank: 2, name: "路人乙".into(), level: 60, duel_point: 2000, is_self: false },
            ],
            page: 1,
            pages: 7,
            me: NpcRankMe { rank: 1, duel_point: 2310 },
        }),
        ..Default::default()
    });
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

fn draw(app: &mut App, w: u16, h: u16) -> (String, ratatui::buffer::Buffer) {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
    let buf = term.backend().buffer().clone();
    let text = (0..h)
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
        .join("\n");
    (text, buf)
}

#[test]
fn ranking_window_shows_table_with_self_marked() {
    let mut a = app_with_ranking("w-rank-1");
    let (s, _) = draw(&mut a, 100, 30);
    assert!(s.contains("名次"), "表头缺名次：\n{s}");
    assert!(s.contains("决斗点数"), "表头缺决斗点数：\n{s}");
    assert!(s.contains("★勇者小明"), "自己那行没有星标：\n{s}");
    assert!(s.contains("路人乙"), "表体缺第二名：\n{s}");
    assert!(s.contains("第 1/7 页"), "缺页码：\n{s}");
    assert!(s.contains("我的名次 1"), "缺自己的名次：\n{s}");
    assert!(s.contains("1. 下一页"), "缺翻页选项：\n{s}");
}

#[test]
fn ranking_header_is_yellow() {
    let mut a = app_with_ranking("w-rank-1");
    let (s, buf) = draw(&mut a, 100, 30);
    // 表头行有“等级”和“名字”（页码行也有“名次”“决斗点数”，不能只找它们）。
    let row = s.lines().position(|l| l.contains("等级") && l.contains("名字")).expect("找不到表头行");
    // 按列数逐格找“名”所在的格（宽字符占两列，buffer 的 x 与字符宽度一致）。
    let mut found = None;
    for x in 0..100u16 {
        if buf[(x, row as u16)].symbol() == "名" {
            found = Some(x);
            break;
        }
    }
    let x = found.expect("表头行里找不到“名”");
    assert_eq!(buf[(x, row as u16)].fg, Color::Yellow, "表头应为黄色，行：{:?}", s.lines().nth(row));
}

/// 回归：服务端推的排行窗口都带 window_id，曾因先判 window_id 走了通用菜单，表格画不出来。
#[test]
fn ranking_window_with_window_id_still_shows_table() {
    let mut a = app_with_ranking("w-rank-1");
    let (s, _) = draw(&mut a, 100, 30);
    assert!(s.contains("决斗点数") && s.contains("★勇者小明"), "带 window_id 的窗口没画出表格：\n{s}");
    assert!(s.contains("> 1. 下一页"), "菜单式窗口缺选中高亮：\n{s}");
    assert!(s.contains("↑↓ 选择"), "菜单式窗口的提示行不对：\n{s}");
}

#[test]
fn ranking_window_without_window_id_uses_digit_hint() {
    let mut a = app_with_ranking("");
    let (s, _) = draw(&mut a, 100, 30);
    assert!(s.contains("决斗点数") && s.contains("★勇者小明"), "旧式窗口没画出表格：\n{s}");
    assert!(s.contains("数字选择"), "旧式窗口的提示行不对：\n{s}");
    assert!(!s.contains("> 1. 下一页"), "旧式窗口不该有选中高亮：\n{s}");
}
