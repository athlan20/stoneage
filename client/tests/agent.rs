//! STAGE-6 验收（客户端一侧）：令牌管理、观察模式、控制权命令、私聊。
//! 连模拟服务端回放 `ui_agent_*` 用例（agent 连接上的步骤由模拟服务端跳过，推送在轮到时发给玩家）；
//! STAGE-104 的观察模式快捷键拦截见 `ui_agent_dialog_control`（对话窗口 + 控制权）。

mod support;

use crossterm::event::KeyCode::{Char, Enter, Esc};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{Kind, Screen};
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

/// 登录、选第一个角色、进入游戏，并进到功能页。
/// STAGE-131 起进入世界的落点是场景层，这里测的是功能页与命令栏里的行为，所以先走 Esc 菜单 → Enter 进页面。
fn enter_world(case: &str) -> Harness {
    let mut h = Harness::new(case, opts);
    h.text("alice");
    h.key(crossterm::event::KeyCode::Tab);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    h
}

fn command(h: &mut Harness, line: &str) {
    if h.app.world.as_ref().is_some_and(|w| !w.nav.is(&stoneage_client::app::Layer::Command)) {
        h.key(Char('/'));
    }
    h.text(line);
    h.key(Enter);
}

fn world(h: &Harness) -> &stoneage_client::app::WorldState {
    h.app.world.as_ref().expect("应在世界中")
}

/// 「系统」页：列出令牌；token new 生成后原文只显示一次；revoke 吊销。
#[test]
fn system_page_manages_tokens() {
    let mut h = enter_world("ui_agent_tokens");
    h.key(Char('8'));
    h.wait("令牌列表", 5, |a| a.world.as_ref().is_some_and(|w| w.agent.loaded));
    assert!(draw(&mut h, 120, 40).contains("生成令牌"));

    command(&mut h, "token new 我的 Claude 30");
    h.wait("生成并刷新列表", 5, |a| a.world.as_ref().is_some_and(|w| w.agent.tokens.len() == 1 && w.agent.fresh.is_some()));
    let screen = draw(&mut h, 120, 40);
    // STAGE-304：系统页的一次性令牌旁边有一行可复制的 Claude Code 接入命令（地址取当前连接）。
    for want in [
        "agent 令牌",
        "t_7Hq2Xa9k",
        "我的 Claude",
        "从未",
        "新令牌（只显示这一次",
        "sat_Zm9vYmFy",
        "claude mcp add stoneage -e STONEAGE_AGENT_TOKEN=sat_Zm9vYmFy",
    ] {
        assert!(screen.contains(want), "缺少 {want}\n{screen}");
    }
    // 令牌原文不进消息栏，也不进剪贴板（STAGE-171：只有 agent 命令会复制）。
    assert!(h.app.log.all().iter().all(|m| !m.text.contains("sat_")), "消息里不应出现令牌原文");
    assert!(h.app.take_clipboard().is_none(), "token new 不应写剪贴板");

    // 切走再切回系统页：新令牌的原文不再显示，列表还在。
    h.key(Char('1'));
    h.key(Char('8'));
    h.run_for(400);
    let screen = draw(&mut h, 120, 40);
    assert!(!screen.contains("sat_Zm9vYmFy") && !screen.contains("新令牌（"), "{screen}");

    command(&mut h, "revoke 1");
    h.wait("吊销后列表为空", 5, |a| a.world.as_ref().is_some_and(|w| w.agent.tokens.is_empty()));
    h.finish();
}

/// 观察模式与控制权命令。
#[test]
fn observer_mode_and_control_commands() {
    let mut h = enter_world("ui_agent_control");
    command(&mut h, "token new 助手 30");
    h.wait("令牌列表", 5, |a| a.world.as_ref().is_some_and(|w| w.agent.tokens.len() == 1));

    // agent 进入游戏接管：菜单栏右侧出现“agent 操控中”。
    h.wait("观察模式", 5, |a| a.world.as_ref().is_some_and(|w| w.observing()));
    let screen = draw(&mut h, 120, 40);
    assert!(screen.contains("agent 操控中"), "{screen}");
    assert!(h.app.log.all().iter().any(|m| m.text.contains("接管了控制权")));

    // “操作”类命令被客户端提前拦下，不发给服务端（严格模式的模拟服务端会把多余的请求算作不符）。
    command(&mut h, "turn 3");
    assert!(h.last_error().contains("输入 take 收回控制权"), "{}", h.last_error());
    command(&mut h, "warp 3");
    assert!(h.last_error().contains("输入 take 收回控制权"), "{}", h.last_error());
    // “查看”类命令照常。
    command(&mut h, "status");
    h.run_for(300);
    assert!(world(&h).observing());

    command(&mut h, "take");
    h.wait("收回控制权", 5, |a| a.world.as_ref().is_some_and(|w| !w.observing()));
    let screen = draw(&mut h, 120, 40);
    assert!(!screen.contains("agent 操控中"), "{screen}");
    command(&mut h, "turn 6");
    h.wait("转向", 5, |a| a.world.as_ref().is_some_and(|w| w.entry.position.direction == 6));

    command(&mut h, "release");
    h.wait("交还", 5, |a| a.world.as_ref().is_some_and(|w| w.observing()));
    assert!(draw(&mut h, 120, 40).contains("agent 操控中"));

    command(&mut h, "kick-agent");
    h.wait("踢下线后恢复操控", 5, |a| a.world.as_ref().is_some_and(|w| !w.observing()));
    assert!(h.app.log.all().iter().any(|m| m.text.contains("控制权回到你手里")));

    command(&mut h, "kick-agent");
    h.run_for(300);
    assert!(h.app.log.all().back().is_some_and(|m| m.kind == Kind::Error), "agent 不在线时应显示错误");
    h.finish();
}

