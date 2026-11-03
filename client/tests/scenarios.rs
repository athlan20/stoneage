//! STAGE-1 验收：客户端连接模拟服务端，按 `docs/protocol/cases/` 的用例逐个场景验证。
//! 模拟服务端以严格模式运行：客户端发出的每个字段值都要与用例一致。

mod support;

use crossterm::event::KeyCode::{BackTab, Backspace, Char, Down, Enter, Esc, Left, Right, Tab, Up};
use stoneage_client::app::{CreateStep, Kind, Screen};
use support::{Harness, opts};

/// 登录页 → 注册页（焦点移到按钮行，选中“注册”）。
fn open_register(h: &mut Harness) {
    h.keys(&[Tab, Tab, Right, Enter]);
    h.wait_screen(Screen::Register);
    h.wait_ready();
}

fn login_as(h: &mut Harness, account: &str, password: &str) {
    h.text(account);
    h.keys(&[Tab]);
    h.text(password);
    h.key(Enter);
}

/// 登录并等到角色选择页。
fn login_to_characters(h: &mut Harness) {
    login_as(h, "alice", "secret1");
    h.wait_screen(Screen::Characters);
}

/// 在空栏位 1 上走完建角三步，与用例 `character_create_ok` 的字段一致。
fn create_default_character(h: &mut Harness, name: &str) {
    h.key(Enter); // 空栏位 → 创建角色
    assert_eq!(h.app.screen, Screen::Create);
    // 外形 3、颜色 2（眼型、嘴型不可选，固定默认值 0）
    h.repeat(Right, 3);
    h.key(Down);
    h.repeat(Right, 2);
    h.key(Enter);
    assert_eq!(h.app.create.step, CreateStep::Stats);
    h.text(name);
    // 能力点 5/5/5/5 → 8/4/4/4
    h.keys(&[Down, Down, Left, Down, Left, Down, Left]);
    h.repeat(Up, 3); // 回到体力那一行
    h.repeat(Right, 3);
    // 属性点 地 5 水 5 → 地 6 水 4（没有剩余点数，从水挪一点给地）
    h.repeat(Down, 4);
    h.key(Right);
    h.key(Enter);
    assert_eq!(h.app.create.step, CreateStep::Home);
    h.key(Enter); // 出生地 0 萨姆吉尔，提交
}

#[test]
fn register_then_login() {
    let mut h = Harness::new("register_ok", opts);
    open_register(&mut h);
    assert!(h.app.register_enabled());
    h.text("Alice"); // 账号大小写不敏感，客户端发送小写
    h.keys(&[Tab]);
    h.text("secret1");
    h.keys(&[Tab]);
    h.text("secret1");
    h.keys(&[Tab, Enter]);
    h.wait_screen(Screen::Login);
    assert_eq!(h.app.login.account.text(), "alice");
    assert!(h.app.login.password.is_empty());
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    assert_eq!(h.app.chars.list.max_slots, 2);
    assert!(h.app.chars.list.characters.is_empty());
    h.finish();
}

/// STAGE-9：服务端只监听 127.0.0.1，客户端用 localhost 当地址也能连上
/// （localhost 可能先解析成 ::1，要逐个地址尝试）。
#[test]
fn connects_through_localhost_name() {
    let mut h = Harness::new("register_ok", |addr| {
        let port = addr.rsplit(':').next().unwrap().to_string();
        opts(format!("localhost:{port}"))
    });
    open_register(&mut h);
    assert!(h.app.register_enabled());
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.keys(&[Tab]);
    h.text("secret1");
    h.keys(&[Tab, Enter]);
    h.wait_screen(Screen::Login);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.finish();
}

#[test]
fn register_checks_locally_before_sending() {
    let mut h = Harness::new("register_ok", opts);
    open_register(&mut h);
    // 账号太短
    h.text("ab");
    h.keys(&[Tab, Enter]);
    assert!(h.last_error().contains("账号"), "{}", h.last_error());
    // 两次密码不一致
    h.keys(&[BackTab]);
    h.text("c");
    h.keys(&[Tab]);
    h.text("secret1");
    h.keys(&[Tab]);
    h.text("secret2");
    h.key(Enter);
    assert!(h.last_error().contains("不一致"), "{}", h.last_error());
    // 什么都没发出去：模拟服务端的用例停在 register 之前，客户端断开后应判定“没走完”
    let mut mock = h.mock;
    drop(h.app);
    let status = mock.wait().unwrap();
    assert!(!status.success());
}

#[test]
fn register_errors_shown() {
    let mut h = Harness::new("register_account_exists", opts);
    open_register(&mut h);
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.keys(&[Tab]);
    h.text("secret1");
    h.keys(&[Tab, Enter]);
    h.wait("账号已存在提示", 5, |a| a.log.all().iter().any(|m| m.text.contains("账号已存在")));
    assert_eq!(h.app.screen, Screen::Register);
    h.finish();
}

