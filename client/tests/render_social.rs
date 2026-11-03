//! 社交页「队伍」栏（STAGE-162，`docs/ui.md`「社交」）的界面和按键测试：没有队伍的入口与规则说明、
//! 成员表（序号、队长/分身标记、HP 迷你条、出战宠物）、宽屏并排详情、窄屏不并排、Unicode 关闭时退回 ASCII、
//! 动作菜单、详情层、解散确认默认取消。不连服务端：只看本地状态和画面。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{App, Kind, Layer, Options, PartyStep, SOCIAL_MENU, Screen, WorldState};
use stoneage_client::proto::{
    AbilityPoints, Appearance, Character, Location, Party, PartyDerived, PartyEquipment, PartyMember, PartyPet,
    WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn draw(a: &mut App, w: u16, h: u16) -> String {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, a)).unwrap();
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

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn text(a: &mut App, s: &str) {
    if a.world.as_ref().is_some_and(|w| !w.text_entry_active()) {
        key(a, KeyCode::Char('/'));
    }
    for c in s.chars() {
        key(a, KeyCode::Char(c));
    }
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().unwrap()
}

fn member(index: u8) -> PartyMember {
    PartyMember {
        index,
        name: if index == 0 {
            "勇者小明".into()
        } else {
            format!("勇者小明·分身{index}")
        },
        leader: index == 0,
        clone: index > 0,
        level: 5,
        hp: 44,
        max_hp: 44,
        mp: 100,
        max_mp: 100,
        pet: Some(PartyPet {
            name: "乌力".into(),
            level: 5,
            hp: 37,
            max_hp: 37,
            skills: vec!["攻击".into()],
        }),
        stats: AbilityPoints {
            vital: 8,
            strength: 4,
            toughness: 4,
            dexterity: 4,
        },
        derived: PartyDerived {
            attack: 5,
            defense: 5,
            agility: 4,
        },
        equipment: vec![PartyEquipment {
            slot: 2,
            name: "小木棒".into(),
        }],
        ..Default::default()
    }
}

fn full_party() -> Party {
    Party {
        kind: "clone".into(),
        leader: "勇者小明".into(),
        members: (0..5).map(member).collect(),
    }
}

/// 进入世界并切到「6 社交」页。
fn social(unicode: bool, party: Option<Party>, hp: u32) -> App {
    let mut a = App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode,
        ..Options::default()
    });
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 5,
        appearance: Appearance {
            body: 3,
            color: 2,
            eye: 1,
            mouth: 4,
        },
        location: Location {
            floor: 1006,
            map_name: "萨姆吉尔的村长家".into(),
        },
    };
    entry.status.hp = hp;
    entry.status.max_hp = 44;
    entry.status.level = 5;
    entry.party = party;
    let mut w = WorldState::new(entry);
    w.nav.push(Layer::Page(0));
    a.world = Some(w);
    a.screen = Screen::World;
    key(&mut a, KeyCode::Char('6'));
    assert_eq!(world(&a).menu, SOCIAL_MENU);
    a
}

#[test]
fn empty_page_shows_the_entry_and_the_rules() {
    let mut a = social(true, None, 44);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("还没有队伍") && s.contains("组成分身队"), "{s}");
    assert!(
        s.contains("不领奖励") && s.contains("全部消失"),
        "规则说明要写出不领奖励和消失条件：\n{s}"
    );
    assert!(!s.contains("未开放") && !s.contains("尚未实现"), "{s}");
}

#[test]
fn entry_is_greyed_with_the_reason_when_the_leader_is_down() {
    let mut a = social(true, None, 0);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("角色已经倒下，不能组成分身队"), "{s}");
}

#[test]
fn table_lists_the_leader_and_four_clones_with_hp_bars() {
    let mut a = social(true, Some(full_party()), 44);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("队伍 · 1 队长 + 4 分身"), "{s}");
    for n in 1..=4 {
        assert!(s.contains(&format!("勇者小明·分身{n}")), "缺少分身 {n}：\n{s}");
    }
    assert!(s.contains("[队长]") && s.contains("[分身]"), "{s}");
    assert!(s.contains("██████ 44/44"), "HP 迷你条：\n{s}");
    assert!(s.contains("出战宠物") && s.contains("乌力 Lv5 37/37"), "{s}");
    // 顶部状态行右侧有“队伍 5/5”。
    let top = s.lines().nth(1).unwrap();
    assert!(top.contains("队伍 5/5"), "顶部状态行：{top}");
}

#[test]
fn ascii_fallback_without_unicode() {
    let mut a = social(false, Some(full_party()), 44);
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("###### 44/44") && !s.contains('█'), "{s}");
}

#[test]
fn wide_shows_the_detail_beside_the_table_and_narrow_does_not() {
    let mut a = social(true, Some(full_party()), 44);
    let wide = draw(&mut a, 140, 30);
    assert!(
        wide.contains("能力") && wide.contains("衍生") && wide.contains("体力 8"),
        "宽屏并排详情：\n{wide}"
    );
    let narrow = draw(&mut a, 80, 24);
    assert!(!narrow.contains("衍生"), "窄屏详情不并排：\n{narrow}");
}