/// 私聊：ag 发给 agent；agent 的回复用洋红色的 agent 消息显示；本地先检查长度。
#[test]
fn private_chat_with_agent() {
    let mut h = enter_world("ui_agent_chat");
    command(&mut h, "token new 助手 30");
    h.wait("令牌列表", 5, |a| a.world.as_ref().is_some_and(|w| w.agent.tokens.len() == 1));

    command(&mut h, "ag");
    assert!(h.last_error().contains("用法"), "{}", h.last_error());
    command(&mut h, &format!("ag {}", "字".repeat(201)));
    assert!(h.last_error().contains("200"), "{}", h.last_error());
    // 以上两条都没有发给服务端（严格模式下多发会算不符）。

    command(&mut h, "ag 先去加加村，别打架");
    h.wait("agent 的回复", 5, |a| a.log.all().iter().any(|m| m.kind == Kind::Agent && m.text.contains("收到，出发")));
    // 自己发出的话（STAGE-17）和 agent 的回复都在消息栏里，先后顺序正确，颜色相同。
    let chat: Vec<_> = h.app.log.all().iter().filter(|m| m.kind == Kind::Agent).map(|m| m.text.clone()).collect();
    assert_eq!(chat, ["你: 先去加加村，别打架", "收到，出发"]);
    let m = h.app.log.all().iter().rev().find(|m| m.kind == Kind::Agent).unwrap();
    assert_eq!(m.kind.color(), ratatui::style::Color::Magenta);
    let screen = draw(&mut h, 120, 40);
    assert!(screen.contains("你: 先去加加村，别打架") && screen.contains("agent 收到，出发"), "{screen}");
    h.finish();
}

/// agent 不在线时发私聊：只显示错误，不能显示“你: …”（STAGE-17）。
#[test]
fn chat_to_offline_agent_shows_only_error() {
    let mut h = enter_world("ui_agent_chat_offline");
    command(&mut h, "ag 在吗");
    h.run_for(300);
    assert!(h.app.log.all().back().is_some_and(|m| m.kind == Kind::Error), "应显示错误");
    assert!(h.app.log.all().iter().all(|m| m.kind != Kind::Agent), "对方不在线，不应出现“你: …”");
    h.finish();
}

/// agent 操控时令牌被吊销：agent 被断开，控制权回到玩家，菜单栏不再显示“agent 操控中”。
#[test]
fn revoking_token_returns_control() {
    let mut h = enter_world("ui_agent_revoked");
    command(&mut h, "token new 助手 30");
    h.wait("令牌列表", 5, |a| a.world.as_ref().is_some_and(|w| w.agent.tokens.len() == 1));
    h.wait("观察模式", 5, |a| a.world.as_ref().is_some_and(|w| w.observing()));
    command(&mut h, "revoke 1");
    h.wait("恢复操控", 5, |a| a.world.as_ref().is_some_and(|w| !w.observing() && w.agent.tokens.is_empty()));
    assert!(!draw(&mut h, 120, 40).contains("agent 操控中"));
    h.finish();
}

