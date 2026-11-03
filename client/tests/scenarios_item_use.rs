//! STAGE-30 验收（场景）：`item use` 使用消耗品——给宠物和给自己用回复肉，消息栏显示回复量，
//! 数量减 1 由推送更新；错误分支（空格、组队目标、倒下的宠物）给出明确提示。
//! 模拟服务端按用例 `world_item_use` 回放（严格模式），战斗段用 defend 和数字键走完。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::{KeyCode, KeyCode::Enter, KeyCode::Tab};
use stoneage_client::app::battle::{Actor, Phase};
use stoneage_client::app::{App, Screen};
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios_items.rs）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 在底部输入行敲一条命令并回车。
fn command(h: &mut Harness, line: &str) {
    h.text(line);
    h.key(Enter);
}

/// 消息栏里有没有一条包含 text 的消息。
fn logged(a: &App, text: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(text))
}

/// 背包里某格物品的数量（没有这格是 0）。
fn count_at(a: &App, slot: u8) -> u32 {
    a.world
        .as_ref()
        .and_then(|w| w.inventory.slots.iter().find(|s| s.slot == slot))
        .map_or(0, |s| s.count)
}

/// 这个回合轮到这个角色出招。
fn input(a: &App, turn: u32, actor: Actor) -> bool {
    a.world
        .as_ref()
        .and_then(|w| w.battle.as_ref())
        .is_some_and(|b| b.turn == turn && b.phase == Phase::Input(actor))
}

#[test]
fn item_use_follows_pushes() {
    let mut h = Harness::new("world_item_use", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.enter_page();
    h.wait("管理命令发的肉在 5 号格（数量 2）", 5, |a| count_at(a, 5) == 2);

    // 给 1 号宠物（乌力）用肉：满血不能用，服务端回 ITEM_UNUSABLE，数量仍是 2（STAGE-179）。
    command(&mut h, "item use 5 1");
    h.wait("满血宠物的拒绝提示", 5, |a| logged(a, "乌力 血量是满的，用不着"));
    h.wait("数量仍是 2", 5, |a| count_at(a, 5) == 2);

    // beaten 剧本：两个回合都防御，怪物第二回合全体逃走（draw），掉血保留。
    h.wait("第 1 回合轮到人物出招", 5, |a| input(a, 1, Actor::Player));
    command(&mut h, "defend");
    h.wait("第 1 回合轮到宠物出招", 5, |a| input(a, 1, Actor::Pet));
    h.key(KeyCode::Char('3')); // 宠物指令条：1 攻击、2 技能、3 防御
    h.wait("第 2 回合轮到人物出招", 10, |a| input(a, 2, Actor::Player));
    command(&mut h, "defend");
    h.wait("第 2 回合轮到宠物出招", 5, |a| input(a, 2, Actor::Pet));
    h.key(KeyCode::Char('3'));
    h.wait("战斗结果窗口", 10, |a| {
        a.world
            .as_ref()
            .and_then(|w| w.battle.as_ref())
            .is_some_and(|b| b.phase == Phase::Ended)
    });
    h.key(Enter); // 关结果窗口
    h.wait("战斗结束回到世界", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.battle.is_none())
    });

    // 战斗外给自己用肉：补回挨的打（delta 2，用例 beaten@40 里被打两下），数量 2→1。
    command(&mut h, "item use 5");
    h.wait("用给自己的消息", 5, |a| logged(a, "对 勇者小明 使用了 普通的肉，血量回复 2"));
    h.wait("数量减到 1", 5, |a| count_at(a, 5) == 1);

    // 错误分支：空格在本地拦下（不发请求）；目标合不合法由服务端判断（宠物栏位、倒下、编号越界）。
    command(&mut h, "item use 6");
    h.wait("空格的本地提示", 5, |a| logged(a, "6 号格子没有物品"));
    command(&mut h, "item use 5 21");
    h.wait("目标越界的提示", 5, |a| logged(a, "目标编号要在 0–20 之间"));
    command(&mut h, "item use 5 3");
    h.wait("空栏位的提示", 5, |a| logged(a, "3 号目标没有宠物"));
    command(&mut h, "item use 5 1");
    h.wait("倒下宠物的提示", 5, |a| logged(a, "乌力 已经倒下"));
    command(&mut h, "item use 6");
    h.wait("斧头不能用的提示", 5, |a| logged(a, "小斧头 现在还不能使用"));
    command(&mut h, "item use 7");
    h.wait("战斗专用物品的提示", 5, |a| logged(a, "结婚蛋糕 只能在战斗中使用"));
    command(&mut h, "item use 8");
    h.wait("满血满魔的气力肉被拒", 5, |a| logged(a, "血量和气力都是满的，用不着"));

    h.finish();
}