#[test]
fn register_rate_limited_shows_wait() {
    let mut h = Harness::new("register_rate_limited", opts);
    open_register(&mut h);
    h.text("rl_user6");
    h.keys(&[Tab]);
    h.text("secret1");
    h.keys(&[Tab]);
    h.text("secret1");
    h.keys(&[Tab, Enter]);
    h.wait("太频繁提示", 5, |a| a.log.all().iter().any(|m| m.text.contains("太频繁")));
    assert!(h.last_error().contains("30 分"), "{}", h.last_error());
    h.finish();
}

#[test]
fn register_button_disabled_when_server_closed() {
    let mut h = Harness::new("register_disabled", opts);
    open_register(&mut h);
    assert!(!h.app.register_enabled());
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.keys(&[Tab]);
    h.text("secret1");
    h.keys(&[Tab, Enter]);
    assert!(h.last_error().contains("未开放注册"), "{}", h.last_error());
    h.finish();
}

#[test]
fn login_wrong_password_then_retry() {
    let mut h = Harness::new("login_wrong_credentials", opts);
    login_as(&mut h, "alice", "wrongpw1");
    h.wait("密码错误提示", 5, |a| a.log.all().iter().any(|m| m.text.contains("账号或密码错误")));
    assert_eq!(h.app.screen, Screen::Login);
    assert_eq!(h.app.login.account.text(), "alice", "账号保留");
    assert!(h.app.login.password.is_empty(), "密码清空");
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.finish();
}

#[test]
fn login_locked_shows_remaining_time() {
    let mut h = Harness::new("login_locked", opts);
    login_as(&mut h, "alice", "secret1");
    h.wait("锁定提示", 5, |a| a.log.all().iter().any(|m| m.text.contains("锁定")));
    assert!(h.last_error().contains("14 分 57 秒"), "{}", h.last_error());
    assert_eq!(h.app.screen, Screen::Login);
    h.finish();
}

#[test]
fn login_disabled_account() {
    let mut h = Harness::new("login_disabled", opts);
    login_as(&mut h, "banned", "secret1");
    h.wait("禁用提示", 5, |a| a.log.all().iter().any(|m| m.text.contains("禁用")));
    assert_eq!(h.app.screen, Screen::Login);
    h.finish();
}

#[test]
fn characters_listed_with_empty_slot() {
    let mut h = Harness::new("login_characters", opts);
    login_to_characters(&mut h);
    let list = &h.app.chars.list;
    assert_eq!(list.max_slots, 2);
    assert_eq!(list.characters.len(), 1);
    assert_eq!(list.characters[0].name, "勇者小明");
    assert!(h.app.chars.character_at(1).is_none());
    h.finish();
}

#[test]
fn create_character_three_steps() {
    let mut h = Harness::new("character_create_ok", opts);
    login_to_characters(&mut h);
    create_default_character(&mut h, "勇者小明");
    h.wait("角色创建后刷新列表", 5, |a| a.screen == Screen::Characters && !a.chars.list.characters.is_empty());
    assert_eq!(h.app.chars.list.characters[0].name, "勇者小明");
    h.finish();
}

#[test]
fn create_name_taken_returns_to_name_step() {
    let mut h = Harness::new("character_create_errors", opts);
    login_to_characters(&mut h);
    create_default_character(&mut h, "勇者小明");
    h.wait("名字被占用提示", 5, |a| a.log.all().iter().any(|m| m.text.contains("角色名已被使用")));
    assert_eq!(h.app.screen, Screen::Create);
    assert_eq!(h.app.create.step, CreateStep::Stats, "回到出错的那一步");
    // 改名后再提交：其余字段保持不变
    h.repeat(Backspace, 4);
    h.text("勇者小红");
    h.key(Enter);
    assert_eq!(h.app.create.step, CreateStep::Home);
    h.key(Enter);
    h.wait("创建成功后刷新列表", 5, |a| a.screen == Screen::Characters && !a.chars.list.characters.is_empty());
    h.finish();
}

#[test]
fn create_warns_when_stat_points_left() {
    let mut h = Harness::new("character_create_ok", opts);
    login_to_characters(&mut h);
    h.key(Enter);
    h.key(Enter); // 外形页 → 名字与属性
    h.text("勇者小明");
    h.keys(&[Down, Left]); // 体力 5 → 4，剩余 1 点
    h.key(Enter);
    assert_eq!(h.app.create.step, CreateStep::Stats, "第一次只提醒");
    assert!(h.last_error().contains("还有 1 点"), "{}", h.last_error());
    h.key(Enter);
    assert_eq!(h.app.create.step, CreateStep::Home, "再按一次确认继续");
    drop(h.app);
    let _ = h.mock.wait();
}

