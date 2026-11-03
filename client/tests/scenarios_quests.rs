//! STAGE-86 验收（场景）：任务页随 `quests_changed` 推送更新、消息栏提示进行中和已完成（清除不提示）、
//! `quest doing/done/all` 切换显示范围、离开再进入进度还在。模拟服务端按用例 `ui_quests` 回放（严格模式）。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Enter, Tab};
use stoneage_client::app::{App, QUEST_MENU, QuestFilter, Screen};
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios_points.rs 的 login_to_characters）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 在底部输入行敲一条命令并回车。
fn command(h: &mut Harness, line: &str) {
    if !h.app.world.as_ref().unwrap().nav.is(&stoneage_client::app::Layer::Command) { h.key(Char('/')); }
    h.text(line);
    h.key(Enter);
}

/// 消息栏里有没有一条包含 text 的消息。
fn logged(a: &App, text: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(text))
}

/// 任务页当前显示的任务编号（按显示范围过滤后的顺序）。
fn visible_nos(a: &App) -> Vec<u32> {
    a.world.as_ref().map_or(vec![], |w| w.visible_quests().iter().map(|q| q.no).collect())
}

/// 任务列表的条数（不受显示范围影响）。
fn quest_len(a: &App) -> usize {
    a.world.as_ref().map_or(0, |w| w.quests.quests.len())
}

/// 进行中和已完成的数量。
fn counts(a: &App) -> Option<(usize, usize)> {
    a.world.as_ref().map(|w| w.quest_counts())
}

#[test]
fn quest_page_follows_pushes_and_filters() {
    let mut h = Harness::new("ui_quests", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();

    // 按 5 切到「任务」页（菜单第 5 项）。
    h.key(Char('5'));
    assert_eq!(h.app.world.as_ref().unwrap().menu, QUEST_MENU, "按 5 应切到任务页");

    // 管理命令改进度：推送不用刷新就出现在列表里，消息栏提示。
    h.wait("推送：猜谜3兄弟已完成", 5, |a| quest_len(a) == 1);
    h.wait("消息栏提示已完成", 5, |a| logged(a, "任务【猜谜3兄弟】已完成"));
    assert_eq!(counts(&h.app), Some((0, 1)));

    h.wait("推送：成人仪式进行中", 5, |a| quest_len(a) == 2);
    h.wait("消息栏提示进行中", 5, |a| logged(a, "任务【成人仪式】进行中"));
    assert_eq!(counts(&h.app), Some((1, 1)));
    assert_eq!(visible_nos(&h.app), vec![1, 2], "进行中的排在前面");

    // 成人仪式完成：状态变成已完成，消息栏提示。
    h.wait("成人仪式变成已完成", 5, |a| counts(a) == Some((0, 2)));
    h.wait("消息栏提示成人仪式已完成", 5, |a| logged(a, "任务【成人仪式】已完成"));

    // 猜谜3兄弟 拿蓝暴 开始：进行中排到最前面。
    h.wait("拿蓝暴进行中", 5, |a| counts(a) == Some((1, 2)));
    assert_eq!(visible_nos(&h.app), vec![3, 1, 2]);

    // 清除猜谜3兄弟：从列表消失，不提示。
    h.wait("猜谜3兄弟被清除", 5, |a| quest_len(a) == 2);
    assert_eq!(visible_nos(&h.app), vec![3, 1]);
    let notes = h.app.log.all().iter().filter(|m| m.text.contains("猜谜3兄弟】")).count();
    assert_eq!(notes, 1, "清除不应有提示，只有之前的一条已完成");

    // quest doing / done / all 切换显示范围。
    command(&mut h, "quest doing");
    assert_eq!(h.app.world.as_ref().unwrap().quest_filter, QuestFilter::Doing);
    assert_eq!(visible_nos(&h.app), vec![3], "只看进行中");
    command(&mut h, "quest done");
    assert_eq!(visible_nos(&h.app), vec![1], "只看已完成");
    command(&mut h, "quest");
    assert_eq!(visible_nos(&h.app), vec![3, 1], "不带参数是全部");
    command(&mut h, "quest 乱来");
    h.wait("用法提示", 5, |a| logged(a, "用法：quest [doing|done|all]"));
    assert_eq!(h.app.world.as_ref().unwrap().quest_filter, QuestFilter::All, "用法错误不改范围");

    // 离开再进入：进度从服务端重新取回来。
    command(&mut h, "leave");
    h.wait_screen(Screen::Characters);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.wait("进入后进度还在", 5, |a| visible_nos(a) == vec![3, 1]);
    // STAGE-131：重新进入世界的落点是场景页（不是某个功能页）。
    assert!(h.app.world.as_ref().unwrap().at_scene(), "重新进入落在场景页");
}
