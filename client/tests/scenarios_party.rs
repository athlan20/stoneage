//! STAGE-162 验收（场景）：模拟服务端按 `ui_party*` 用例回放，客户端在社交页和命令栏里组队、解散，
//! 战斗里分身同场（带“自动”标记），agent 操控时的拦截。严格模式下客户端发出的请求要与用例逐字段一致；
//! “没有发出请求”由用例里没有对应的步骤保证（多发一条请求，模拟服务端就报不符）。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Down, Enter, Esc, Right, Tab, Up};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::{Actor, BattleState, Phase};
use stoneage_client::app::{App, Kind, Layer, PartyStep, SOCIAL_MENU, Screen};
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

/// 登录、选第一个角色、进入世界（落在场景层）。
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

/// 场景 → Esc 打开游戏菜单 → 选「社交」（第 6 项）→ Enter。
fn open_social(h: &mut Harness) {
    // 先退回场景（`token new` 之类的命令会把页面切走）。
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

fn party_size(a: &App) -> Option<usize> {
    a.world.as_ref()?.party().map(|p| p.size())
}

fn has_log(a: &App, kind: Kind, text: &str) -> bool {
    a.log.all().iter().any(|m| m.kind == kind && m.text.contains(text))
}

/// 社交页没有队伍：Enter 直接组成分身队；传送后成员位置同步；动作菜单解散先确认（默认取消不发请求）；
/// `/party clone` 再组，`/party disband` 取消一次，`/party disband yes` 直接解散。
#[test]
fn clone_move_and_disband_from_the_social_page() {
    let mut h = enter_world("ui_party");
    open_social(&mut h);
    let s = draw(&mut h, 80, 24);
    assert!(s.contains("组成分身队") && s.contains("还没有队伍"), "{s}");

    h.key(Enter);
    h.wait("队伍出现", 5, |a| party_size(a) == Some(5));
    assert!(
        has_log(&h.app, Kind::Party, "组成分身队：你是队长，带着 4 个分身（共 5 人）"),
        "{:?}",
        h.app.log.all()
    );
    let s = draw(&mut h, 80, 24);
    assert!(
        s.contains("队伍 5/5") && s.contains("勇者小明·分身4") && s.contains("队伍 · 1 队长 + 4 分身"),
        "{s}"
    );

    // 传送：位置推送之后紧跟 moved，成员都在队长的新位置上。
    h.command("warp 1");
    h.wait("队伍同步到新位置", 5, |a| {
        a.world
            .as_ref()
            .and_then(|w| w.party())
            .is_some_and(|p| p.members.iter().all(|m| m.position.floor == 1000))
    });

    // 动作菜单 → 解散队伍 → 确认默认取消：不发请求，队伍还在。
    h.keys(&[Enter, Down, Enter]);
    assert!(h.app.world.as_ref().unwrap().confirm.is_some());
    h.key(Enter);
    assert_eq!(party_size(&h.app), Some(5));
    assert!(has_log(&h.app, Kind::System, "已取消解散"));
    // 再来一次，这次选确认（→ 切到确认，Enter）：发出 party_disband。
    h.keys(&[Enter, Down, Enter, Right, Enter]);
    h.wait("队伍消失", 5, |a| party_size(a).is_none());
    assert!(has_log(&h.app, Kind::Party, "队伍已解散，分身消失"));
    assert!(h.app.world.as_ref().unwrap().party_step.is_none());
    let s = draw(&mut h, 80, 24);
    assert!(s.contains("还没有队伍") && !s.contains("队伍 5/5"), "{s}");

    // 命令：clone、取消一次 disband、disband yes。
    h.command("party clone");
    h.wait("再次组队", 5, |a| party_size(a) == Some(5));
    h.command("party disband");
    assert!(h.app.world.as_ref().unwrap().confirm.is_some());
    h.key(Esc);
    assert!(h.app.world.as_ref().unwrap().confirm.is_none() && party_size(&h.app) == Some(5));
    h.command("party disband yes");
    h.wait("再次解散", 5, |a| party_size(a).is_none());
    h.finish();
}

/// STAGE-189：遇敌开着时组分身队，不关遇敌，`party_clone` 照常成功，遇敌开关保持开着。
#[test]
fn clone_keeps_encounter_on() {
    let mut h = enter_world("ui_party_encounter");
    h.command("encounter on");
    h.wait("遇敌开着", 5, |a| a.world.as_ref().is_some_and(|w| w.entry.encounter));
    h.command("party clone");
    h.wait("队伍出现", 5, |a| party_size(a) == Some(5));
    assert!(h.app.world.as_ref().unwrap().entry.encounter, "组队不应关掉遇敌");
    assert!(has_log(&h.app, Kind::Party, "组成分身队：你是队长，带着 4 个分身（共 5 人）"));
    h.finish();
}

fn battle(a: &App) -> Option<&BattleState> {
    a.world.as_ref()?.battle.as_ref()
}

fn input(a: &App, turn: u32, actor: Actor) -> bool {
    battle(a).is_some_and(|b| b.turn == turn && b.phase == Phase::Input(actor))
}

/// 带着分身队开战：战场上己方 5 个人物 + 5 只宠物，分身带自动标记；队长只交两条指令就结算；
/// 结果窗口注明分身不领奖励；战后队伍还在。
#[test]
fn clones_fight_automatically_and_do_not_get_rewards() {
    let mut h = enter_world("ui_party_battle");
    open_social(&mut h);
    h.key(Enter); // 组成分身队
    h.wait("队伍出现", 5, |a| party_size(a) == Some(5));
    h.wait("第 1 回合轮到人物出招", 5, |a| input(a, 1, Actor::Player));
    {
        let b = battle(&h.app).unwrap();
        assert_eq!(b.units.len(), 12, "己方 5 人 + 5 宠，敌方 2 只");
        assert_eq!(b.units.values().filter(|u| u.auto).count(), 8);
        assert!(b.has_clones());
        assert!(!b.units[&0].auto && !b.units[&5].auto, "队长和队长的宠物不是自动单位");
        assert!(b.units[&1].auto && b.units[&6].auto && b.units[&9].auto);
    }
    // STAGE-167 角色舞台：八个分身人宠在紧凑档均带“自”，完整档均带“自动”。
    let s = draw(&mut h, 80, 24);
    assert_eq!(s.matches(" 自").count(), 8, "八个分身人宠均保留标记：\n{s}");
    let s = draw(&mut h, 120, 50);
    assert_eq!(s.matches(" 自动").count(), 8, "完整档八个分身人宠均保留自动标记：\n{s}");
    // 队长出招：人物攻击敌方 3 号（15 号），宠物同一目标；不用等分身。
    h.keys(&[Char('1'), Char('3')]);
    h.wait("宠物轮到出招", 5, |a| input(a, 1, Actor::Pet));
    h.keys(&[Char('1'), Char('3')]);
    h.wait("结果窗口弹出", 10, |a| {
        battle(a).is_some_and(|b| b.result.is_some())
    });
    let s = draw(&mut h, 100, 40);
    assert!(s.contains("战斗胜利") && s.contains("分身不领奖励"), "{s}");
    assert!(!s.contains("勇者小明·分身1  经验"), "分身没有经验行：\n{s}");
    h.key(Enter);
    h.wait("战后队伍状态", 5, |a| {
        has_log(a, Kind::Party, "战斗结束，分身已恢复满血")
    });
    assert_eq!(party_size(&h.app), Some(5), "战后队伍还在");
    h.finish();
}

/// agent 操控期间：玩家连接是观察方，agent 组队后收到推送；社交页可以浏览、看详情，解散在客户端被拦（不发请求）；
/// take 收回控制权之后 `party disband yes` 才发出请求。
#[test]
fn observer_is_blocked_from_disbanding_until_control_is_taken_back() {
    let mut h = enter_world("ui_party_agent");
    h.command("token new 助手 30");
    h.wait("观察方并收到队伍", 10, |a| {
        a.world.as_ref().is_some_and(|w| w.observing() && w.party().is_some())
    });
    open_social(&mut h);
    let s = draw(&mut h, 80, 24);
    assert!(
        s.contains("队伍 · 1 队长 + 4 分身") && s.contains("agent 操控中"),
        "{s}"
    );
    // 动作菜单里解散保留显示并解释；Enter 只在消息栏说明，不发请求。
    h.keys(&[Enter, Down]);
    let s = draw(&mut h, 80, 24);
    assert!(s.contains("（agent 正在操控，输入 take 收回控制权）"), "{s}");
    h.key(Enter);
    assert!(has_log(&h.app, Kind::Error, "agent 正在操控"));
    // 看详情照常可以。
    h.keys(&[Up, Enter]);
    assert!(matches!(
        h.app.world.as_ref().unwrap().party_step,
        Some(PartyStep::Detail { .. })
    ));
    h.keys(&[Esc, Esc]);
    // 命令同样被拦。
    h.command("party disband yes");
    assert_eq!(party_size(&h.app), Some(5), "被拦下，队伍还在");
    // 收回控制权后可以解散。
    h.command("take");
    h.wait("收回控制权", 5, |a| {
        a.world.as_ref().is_some_and(|w| !w.observing())
    });
    h.command("party disband yes");
    h.wait("队伍消失", 5, |a| party_size(a).is_none());
    h.finish();
}