#[test]
fn delete_requires_typing_the_name() {
    let mut h = Harness::new("character_delete", opts);
    login_to_characters(&mut h);
    h.key(Char('d'));
    assert!(h.app.chars.confirm.is_some());
    h.text("别人");
    h.key(Enter);
    assert!(h.app.chars.confirm.is_some(), "输错名字不关闭确认框");
    assert!(h.last_error().contains("不一致"));
    h.repeat(Backspace, 2);
    h.text("勇者小明");
    h.key(Enter);
    h.key(Right);
    h.key(Enter);
    h.wait("删除后列表刷新", 5, |a| a.chars.confirm.is_none() && a.chars.list.characters.is_empty() && a.screen == Screen::Characters);
    h.finish();
}

#[test]
fn delete_wrong_name_sends_nothing() {
    let mut h = Harness::new("character_delete", opts);
    login_to_characters(&mut h);
    h.key(Char('d'));
    h.text("别人");
    h.key(Enter);
    h.key(Esc);
    assert!(h.app.chars.confirm.is_none());
    // 用例停在 delete_character 之前，应判定“没走完”
    let mut mock = h.mock;
    drop(h.app);
    assert!(!mock.wait().unwrap().success());
}

#[test]
fn enter_world_shows_map_and_position() {
    let mut h = Harness::new("world_enter", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    let w = h.app.world.as_ref().unwrap();
    assert_eq!(w.entry.position.map_name, "萨姆吉尔村长之家");
    assert_eq!((w.entry.position.floor, w.entry.position.x, w.entry.position.y), (1006, 15, 22));
    h.finish();
}

#[test]
fn dev_shortcut_logs_in_and_enters_world() {
    let mut h = Harness::new("world_enter", |addr| stoneage_client::app::Options {
        server: addr,
        account: Some("alice".into()),
        password: Some("secret1".into()),
        character: Some("勇者小明".into()),
        unicode: true,
        update_check: false,
        reduce_motion: false,
    });
    h.wait_screen(Screen::World);
    h.enter_page();
    assert_eq!(h.app.world.as_ref().unwrap().entry.position.floor, 1006);
    h.finish();
}

#[test]
fn kicked_by_duplicate_login_returns_to_login_page() {
    let mut h = Harness::new("disconnect_duplicate_login", opts);
    login_to_characters(&mut h);
    h.wait("被挤下线后回到登录页", 8, |a| a.screen == Screen::Login);
    assert!(h.last_error().contains("账号在别处登录"), "{}", h.last_error());
    assert!(h.app.login.password.is_empty());
    h.finish();
}

#[test]
fn idle_timeout_and_shutdown_push_return_to_login_page() {
    for (case, text) in [("disconnect_idle_timeout", "长时间没有响应"), ("disconnect_shutdown", "维护")] {
        let mut h = Harness::new(case, opts);
        login_to_characters(&mut h);
        h.wait("回到登录页", 8, |a| a.screen == Screen::Login);
        assert!(h.last_error().contains(text), "{case}: {}", h.last_error());
        h.finish();
    }
}

#[test]
fn heartbeat_is_sent_at_server_interval() {
    let mut h = Harness::new("hello_heartbeat", opts);
    open_register(&mut h);
    // 模拟服务端把心跳间隔设成 1 秒；用例要求握手后收到一次 ping
    h.run_for(2500);
    h.finish();
}


/// STAGE-22：`warp` 把地点名或编号换成编号发给服务端；位置由 `position_changed` 更新，地图页的 @ 和位置面板跟着变；
/// 编号或名字不对时只在本地提示、不发请求（严格模式的模拟服务端会把多发的请求算作不符）。
#[test]
fn warp_command_teleports() {
    let case = format!("{}/tests/cases/warp_commands.jsonl", env!("CARGO_MANIFEST_DIR"));
    let mut h = Harness::new(&case, opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    h.wait("地点表", 5, |a| a.world.as_ref().is_some_and(|w| w.places.len() == 8));
    let at = |h: &Harness| {
        let p = &h.app.world.as_ref().unwrap().entry.position;
        (p.floor, p.x, p.y)
    };
    for (line, want) in [("warp 3", (3000, 74, 54)), ("warp 萨姆吉尔村", (1000, 50, 116)), ("warp 卡鲁它那村", (4000, 100, 96))] {
        h.text(line);
        h.key(Enter);
        h.wait(line, 5, |a| a.world.as_ref().is_some_and(|w| (w.entry.position.floor, w.entry.position.x, w.entry.position.y) == want));
    }
    h.wait("成功提示", 5, |a| a.log.all().iter().any(|m| m.text == "已传送到 卡鲁它那村"));
    for (line, hint) in [("warp", "用法"), ("warp 不存在", "没有叫“不存在”"), ("warp 村", "好几个")] {
        h.text(line);
        h.key(Enter);
        let last = h.app.log.all().back().map(|m| (m.kind.clone(), m.text.clone())).unwrap();
        assert!(last.0 == Kind::Error && last.1.contains(hint), "{line}: {last:?}");
    }
    assert_eq!(at(&h), (4000, 100, 96), "出错时位置不变");
    // STAGE-82：纯数字不是地点编号时当地图编号发给服务端（1006 是村长家），落点由服务端给。
    h.text("warp 1006");
    h.key(Enter);
    h.wait("地图编号", 5, |a| a.world.as_ref().is_some_and(|w| (w.entry.position.floor, w.entry.position.x, w.entry.position.y) == (1006, 10, 20)));
    h.wait("地图成功提示", 5, |a| a.log.all().iter().any(|m| m.text == "已传送到 萨姆吉尔的村长家"));
    // STAGE-83：地图名搜索（进入世界时取到的地图表）；同名的地图要写编号，错误提示说明原因。
    h.wait("地图表", 5, |a| a.world.as_ref().is_some_and(|w| w.maps.len() == 4));
    for (line, want) in [("warp 村长", (1006, 10, 20)), ("warp 奇努伊", (40, 1, 9))] {
        h.text(line);
        h.key(Enter);
        h.wait(line, 5, |a| a.world.as_ref().is_some_and(|w| (w.entry.position.floor, w.entry.position.x, w.entry.position.y) == want));
    }
    h.text("warp 梦幻的洞窟");
    h.key(Enter);
    let last = h.app.log.all().back().map(|m| (m.kind.clone(), m.text.clone())).unwrap();
    assert!(last.0 == Kind::Error && last.1.contains("41、42"), "{last:?}");
    assert_eq!(at(&h), (40, 1, 9), "同名歧义时位置不变");
    h.finish();
}

/// STAGE-23：角色页显示服务端给的能力、衍生数值和属性；`status` 命令返回的数值也一致（用例 `world_character_panel`）。
#[test]
fn character_page_shows_status_from_server() {
    let mut h = Harness::new("world_character_panel", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    h.key(Char('2'));
    let w = h.app.world.as_ref().unwrap();
    assert_eq!(w.menu, 1, "按 2 应切到角色页");
    let s = &w.entry.status;
    assert_eq!((s.max_hp, s.stats.vital, s.stats.strength), (35, 5, 5));
    assert_eq!((s.derived.attack, s.derived.defense, s.derived.agility), (6, 6, 5));
    assert_eq!((s.elements.earth, s.elements.water), (8, 2));
    // status 的响应在 leave 的响应之前到达，所以回到角色选择页时它已经处理完了。
    h.text("status");
    h.key(Enter);
    h.text("leave");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.finish();
}

/// STAGE-28：进入世界后取背包；服务端推送 `inventory_changed` 后背包页不用刷新就出现小斧头；
/// 离开再进入世界物品还在（用例 `world_inventory_give`，give 由模拟服务端按剧本回放）。
#[test]
fn bag_page_follows_inventory_changes() {
    use ratatui::Terminal;
    use ratatui::backend::TestBackend;
    use unicode_width::UnicodeWidthStr;
    fn page(h: &mut Harness) -> String {
        let mut term = Terminal::new(TestBackend::new(120, 40)).unwrap();
        term.draw(|f| stoneage_client::ui::render(f, &mut h.app)).unwrap();
        let buf = term.backend().buffer().clone();
        (0..40u16)
            .map(|y| {
                let (mut line, mut x) = (String::new(), 0u16);
                while x < 120 {
                    let sym = buf[(x, y)].symbol();
                    line.push_str(sym);
                    x += UnicodeWidthStr::width(sym).max(1) as u16;
                }
                line
            })
            .collect::<Vec<_>>()
            .join("\n")
    }
    let mut h = Harness::new("world_inventory_give", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    // 用例先回空背包、紧接着推 inventory_changed；两帧几乎同时到，等最终状态即可。
    h.wait("背包与发物品推送", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.inventory.gold == 1_000_000 && w.inventory.slots.len() == 1)
    });
    let w = h.app.world.as_ref().unwrap();
    let it = &w.inventory.slots[0];
    assert_eq!((it.slot, it.name.as_str(), it.count), (5, "小斧头", 1), "小斧头应放进第一个空背包格");
    h.key(Char('4'));
    let s = page(&mut h);
    assert!(s.contains("小斧头") && s.contains("石币 1000000"), "背包页不用刷新就出现它\n{s}");
    // 离开再进入：物品还在（重新取一次 inventory）。
    h.text("leave");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    h.wait("重进后物品仍在", 5, |a| a.world.as_ref().is_some_and(|w| w.inventory.slots.len() == 1));
    h.finish();
}
