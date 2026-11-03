//! STAGE-299 精工（客户端）验收：回放 `ui_item_inlay`（严格模式），`docs/inlay.md` 2.8、`docs/protocol.md` 5.73。
//! 背包动作「精工」取消（默认焦点在取消）→ 勾选 2 件材料确认镶上（绿色）→ 命令 `item inlay` 中途镶满（部分成功，黄色）
//! → 服务端台词 → 本地拦截不存在的格子 → 镶满后动作灰掉。请求与推送按用例逐字段对。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Down, Enter, Esc, Right, Tab};
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

/// 背包的某一格是不是这件物品。
fn bag_has(a: &App, slot: u8, name: &str) -> bool {
    a.world
        .as_ref()
        .is_some_and(|w| w.inventory.slots.iter().any(|s| s.slot == slot && s.name == name))
}

/// 背包的某一格是否已经没有东西。
fn bag_empty(a: &App, slot: u8) -> bool {
    a.world
        .as_ref()
        .is_some_and(|w| w.inventory.slots.iter().all(|s| s.slot != slot))
}

#[test]
fn inlay_menu_and_command_follow_the_protocol() {
    let mut h = Harness::new("ui_item_inlay", opts);
    login_to_characters(&mut h);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
    h.enter_page();
    h.key(Char('4')); // 背包页，默认选中 5 号轻的斧头
    h.wait("背包里有可镶的斧头", 5, |a| bag_has(a, 5, "轻的斧头"));

    // 1. 取消：Enter 打开动作 → ↓ 到「精工」→ Enter 进材料选择 → 空格勾 6 号 → Enter 进确认 → 默认焦点在「取消」→ Enter 取消。
    h.keys(&[Enter, Down, Enter, Char(' ')]);
    h.key(Enter);
    h.wait("确认里写清目标和材料", 5, |a| {
        logged(a, "将 魅力梨子 镶到 轻的斧头 上，永久占用 1 格")
    });
    h.key(Enter);
    h.wait("默认取消，不发请求", 5, |a| logged(a, "已取消精工"));

    // 2. 成功：勾 6、7 号（剩余 2 格，再勾 8 号被拦下），确认窗口 → 右移到「确认」→ Enter。
    h.keys(&[Enter, Down, Enter, Char(' '), Down, Char(' '), Down, Char(' ')]);
    h.wait("勾满了就拦下", 5, |a| logged(a, "最多勾 2 件（剩余 2 格）"));
    h.key(Enter);
    h.wait("确认里写清 2 件", 5, |a| logged(a, "永久占用 2 格"));
    h.keys(&[Right, Enter]);
    h.wait("绿色的成功消息", 5, |a| logged(a, "轻的斧头 镶上了 2 件材料"));
    h.wait("红宝石扣掉 1 个，梨子没了", 5, |a| {
        bag_empty(a, 6) && bag_has(a, 7, "红宝石")
    });

    // 2.5 镶满的斧头：动作「精工」灰掉，回车只说明原因。
    h.keys(&[Enter, Down, Enter]);
    h.wait("镶满后灰掉并说明", 5, |a| logged(a, "已镶满，不能再精工"));
    h.key(Esc);

    // 3. 部分成功（命令）：大斧头只剩 1 格，8、9 号镶到第 2 件时镶满。
    command(&mut h, "item inlay 10 8 9 yes");
    h.wait("黄色的部分成功消息", 5, |a| {
        logged(a, "镶了 1 件，其余没有镶上：武器或防具已经镶满了。")
    });
    h.wait("绿宝石扣掉了", 5, |a| bag_empty(a, 8));

    // 4. 服务端台词：目标 9 号是材料，不是武器或防具。
    command(&mut h, "item inlay 9 7 yes");
    h.wait("服务端台词", 5, |a| logged(a, "精工必须选择一个武器或防具。"));

    // 5. 本地拦截：6 号已经没有东西，不发请求。
    command(&mut h, "item inlay 5 6 yes");
    h.wait("格子没有物品", 5, |a| logged(a, "6 号格子没有物品"));

    h.finish();
}