/// 观察模式下按数字选对话选项（STAGE-104，用例 `ui_agent_dialog_control`）：商店窗口开着时 agent 接管，
/// 玩家按数字被客户端拦下——窗口不换、消息栏提示、提示行换成观察模式的提示，而且**不发 `npc_choose`**
/// （用例里收回控制权之后那一步 `npc_choose` 才是要发出的；多发一次会出现在模拟服务端日志里）。
#[test]
fn observer_digit_on_dialog_does_not_send_npc_choose() {
    let mut h = enter_world("ui_agent_dialog_control");
    // 用例里 agent 用这个令牌登录，客户端也要先把它建出来。
    command(&mut h, "token new 助手 30");
    h.wait("令牌列表", 5, |a| a.world.as_ref().is_some_and(|w| w.agent.tokens.len() == 1));
    // 传送到萨姆吉尔村，和特产品贩卖员 198 对话：窗口有「买」「卖」两个选项。
    command(&mut h, "warp 1");
    h.wait("到萨姆吉尔村", 5, |a| a.world.as_ref().is_some_and(|w| w.entry.position.map_name == "萨姆吉尔村"));
    command(&mut h, "talk 198");
    h.wait("商店菜单", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.npc_window.as_ref().is_some_and(|d| d.npc == 198 && d.options.len() == 2))
    });
    // agent 进世界接管：玩家变成观察方，窗口还开着。
    h.wait("观察模式", 5, |a| a.world.as_ref().is_some_and(|w| w.observing()));
    assert!(world(&h).npc_window.is_some(), "接管时窗口还在");

    // 按 1（选「买」）：客户端拦下，不发 npc_choose，窗口不换。
    h.key(Char('1'));
    h.run_for(300);
    assert!(h.last_error().contains("输入 take 收回控制权"), "{}", h.last_error());
    assert!(
        world(&h).npc_window.as_ref().is_some_and(|d| d.options.len() == 2),
        "窗口没有换成商品列表，说明没有发出 npc_choose"
    );
    assert!(draw(&mut h, 120, 40).contains("agent 操控中 · take 收回"), "提示行换成观察模式的提示");

    // take 收回控制权后，同一个键照常发出 npc_choose：商品列表弹出。
    command(&mut h, "take");
    h.wait("收回控制权", 5, |a| a.world.as_ref().is_some_and(|w| !w.observing()));
    h.key(Char('1'));
    h.wait("商品列表", 5, |a| {
        a.world.as_ref().is_some_and(|w| {
            w.npc_window.as_ref().is_some_and(|d| d.options.first().is_some_and(|o| o.label.contains("25 金币")))
        })
    });

    // 关窗，然后查模拟服务端的日志：它收到的 npc_choose 只该有一次（收回控制权之后那次）。
    h.key(Esc);
    h.wait("返回买卖菜单", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.as_ref().is_some_and(|n| n.stage == "menu")));
    h.key(Esc);
    h.wait("关窗", 5, |a| a.world.as_ref().is_some_and(|w| w.npc_window.is_none()));
    let log = h.finish_with_log();
    let received = log.lines().filter(|l| l.contains('←') && l.contains(r#""type":"npc_choose""#)).count();
    assert_eq!(received, 1, "观察模式下的数字键不该发出 npc_choose：\n{log}");
}

/// STAGE-170：`agent` 命令生成令牌，并把接入说明和令牌一起写进消息栏（可整段复制给 agent），不跳「系统」页。
#[test]
fn agent_command_writes_connection_guide_with_token() {
    let mut h = enter_world("ui_agent_guide");
    let menu_before = world(&h).menu;
    command(&mut h, "agent 我的 Claude 30");
    h.wait("接入说明", 5, |a| a.log.all().iter().any(|m| m.text.contains("复制下面")));
    h.wait("刷新令牌列表", 5, |a| a.world.as_ref().is_some_and(|w| w.agent.tokens.len() == 1));

    let texts: Vec<String> = h.app.log.all().iter().map(|m| m.text.clone()).collect();
    let all = texts.join("\n");
    for want in ["/agent.md", "/agent-connect.py", "sat_Zm9vYmFy", "只显示这一次", "STONEAGE_AGENT_TOKEN", "结束"] {
        assert!(all.contains(want), "缺少 {want}\n{all}");
    }
    // 模拟服务端是裸 TCP 线路：WebSocket 地址只能推测，说明里要提醒。
    assert!(all.contains("推测"), "{all}");
    // STAGE-171：说明同时放进剪贴板（不含分隔行和“推测”提示），并在消息栏告知。
    let copied = h.app.take_clipboard().expect("应有待复制的文本");
    assert!(
        copied.contains("sat_Zm9vYmFy") && copied.contains("agent.md"),
        "{copied}"
    );
    assert!(
        !copied.contains("复制下面") && !copied.contains("结束") && !copied.contains("推测"),
        "{copied}"
    );
    assert!(all.contains("已复制到剪贴板"), "{all}");
    // 不跳页，也不走「新令牌」那条只在系统页显示的流程。
    assert_eq!(world(&h).menu, menu_before);
    assert!(world(&h).agent.fresh.is_none());
    h.finish();
}