#[test]
fn enter_opens_actions_and_detail_is_a_layer_that_escape_closes() {
    let mut a = social(true, Some(full_party()), 44);
    // 选到第二行（分身 1）再 Enter。
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    assert_eq!(world(&a).party_step, Some(PartyStep::Actions { member: 1, index: 0 }));
    assert!(world(&a).nav.is(&Layer::Action));
    let s = draw(&mut a, 80, 24);
    assert!(
        s.contains("动作 · 勇者小明·分身1") && s.contains("查看详情") && s.contains("解散队伍"),
        "{s}"
    );
    assert!(
        !s.contains("私聊") && !s.contains("交易") && !s.contains("邀请"),
        "对分身不出现私聊、交易、邀请：\n{s}"
    );
    // 查看详情压一层 Layer::Detail，整页显示。
    key(&mut a, KeyCode::Enter);
    assert!(world(&a).nav.is(&Layer::Detail));
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("勇者小明·分身1 · 分身") && s.contains("创建时的快照"), "{s}");
    assert!(
        s.contains("场景 › ") && s.contains("社交 › 动作 › 详情"),
        "路径条：\n{s}"
    );
    // Esc 回动作菜单，再 Esc 回社交页，选中项还是分身 1。
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).nav.is(&Layer::Action));
    key(&mut a, KeyCode::Esc);
    assert!(world(&a).nav.is(&Layer::Page(SOCIAL_MENU)) && world(&a).party_step.is_none());
    assert_eq!(world(&a).selected_party_index(), Some(1));
}

#[test]
fn disband_confirmation_defaults_to_cancel_and_sends_nothing() {
    let mut a = social(true, Some(full_party()), 44);
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Down); // 解散队伍
    key(&mut a, KeyCode::Enter);
    let w = world(&a);
    assert!(
        w.confirm.is_some() && w.confirm_native && w.party_step.is_none(),
        "进入确认层并关掉动作菜单"
    );
    let s = draw(&mut a, 80, 24);
    assert!(
        s.contains("解散队伍？4 个分身会立即消失") && s.contains("[取消]"),
        "默认停在取消：\n{s}"
    );
    key(&mut a, KeyCode::Enter);
    assert!(world(&a).confirm.is_none() && world(&a).party().is_some(), "取消不解散");
    assert!(a.log.all().iter().any(|m| m.text == "已取消解散"));
    assert!(!a.session.busy(), "取消不发请求");
}

#[test]
fn unknown_party_arguments_report_usage_in_the_command_bar() {
    let mut a = social(true, Some(full_party()), 44);
    text(&mut a, "party foo");
    key(&mut a, KeyCode::Enter);
    let last = a
        .log
        .all()
        .iter()
        .rev()
        .find(|m| m.kind == Kind::Error)
        .map(|m| m.text.clone())
        .unwrap_or_default();
    assert!(last.contains("用法：party"), "{last}");
    assert!(world(&a).nav.is(&Layer::Command), "未知参数留在命令栏内");
}

#[test]
fn commands_explain_why_they_do_not_run_without_sending() {
    // 已有队伍：clone 被拦。
    let mut a = social(true, Some(full_party()), 44);
    text(&mut a, "party clone");
    key(&mut a, KeyCode::Enter);
    assert!(a.log.all().iter().any(|m| m.text == "已经有队伍了，先解散再组"));
    assert!(!a.session.busy());
    // 没有队伍：disband 被拦。
    let mut a = social(true, None, 44);
    text(&mut a, "party disband");
    key(&mut a, KeyCode::Enter);
    assert!(a.log.all().iter().any(|m| m.text == "没有队伍可解散"));
    assert!(!a.session.busy());
    // 倒下：clone 被拦；Enter 同样不发请求。
    let mut a = social(true, None, 0);
    key(&mut a, KeyCode::Enter);
    assert!(a.log.all().iter().any(|m| m.text == "角色已经倒下，不能组成分身队"));
    assert!(!a.session.busy());
}

#[test]
fn observer_cannot_clone_or_disband_but_can_read_the_detail() {
    let mut a = social(true, Some(full_party()), 44);
    a.world.as_mut().unwrap().entry.control.you = "observer".into();
    key(&mut a, KeyCode::Enter);
    key(&mut a, KeyCode::Down);
    let s = draw(&mut a, 80, 24);
    assert!(
        s.contains("（agent 正在操控，输入 take 收回控制权）"),
        "解散保留显示并解释原因：\n{s}"
    );
    key(&mut a, KeyCode::Enter);
    assert!(
        a.log
            .all()
            .iter()
            .any(|m| m.kind == Kind::Error && m.text.contains("agent 正在操控"))
    );
    assert!(world(&a).confirm.is_none() && !a.session.busy());
    // 看详情不受限制。
    key(&mut a, KeyCode::Up);
    key(&mut a, KeyCode::Enter);
    assert!(world(&a).nav.is(&Layer::Detail));
}

#[test]
fn game_menu_no_longer_says_social_is_closed() {
    let mut a = social(true, Some(full_party()), 44);
    key(&mut a, KeyCode::Esc); // 社交页 → 场景
    key(&mut a, KeyCode::Esc); // 场景 → 游戏菜单
    assert!(world(&a).at_menu());
    let s = draw(&mut a, 80, 24);
    assert!(s.contains("社交") && !s.contains("未开放"), "{s}");
}
