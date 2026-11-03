//! STAGE-5 联调：真实的客户端（`App`，按键驱动）连真实的 Go 服务端（`cmd/stoneage-server`，真实的
//! Argon2 参数、真实的 PostgreSQL），完整走一遍 注册 → 登录 → 建角 → 进入游戏 → 离开 → 再进入。
//!
//! 需要数据库：设置环境变量 `STONEAGE_TEST_DATABASE_URL`（同服务端的 Go 测试）和 Go 工具链，
//! 没有时这些测试直接跳过。每个测试都起一个独立的服务端进程，用 `-temp-schema` 在库里建一个临时
//! schema，结束时删除，所以不碰开发库里的数据，注册和登录的限流计数也互不影响。
//! 步骤和手工冒烟检查见 `docs/smoke.md`。


mod agent;
mod driver;
mod server;
mod reconnect;
mod exits;

use crossterm::event::KeyCode::*;
use driver::{Client, unique};
use server::Server;
use stoneage_client::app::{App, CreateStep, Kind, Options, Screen};

pub const PASSWORD: &str = "Zq9-plain-secret";

// ---------- 验收 ----------

/// 注册 → 登录 → 建角 → 进入游戏；界面上的地图名和坐标与所选出生地一致；
/// 离开世界再进入、退出客户端重新登录再进入，位置都不变；日志里没有明文密码。
#[test]
fn register_login_create_enter_leave_reenter() {
    let Some(mut server) = Server::start() else { return };
    let account = unique("e2e_");

    let mut c = Client::connect(&server.addr);
    c.register(&account, PASSWORD);
    assert_eq!(c.app.login.account.text(), account);
    c.text(PASSWORD);
    c.key(Enter);
    c.wait_screen(Screen::Characters);
    assert_eq!(c.app.chars.list.max_slots, 2);
    assert!(c.app.chars.list.characters.is_empty());

    c.create_character("勇者小明");
    c.wait("角色出现在列表里", 15, |a| a.screen == Screen::Characters && a.chars.list.characters.len() == 1);
    c.key(Enter);

    c.key(Enter);

    c.wait_screen(Screen::World);

    // 萨姆吉尔的村长之家：1006 (15,22)。
    let (floor, x, y, map_name) = c.world_position();
    assert_eq!((floor, x, y), (1006, 15, 22), "出生点不对");
    assert_eq!(map_name, "萨姆吉尔的村长家");
    let screen = c.screen_text();
    assert!(screen.contains("萨姆吉尔的村长家"), "界面上没有地图名：\n{screen}");
    assert!(screen.contains("15,22"), "界面上没有坐标：\n{screen}");
    assert!(screen.contains("勇者小明"), "界面上没有角色名：\n{screen}");

    // 离开世界后再进入。
    c.key(Char('/'));
    c.text("leave");
    c.key(Enter);
    c.wait_screen(Screen::Characters);
    c.key(Enter);

    c.key(Enter);

    c.wait_screen(Screen::World);
    assert_eq!(c.world_position(), (1006, 15, 22, "萨姆吉尔的村长家".into()), "离开后再进入位置变了");

    // 不离开世界，直接退出客户端（断开连接），重新登录再进入。
    drop(c);
    let mut again = Client::new(Options {
        server: server.addr.clone(),
        account: Some(account.clone()),
        password: Some(PASSWORD.into()),
        character: Some("勇者小明".into()),
        ..Options::default()
    });
    again.wait_screen(Screen::World);
    assert_eq!(again.world_position(), (1006, 15, 22, "萨姆吉尔的村长家".into()), "断开重连后位置变了");
    drop(again);

    server.stop();
    server.assert_no_plaintext(&[PASSWORD]);
}

