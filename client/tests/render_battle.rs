//! 战斗画面（STAGE-37）的界面测试：三种尺寸（80×24、120×40、200×60）、`--unicode false` 降级、
//! 卡片的文字（序号、名字、等级、血量、状态标记、倒下）、倒计时、战斗记录和结果窗口。
//! STAGE-248：120×40 的日志区占 9 行后战场只剩 22 行，落到紧凑档；要测完整档写法的用例终端高度改用 44。
//! 战场直接用 `BattleState` 搭出来，不经过网络（网络路径见 `scenarios_battle.rs`）。

use std::time::{Duration, Instant};

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use ratatui::buffer::Buffer;
use ratatui::style::Color;
use stoneage_client::app::battle::{Act, Actor, BattleState, Phase, Pick, slots};
use stoneage_client::app::{App, Options, Screen, WorldState};
use stoneage_client::proto::{
    Appearance, BattleAllow, BattleEnd, BattleEvent, BattleRound, BattleStart, BattleTurn, BattleUnit, BattleUnitState,
    Character, ElementPoints, ExpGain, Location, Penalty, Progress, Rewards, TargetType, WorldEntry,
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

/// 一场进行中的战斗：我方勇者（45/60）和乌力，敌方两只野猪（一只中毒、血少）。
fn field_battle() -> BattleState {
    let mut hero = unit(0, "player", "勇者小明", 12, 75);
    hero.hp = Some(45);
    hero.max_hp = Some(60);
    hero.mp = Some(100);
    hero.max_mp = Some(100);
    hero.elements = ElementPoints { earth: 60, water: 40, fire: 0, wind: 0 };
    let mut pet = unit(5, "pet", "乌力", 5, 100);
    pet.hp = Some(35);
    pet.max_hp = Some(35);
    let mut boar = unit(15, "monster", "野猪", 3, 100);
    boar.elements = ElementPoints { earth: 100, water: 0, fire: 0, wind: 0 };
    // 等级和上面那只不等，测试才能用 `Lv.x 野猪` 唯一定位到某一只。
    let mut hurt = unit(16, "monster", "野猪", 4, 40);
    hurt.status = vec!["poison".into()];
    let start = BattleStart { party: None,
            field: None,
        battle_id: "b_1".into(),
        kind: "wild".into(),
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow { escape: true, capture: true },
        units: vec![hero, pet, boar, hurt],
        npc: None,
    };
    let mut b = BattleState::from_start(start);
    b.on_turn(
        BattleTurn {
                field: None,
            battle_id: "b_1".into(),
            turn: 1,
            deadline_ms: 1_030_000,
            server_time_ms: 1_000_000,
            needs: vec!["player".into(), "pet".into()],
            optional: vec![],
            progress: Progress { done: 0, total: 1 },
        },
        Instant::now(),
    );
    b
}

/// 进入世界并挂上战斗。
fn battle_app(unicode: bool, battle: BattleState) -> App {
    let mut a = app(unicode);
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 12,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 1000, map_name: "萨姆吉尔村".into() },
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

/// 每一行里要有的文字（找不到就打印整屏，方便看）。
fn assert_has(screen: &str, wants: &[&str]) {
    for want in wants {
        assert!(screen.contains(want), "缺少“{want}”\n{screen}");
    }
}

#[test]
fn full_layout_120x40_prioritizes_bodies_and_command_bar() {
    let mut a = battle_app(true, field_battle());
    let s = draw(&mut a, 120, 44);
    // 上边框：战场标题、倒计时；标签行换成顶部信息条（STAGE-106）：回合、阶段、自己和宠物的血量。
    assert_has(&s, &["战斗 · 野外", "倒计时", "30s", "战斗中", "第 1 回合 · 人物指令", "勇者小明 HP 45/60 MP 100/100", "乌力 HP 35/35"]);
    assert!(!s.contains("1快捷传送") && !s.contains("9百科"), "战斗中菜单标签不显示\n{s}");
    // 卡片：`Lv.xx 名字`（STAGE-173 起不再带「敌 / 我 + 编号」前缀）、血量（自己数字、对方百分比）、状态标记。
    assert_has(&s, &["Lv.12 勇者小明", "45/60", "Lv.5 乌力", "Lv.3 野猪", "40%", "毒"]);
    // 排的标签和右侧栏。
    assert_has(&s, &["敌左我右", "战斗日志", "/|\\", "<(oo)", "/^^\\"]);
    assert!(!s.contains("目标信息"), "战场不再挤出常驻侧栏");
    // 指令条：人物指令的七个动作。
    assert_has(&s, &["人物指令", "1 攻击", "2 精灵", "3 道具", "4 防御", "5 捕捉", "6 逃跑", "7 换宠"]);
}

#[test]
fn compact_layout_80x24_drops_side_panels_but_keeps_cards() {
    let mut a = battle_app(true, field_battle());
    let s = draw(&mut a, 80, 24);
    // 80 列的卡片只有 14 列：名字区 10 列，等级省掉点写成 `Lv3`；四个字的名字仍优先保住名字（等级让位）。
    assert_has(&s, &["战斗 · 野外", "倒计时", "Lv3 野猪", "Lv4野猪", "勇者小明", "^o^", "/|\\", "人物指令", "1 攻击"]);
    assert!(!s.contains("战斗记录"), "紧凑档没有右侧栏\n{s}");
    assert!(!s.contains("前排"), "紧凑档不写排的标签\n{s}");
}

/// STAGE-108：紧凑档相邻卡片之间留一列空格，血量文字和下一张卡片的血条不再粘在一起。
#[test]
fn compact_cards_do_not_touch_their_neighbours() {
    let mut a = battle_app(true, field_battle());
    let s = draw(&mut a, 80, 24);
    assert!(!s.contains("%█") && !s.contains("%░"), "血量和下一张卡片的血条粘在一起\n{s}");
}

/// STAGE-108：完整档卡片第 1 行放名字（120 列时卡片 14 列，名字能完整显示），状态标记在同一行的右边。
#[test]
fn full_card_puts_the_name_and_marks_on_one_row() {
    let mut a = battle_app(true, field_battle());
    let s = draw(&mut a, 120, 40);
    assert!(s.lines().any(|l| l.contains("勇者小明")), "{s}");
    assert!(s.lines().any(|l| l.contains("野猪") && l.contains("毒")), "野猪的名字和毒标记在同一行\n{s}");
}

#[test]
fn large_layout_200x60_still_shows_everything() {
    let mut a = battle_app(true, field_battle());
    let s = draw(&mut a, 200, 60);
    assert_has(&s, &["Lv.3 野猪", "Lv.12 勇者小明", "战斗日志", "人物指令"]);
}

#[test]
fn unicode_off_keeps_the_text_readable() {
    // 剩一半时间：条一半是 = 一半是 -。
    let mut b = field_battle();
    b.deadline = Some(Instant::now() + Duration::from_secs(15));
    let mut a = battle_app(false, b);
    let s = draw(&mut a, 120, 44);
    assert_has(&s, &["Lv.3 野猪", "Lv.12 勇者小明", "45/60", "人物指令", "倒计时"]);
    // 降级的倒计时用 = 和 -。
    assert!(s.contains("===") && s.contains("---"), "倒计时应退回细符号\n{s}");
}

#[test]
fn fainted_unit_is_marked_and_empty_slots_stay_blank() {
    let mut b = field_battle();
    b.units.get_mut(&15).unwrap().hp_pct = 0;
    let mut a = battle_app(true, b);
    let s = draw(&mut a, 120, 44);
    assert_has(&s, &["野猪 ✕"]);
    // 没有单位的格子不画卡片：只有 4 张卡片，`Lv.` 只由这四张的名字行给出。
    assert_eq!(s.matches("Lv.").count(), 4, "四张卡片各一个等级：{s}");
}

#[test]
fn records_show_the_latest_round_in_the_side_panel() {
    let t0 = Instant::now();
    let mut b = field_battle();
    let round = BattleRound {
        battle_id: "b_1".into(),
        turn: 1,
        events: vec![BattleEvent {
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
        }],
        state: vec![BattleUnitState { no: 15, hp_pct: 70, ..BattleUnitState::default() }],
    };
    b.on_round(round, t0);
    assert!(b.skip(), "跳过后回放完毕");
    let mut a = battle_app(true, b);
    let s = draw(&mut a, 180, 40);
    assert_has(&s, &["勇者小明 攻击 野猪 -23", "第 1 回合", "战斗日志"]);
}

#[test]
fn result_window_shows_rewards_and_closes_on_enter_or_typing() {
    let mut b = field_battle();
    b.show_result(BattleEnd { duel_point: None,
        battle_id: "b_1".into(),
        result: "win".into(),
        turn: 2,
        npc: None,
        rewards: Rewards {
            exp: vec![ExpGain {
                who: "player".into(),
                name: "勇者小明".into(),
                gain: 6,
                level_from: 1,
                level_to: 2,
                skill_points: 2,
                ..ExpGain::default()
            }],
            gold: 30,
            ..Rewards::default()
        },
        penalty: None,
    });
    let mut a = battle_app(true, b);
    let s = draw(&mut a, 120, 40);
    assert_has(&s, &["战斗胜利", "勇者小明  经验 +6", "Lv 1 → 2", "能力点 +2", "石币 +30", "[继续]"]);
    // 打字键关闭，并把这个字写进输入行：战斗结束，回到原来的菜单页，中间区域不再是战场。
    a.on_key(KeyEvent::new(KeyCode::Char('x'), KeyModifiers::NONE));
    assert!(a.world.as_ref().is_some_and(|w|w.battle.is_some()),"普通字母不关闭结果");
    a.on_key(KeyEvent::new(KeyCode::Enter, KeyModifiers::NONE));
    assert!(a.world.as_ref().is_some_and(|w| w.battle.is_none()));
    assert_eq!(a.world.as_ref().map(|w| w.input.text().to_string()).as_deref(), Some(""));
    let s = draw(&mut a, 120, 40);
    assert!(!s.contains("倒计时"), "{s}");
}

#[test]
fn lose_window_names_the_respawn_place() {
    let mut b = field_battle();
    b.show_result(BattleEnd { duel_point: None,
        result: "lose".into(),
        npc: None,
        penalty: Some(Penalty {
            respawn: Some(stoneage_client::proto::Position {
                floor: 1006,
                map_name: "萨姆吉尔村长之家".into(),
                x: 15,
                y: 22,
                direction: 4,
            }),
            in_place: false,
            gold_lost: 0,
            exp_lost: 0,
        }),
        ..BattleEnd::default()
    });
    let mut a = battle_app(true, b);
    let s = draw(&mut a, 120, 40);
    assert_has(&s, &["战斗失败", "被送回：萨姆吉尔村长之家"]);
}

#[test]
fn lose_window_says_revived_in_place_when_not_flown() {
    // STAGE-194：被打死原地复活，窗口写“原地复活，体力剩 1”，不写“被送回”。
    let mut b = field_battle();
    b.show_result(BattleEnd { duel_point: None,
        result: "lose".into(),
        npc: None,
        penalty: Some(Penalty {
            respawn: Some(stoneage_client::proto::Position {
                floor: 1006,
                map_name: "萨姆吉尔村长之家".into(),
                x: 15,
                y: 22,
                direction: 4,
            }),
            in_place: true,
            gold_lost: 0,
            exp_lost: 0,
        }),
        ..BattleEnd::default()
    });
    let mut a = battle_app(true, b);
    let s = draw(&mut a, 120, 40);
    assert_has(&s, &["战斗失败", "原地复活，体力剩 1"]);
    assert!(!s.contains("被送回"), "{s}");
}

#[test]
fn countdown_shows_seconds_left_rounded_up() {
    // 出招时限 30 秒，只剩 4.5 秒：向上取整显示 5s，条很短而且是红的。
    let mut b = field_battle();
    b.deadline = Some(Instant::now() + Duration::from_millis(4_500));
    let mut a = battle_app(true, b);
    let s = draw(&mut a, 120, 40);
    assert_has(&s, &[" 5s"]);
}

#[test]
fn waiting_bar_shows_progress_instead_of_actions() {
    // 已经交完指令、还在等别人：指令条写等待人数，不再列动作。
    let mut b = field_battle();
    b.phase = stoneage_client::app::battle::Phase::Waiting;
    b.progress = Progress { done: 1, total: 2 };
    let mut a = battle_app(true, b);
    let s = draw(&mut a, 120, 40);
    assert_has(&s, &["等待其他玩家 1/2"]);
    assert!(!s.contains("1 攻击"), "{s}");
}

#[test]
fn replay_bar_shows_progress_without_skip_keys() {
    let mut b = field_battle();
    b.on_round(BattleRound { turn: 1, ..BattleRound::default() }, Instant::now());
    let mut a = battle_app(true, b);
    let s = draw(&mut a, 120, 40);
    assert_has(&s, &["结算中"]);
    assert!(!s.contains("空格 跳过") && !s.contains("f 倍速"), "跳过与倍速只在系统页设：\n{s}");
}

#[test]
fn escape_and_draw_windows_say_so_in_one_line() {
    for (result, want) in [("escaped", "你逃离了战斗"), ("draw", "没有分出胜负")] {
        let mut b = field_battle();
        b.show_result(BattleEnd { duel_point: None, result: result.into(), ..BattleEnd::default() });
        let mut a = battle_app(true, b);
        let s = draw(&mut a, 120, 40);
        assert_has(&s, &[want, "[继续]"]);
    }
}

/// STAGE-167：同一站位同行；敌方在左，我方在右，中央保留交手空间。
#[test]
fn opposing_front_lines_share_a_row_with_a_clear_horizontal_gap() {
    for (width,height) in [(80,24),(120,44)] {
        let s = draw(&mut battle_app(true,field_battle()),width,height);
        let find = |needle: &str| s.lines().enumerate().find_map(|(row,line)| line.find(needle).map(|offset|(row,UnicodeWidthStr::width(&line[..offset])))).unwrap_or_else(||panic!("缺少 {needle}\n{s}"));
        // 紧凑档等级省掉点写成 `Lv3`，两档用各自的写法定位同一张卡片。
        let (enemy_pat, mine_pat) = if width == 80 { ("Lv3 野猪", "Lv5 乌力") } else { ("Lv.3 野猪", "Lv.5 乌力") };
        let enemy = find(enemy_pat); let mine = find(mine_pat);
        assert_eq!(enemy.0,mine.0,"双方同一个站位上下对齐\n{s}");
        assert!(enemy.1 < width as usize / 2 && mine.1 > width as usize / 2,"敌左我右\n{s}");
        assert!(mine.1-enemy.1>20,"中场留给移动与飞行物\n{s}");
        assert!(s.contains("战斗日志"));
    }
}

/// STAGE-108：没有右侧栏（80×24）时，选目标的编号、名字、等级和血量写在指令条里。
#[test]
fn compact_target_bar_names_the_target() {
    let mut b = field_battle();
    b.pick = Pick::Target { act: Act::Attack, extra: None, kind: TargetType::Enemy };
    b.cursor = slots::Cell { side: 1, front: true, col: 1 };
    let mut a = battle_app(true, b);
    let s = draw(&mut a, 80, 24);
    assert_has(&s, &["选目标  敌 2 野猪 Lv.4 血 40%", "只能选敌方"]);
}

/// 渲染到缓冲区（颜色测试用）。
fn draw_buffer(app: &mut App, w: u16, h: u16) -> Buffer {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
    term.backend().buffer().clone()
}

/// 标题里以 `label`（如“我 8”）出现的那一段文字的前景色；找不到返回 None。
/// 宽字符占两格，后面那一格跳过（和 `draw` 的读法一致）。
fn label_fg(buf: &Buffer, label: &str) -> Option<Color> {
    let want: Vec<String> = label.chars().map(|c| c.to_string()).collect();
    for y in 0..buf.area.height {
        let mut row: Vec<(String, Color)> = vec![];
        let mut x = 0;
        while x < buf.area.width {
            let sym = buf[(x, y)].symbol();
            row.push((sym.to_string(), buf[(x, y)].fg));
            x += UnicodeWidthStr::width(sym).max(1) as u16;
        }
        if let Some(i) = row.windows(want.len()).position(|w| w.iter().zip(&want).all(|((sym, _), t)| sym == t)) {
            return Some(row[i].1);
        }
    }
    None
}

/// STAGE-106：正在下指令的单位青色加粗：人物指令是自己，宠物指令是出战宠物。
/// STAGE-173 起卡片名字行是 `Lv.xx 名字`，用「等级 + 名字」定位，避开顶部信息条里的同名文字。
#[test]
fn commanding_unit_is_cyan_in_player_and_pet_phases() {
    let mut a = battle_app(true, field_battle());
    let buf = draw_buffer(&mut a, 120, 44);
    assert_eq!(label_fg(&buf, "Lv.12 勇者小明"), Some(Color::Cyan), "人物指令时自己青色");
    assert_ne!(label_fg(&buf, "Lv.5 乌力"), Some(Color::Cyan), "宠物还没轮到");

    let mut b = field_battle();
    b.phase = Phase::Input(Actor::Pet);
    let mut a = battle_app(true, b);
    let buf = draw_buffer(&mut a, 120, 44);
    assert_eq!(label_fg(&buf, "Lv.5 乌力"), Some(Color::Cyan), "宠物指令时出战宠物青色");
    assert_ne!(label_fg(&buf, "Lv.12 勇者小明"), Some(Color::Cyan), "人物已经交过了");
}

/// STAGE-106：选目标时焦点是黄色，正在下指令的单位保持青色，两者一眼能分开。
#[test]
fn target_focus_is_yellow_while_the_commander_stays_cyan() {
    let mut b = field_battle();
    b.pick = Pick::Target { act: Act::Attack, extra: None, kind: TargetType::Enemy };
    b.cursor = slots::cell_of(15);
    let mut a = battle_app(true, b);
    let buf = draw_buffer(&mut a, 120, 44);
    assert_eq!(label_fg(&buf, "Lv.3 野猪"), Some(Color::Yellow), "焦点落在敌方的野猪上");
    assert_eq!(label_fg(&buf, "Lv.12 勇者小明"), Some(Color::Cyan), "下指令的人仍是青色");
}

/// STAGE-106：等别人时顶部信息条写等待人数，托管中在信息条里标出（不再放在菜单栏右侧）。
#[test]
fn top_bar_shows_waiting_progress_and_auto_mark() {
    let mut b = field_battle();
    b.phase = Phase::Waiting;
    b.progress = Progress { done: 1, total: 2 };
    let mut a = battle_app(true, b);
    a.world.as_mut().unwrap().auto_battle = true;
    let s = draw(&mut a, 120, 40);
    assert_has(&s, &["第 1 回合 · 等待 1/2", "勇者小明 HP 45/60 MP 100/100", "自动挂机中"]);
}

/// STAGE-106：战斗结束、关闭窗口后（战斗状态清掉），菜单标签回来。
#[test]
fn menu_tabs_come_back_after_the_battle() {
    let mut a = battle_app(true, field_battle());
    a.world.as_mut().unwrap().battle = None;
    let s = draw(&mut a, 120, 40);
    // STAGE-131：战斗结束回到战前那一层（这里是地图页），顶部显示导航路径而不是常驻的菜单标签。
    assert_has(&s, &["场景 › 快捷传送"]);
    // STAGE-248：战斗记录已经写进消息日志，底部消息栏里会有“第 1 回合”的文字，所以只看战斗的顶栏和标记。
    assert!(!s.contains("战斗中") && !s.contains("第 1 回合 · 人物指令"), "{s}");
}

/// STAGE-109：宠物指令阶段能退回人物指令时，指令条写“Esc 改人物指令”；退回重选时标题写“人物指令（改）”，
/// 不能退回时（比如宠物指令已经交上）不写 Esc。
#[test]
fn pet_bar_offers_esc_only_while_the_player_command_can_be_revised() {
    use stoneage_client::app::battle::{Actor, Phase};
    let mut b = field_battle();
    b.phase = Phase::Input(Actor::Player);
    b.on_accepted("pet");
    let mut a = battle_app(true, b);
    let s = draw(&mut a, 120, 40);
    assert_has(&s, &["宠物指令", "Esc 改人物指令"]);
    let battle = a.world.as_mut().unwrap().battle.as_mut().unwrap();
    battle.revert_to_player();
    let s = draw(&mut a, 120, 40);
    assert_has(&s, &["人物指令（改）"]);
    assert!(!s.contains("Esc 改人物指令"), "{s}");
    let mut b = field_battle();
    b.phase = Phase::Input(Actor::Player);
    b.on_accepted("wait");
    b.phase = Phase::Input(Actor::Pet);
    let mut a = battle_app(true, b);
    let s = draw(&mut a, 120, 40);
    assert!(!s.contains("Esc 改人物指令"), "不能退回时不写 Esc：{s}");
}

/// STAGE-167：身体不是状态卡，二十个单位必须各有一个完整轮廓和编号。
#[test]
fn twenty_units_keep_separate_bodies_at_normal_and_wide_widths() {
    for (width, height) in [(80, 24), (110, 24), (120, 24), (200, 24), (120, 44), (200, 60)] {
        let units = (0..20).map(|no| unit(no, if no % 10 < 5 { "player" } else { "pet" }, "单位", 3, 100)).collect();
        let b = BattleState::from_start(BattleStart {you: 0, units, ..Default::default()});
        let s = draw(&mut battle_app(true, b), width, height);
        assert_eq!(s.matches("/|\\").count(), 10, "十个人形身体必须保留\n{s}");
        assert_eq!(s.matches(if height == 24 { "oo>" } else { "(oo)>" }).count(), 5, "敌方宠物朝右\n{s}");
        assert_eq!(s.matches(if height == 24 { "<oo" } else { "<(oo)" }).count(), 5, "我方宠物朝左\n{s}");
        // STAGE-173：名字行不再带编号，二十个单位的名字与等级各出现一次。
        let name = if height == 24 { "Lv3 单位" } else { "Lv.3 单位" };
        assert_eq!(s.matches(name).count(), 20, "二十个单位都要有名字与等级\n{s}");
    }
}

/// STAGE-153：挂机中的指令框在 120×40 下首行必须完整（黑盒截屏曾见“Tab 设”和边框错位，
/// 渲染层面确认没有截断；截屏错位多半是终端把「·」按歧义宽度算成两列，见 docs/battle-ui.md）。
#[test]
fn auto_command_bar_first_line_is_whole_at_120x40() {
    let mut a = battle_app(true, field_battle());
    a.world.as_mut().unwrap().auto_battle = true;
    let s = draw(&mut a, 120, 40);
    assert_has(&s, &[" 自动挂机中 · 准备出招 · 8 停止挂机 · 7 停止遇敌 · Tab 设置 ", "要手动打：先按 8 停止，这一场继续手动打完"]);
    // 指令框首行所在的那一行，右边框（同行的战斗日志框）要完整收尾。
    let row = s.lines().find(|l| l.contains("准备出招")).unwrap();
    assert!(row.trim_end().ends_with('│'), "指令框首行右侧边框完整：{row}");
}
