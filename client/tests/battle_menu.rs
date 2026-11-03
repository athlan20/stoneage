use crossterm::event::{KeyCode,KeyEvent,KeyModifiers};
use ratatui::{Terminal,backend::TestBackend};
use stoneage_client::{app::{App,Options,WorldState,Screen,Layer,battle::{BattleState,Phase,Actor}},proto::{WorldEntry,BattleStart,BattleEnd}};
use unicode_width::UnicodeWidthStr;

fn app()->App{
    let mut a=App::new(Options::default());let mut w=WorldState::new(WorldEntry::default());
    let mut b=BattleState::from_start(BattleStart::default());b.phase=Phase::Input(Actor::Player);w.battle=Some(b);
    a.world=Some(w);a.screen=Screen::World;a
}
fn key(a:&mut App,k:KeyCode){a.on_key(KeyEvent::new(k,KeyModifiers::NONE));}

#[test]
fn ordinary_characters_do_not_type_or_close_results(){
    let mut a=app();key(&mut a,KeyCode::Char('x'));assert!(a.world.as_ref().unwrap().input.is_empty());
    a.world.as_mut().unwrap().battle.as_mut().unwrap().show_result(BattleEnd{result:"win".into(),..Default::default()});
    key(&mut a,KeyCode::Char('a'));assert!(a.world.as_ref().unwrap().battle.is_some());
    key(&mut a,KeyCode::Enter);assert!(a.world.as_ref().unwrap().battle.is_none());
}

#[test]
fn closing_command_draft_restores_menu_input(){
    let mut a=app();key(&mut a,KeyCode::Char('/'));key(&mut a,KeyCode::Char('a'));key(&mut a,KeyCode::Esc);
    assert_eq!(a.world.as_ref().unwrap().input.text(),"a");
    assert!(!a.world.as_ref().unwrap().nav.is(&Layer::Command));
    // 输入行空出来以后方向键回到战斗网格：攻击下面一格是第二行第一列的「捕捉」（STAGE-153）。
    key(&mut a,KeyCode::Down);assert_eq!(a.world.as_ref().unwrap().battle.as_ref().unwrap().action_focus,4);
}

#[test]
fn all_seven_actions_fit_minimum_and_wide_terminals(){
    for unicode in [true,false]{for (width,height) in [(80,24),(110,30),(120,40),(200,60)]{
        let mut a=app();a.unicode=unicode;
        let mut t=Terminal::new(TestBackend::new(width,height)).unwrap();t.draw(|f|stoneage_client::ui::render(f,&mut a)).unwrap();
        let b=t.backend().buffer();let mut text=String::new();
        for y in 0..height{let mut x=0;while x<width{let s=b[(x,y)].symbol();text.push_str(s);x+=UnicodeWidthStr::width(s).max(1) as u16;}}
        for label in ["攻击","精灵","道具","防御","捕捉","逃跑","换宠","开始自动挂机"]{assert!(text.contains(label),"{width}x{height} 缺少 {label}: {text}");}
        assert!(text.contains("Tab 控制"),"{width}x{height} 提示行没写面板入口: {text}");
    }}
}

/// STAGE-153：动作层画成两行，挂机管理项在第二行末尾；提示行一直写着 Tab 控制面板入口。
#[test]
fn action_grid_has_two_rows_and_the_auto_item() {
    let mut a = app();
    let mut t = Terminal::new(TestBackend::new(120, 40)).unwrap();
    t.draw(|f| stoneage_client::ui::render(f, &mut a)).unwrap();
    let b = t.backend().buffer();
    let mut lines = Vec::new();
    for y in 0..40 {
        let mut row = String::new();
        let mut x = 0;
        while x < 120 {
            let s = b[(x, y)].symbol();
            row.push_str(s);
            x += UnicodeWidthStr::width(s).max(1) as u16;
        }
        lines.push(row);
    }
    assert!(
        lines.iter().any(|l| l.contains("1 攻击") && l.contains("4 防御")),
        "第一行是四个动作：{lines:?}"
    );
    let second = lines.iter().find(|l| l.contains("5 捕捉")).expect("第二行有捕捉");
    assert!(second.contains("8 开始自动挂机"), "第二行末尾是挂机管理项：{second}");
    assert!(lines.iter().any(|l| l.contains("Tab 控制")), "提示行写着面板入口：{lines:?}");
}

/// STAGE-153：正常玩家在战斗中按 Tab 打开战斗控制面板，默认落在挂机项；开始挂机后关面板会重新安排出招。
#[test]
fn tab_opens_the_battle_control_panel() {
    let mut a = app();
    key(&mut a, KeyCode::Tab);
    assert!(a.world.as_ref().unwrap().system.battle_open, "Tab 打开面板");
    assert_eq!(a.world.as_ref().unwrap().system.index, 6, "默认落在自动挂机项");
    key(&mut a, KeyCode::Enter);
    assert!(a.world.as_ref().unwrap().auto_battle, "面板里能开始挂机");
    // 面板标题和行文本：动态挂机项 + 停止自动遇敌。
    let mut t = Terminal::new(TestBackend::new(120, 40)).unwrap();
    t.draw(|f| stoneage_client::ui::render(f, &mut a)).unwrap();
    let b = t.backend().buffer();
    let mut text = String::new();
    for y in 0..40 {
        let mut x = 0;
        while x < 120 {
            let s = b[(x, y)].symbol();
            text.push_str(s);
            x += UnicodeWidthStr::width(s).max(1) as u16;
        }
        text.push('\n');
    }
    assert!(text.contains("战斗控制"), "面板标题：{text}");
    assert!(text.contains("自动挂机：开"), "开着时显示开：{text}");
    assert!(text.contains("自动遇敌：关"), "遇敌没开时显示关：{text}");
    assert!(!text.contains("停止：") && !text.contains("开始自动挂机"), "动作名和状态分开写：{text}");
    key(&mut a, KeyCode::Esc);
    assert!(!a.world.as_ref().unwrap().system.battle_open, "Esc 关面板");
    let b = a.world.as_ref().unwrap().battle.as_ref().unwrap();
    assert!(b.auto_due.is_some(), "关面板时仍开着挂机，按延迟重新安排出招");
}