/// 两个终端用同一个账号登录，先登录的被挤下线，回到登录页并显示原因；后登录的不受影响。
#[test]
fn second_login_kicks_first_and_shows_reason() {
    let Some(server) = Server::start() else { return };
    let account = unique("kick_");

    let mut first = Client::connect(&server.addr);
    first.register(&account, PASSWORD);
    first.submit_password(PASSWORD);
    first.wait_screen(Screen::Characters);

    let mut second = Client::connect(&server.addr);
    second.login(&account, PASSWORD);
    second.wait_screen(Screen::Characters);

    first.wait("被挤下线后回到登录页", 10, |a| a.screen == Screen::Login);
    assert!(first.last_error().contains("别处登录"), "应显示被挤下线的原因，实际 {:?}", first.messages());
    assert!(first.screen_text().contains("别处登录"), "消息栏里应能看到原因");
    // 后登录的还能正常操作。
    second.key(Enter);
    second.run_ms(500);
    assert_eq!(second.app.screen, Screen::Create, "后登录的一方应还在线并能继续操作");
}

/// 关掉服务端：客户端提示连接断开、回到登录页，不崩溃（之后还能正常画界面）。
#[test]
fn server_shutdown_preserves_readonly_world() {
    let Some(mut server) = Server::start() else { return };
    let account = unique("down_");

    let mut c = Client::connect(&server.addr);
    c.register(&account, PASSWORD);
    c.submit_password(PASSWORD);
    c.wait_screen(Screen::Characters);
    c.create_character("勇者小红");
    c.wait("角色出现在列表里", 15, |a| a.screen == Screen::Characters && a.chars.list.characters.len() == 1);
    c.key(Enter);

    c.key(Enter);

    c.wait_screen(Screen::World);

    server.stop();
    c.wait("离线只读", 15, |a| a.world.as_ref().is_some_and(|w|!w.online));
    assert!(c.last_error().contains("维护"), "应提示服务器维护，实际 {:?}", c.messages());
    let screen = c.screen_text();
    assert!(screen.contains("离线") && screen.contains("勇者小红"), "只读页面应保留角色：\n{screen}");
}

/// 连续输错密码后被锁定，客户端显示剩余时间；锁定期间密码正确也进不去。
#[test]
fn wrong_passwords_lock_account_and_show_remaining_time() {
    let Some(mut server) = Server::start() else { return };
    let account = unique("lock_");

    let mut c = Client::connect(&server.addr);
    c.register(&account, PASSWORD);
    let wrong = |a: &App| a.log.all().iter().filter(|m| m.text.contains("账号或密码错误")).count();
    for i in 1..=5 {
        c.submit_password("wrong-pass-1");
        c.wait(&format!("第 {i} 次密码错误"), 15, |a| wrong(a) == i);
        assert_eq!(c.app.screen, Screen::Login);
    }
    // 第 6 次，即使密码正确也被锁定。
    c.submit_password(PASSWORD);
    c.wait("显示锁定", 15, |a| a.log.all().iter().any(|m| m.text.contains("锁定")));
    let msg = c.last_error();
    assert!(msg.contains("锁定") && (msg.contains("分钟") || msg.contains("秒")), "应显示剩余时间，实际 {msg:?}");
    assert_eq!(c.app.screen, Screen::Login);
    server.stop();
    server.assert_no_plaintext(&[PASSWORD, "wrong-pass-1"]);
}

/// 服务端拒绝名字（含 GM、重名）时，客户端回到“名字与属性”一步并显示服务端给的原因，改名后可以继续。
#[test]
fn server_rejected_names_return_to_name_step() {
    let Some(server) = Server::start() else { return };

    let mut a = Client::connect(&server.addr);
    let acct_a = unique("namea_");
    a.register(&acct_a, PASSWORD);
    a.submit_password(PASSWORD);
    a.wait_screen(Screen::Characters);
    // 客户端本地检查放过、服务端不允许的名字：含 GM（全角也算）、含“华义”。
    for bad in ["勇者GM", "Ｇｍ小明", "华义战士"] {
        a.create_character(bad);
        a.wait("服务端拒绝名字", 15, |app| app.screen == Screen::Create && app.create.step == CreateStep::Stats && app.log.all().iter().any(|m| m.kind == Kind::Error));
        let err = a.last_error();
        assert!(err.contains("GM") || err.contains("华义"), "应显示服务端给的原因，实际 {err:?}");
        assert_eq!(a.app.create.name.text(), bad, "名字应保留，方便修改");
        // 回到角色选择页重新来过。
        a.key(Esc);
        a.key(Esc);
        a.wait_screen(Screen::Characters);
    }
    a.create_character("勇者小明");
    a.wait("创建成功", 15, |app| app.screen == Screen::Characters && app.chars.list.characters.len() == 1);

    // 另一个账号想用同名：NAME_TAKEN，改名后成功。
    let mut b = Client::connect(&server.addr);
    let acct_b = unique("nameb_");
    b.register(&acct_b, PASSWORD);
    b.submit_password(PASSWORD);
    b.wait_screen(Screen::Characters);
    b.create_character("勇者小明");
    b.wait("名字被占用", 15, |app| app.screen == Screen::Create && app.log.all().iter().any(|m| m.text.contains("已被使用")));
    assert_eq!(b.app.create.step, CreateStep::Stats, "回到出错的那一步");
    b.repeat(Backspace, 4);
    b.text("勇者小红");
    b.key(Enter);
    b.key(Enter);
    b.wait("创建成功", 15, |app| app.screen == Screen::Characters && app.chars.list.characters.len() == 1);
}

/// 中文、emoji 的名字经过客户端 → 服务端 → 数据库 → 客户端一圈后原样显示，界面不乱。
#[test]
fn unicode_names_round_trip() {
    let Some(server) = Server::start() else { return };
    let account = unique("uni_");
    let mut c = Client::connect(&server.addr);
    c.register(&account, PASSWORD);
    c.submit_password(PASSWORD);
    c.wait_screen(Screen::Characters);

    let long = "一二三四五六七八九十一二三四五六"; // 正好 16 个汉字
    c.create_character(long);
    c.wait("创建成功", 15, |a| a.screen == Screen::Characters && a.chars.list.characters.len() == 1);
    assert!(c.screen_text().contains(long), "16 个汉字的名字应完整显示：\n{}", c.screen_text());

    // 第二个栏位：带 emoji 的名字。
    c.key(Down);
    c.create_character("石器😀勇者");
    c.wait("创建成功", 15, |a| a.screen == Screen::Characters && a.chars.list.characters.len() == 2);
    let names: Vec<_> = c.app.chars.list.characters.iter().map(|ch| ch.name.clone()).collect();
    assert_eq!(names, vec![long.to_string(), "石器😀勇者".to_string()]);
    c.key(Enter);

    c.key(Enter);

    c.wait_screen(Screen::World);
    assert!(c.screen_text().contains("石器😀勇者"));
}

/// 删除角色：要再输入一次角色名；输错不删；删除后列表里没有，也能在同一栏位重新创建。
#[test]
fn delete_character_flow() {
    let Some(server) = Server::start() else { return };
    let account = unique("del_");
    let mut c = Client::connect(&server.addr);
    c.register(&account, PASSWORD);
    c.submit_password(PASSWORD);
    c.wait_screen(Screen::Characters);
    c.create_character("勇者小明");
    c.wait("创建成功", 15, |a| a.screen == Screen::Characters && a.chars.list.characters.len() == 1);

    c.key(Char('d'));
    c.text("别人");
    c.key(Enter);
    assert!(c.app.chars.confirm.is_some(), "输错名字不关闭确认框");
    c.repeat(Backspace, 2);
    c.text("勇者小明");
    c.key(Enter);
    c.key(Right);
    c.key(Enter);
    c.wait("删除后列表刷新", 15, |a| a.chars.confirm.is_none() && a.chars.list.characters.is_empty());

    c.create_character("勇者小明");
    c.wait("重新创建成功", 15, |a| a.screen == Screen::Characters && a.chars.list.characters.len() == 1);
}

