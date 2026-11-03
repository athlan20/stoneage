//! STAGE-208 验收（场景）：宠物页技能里选加工，勾选两件斧头、确认后发出 pet_skill（带 items），得到合成结果页；
//! 命令 `pet skill` 带格号在冷却中扣料还一件；界面再选两件食物同样因冷却还料。
//! 模拟服务端按用例 `ui_pet_merge` 回放（严格模式：请求的 type 和 data 都要一致）。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Down, Enter, Esc, Left, Tab};
use stoneage_client::app::{App, Confirm, MergeKind, PETS_MENU, PetStep, Screen};
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios_pet_manage.rs 的 login_to_characters）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 从场景打开宠物页：先退回场景，Esc 打开游戏菜单，下一下到「宠物」（游戏菜单第 2 项），Enter 进页。
fn open_pets(h: &mut Harness) {
    for _ in 0..8 {
        if h.app.world.as_ref().is_some_and(|w| w.at_scene()) {
            break;
        }
        h.key(Esc);
    }
    h.key(Esc);
    h.key(Down);
    h.key(Enter);
    assert_eq!(h.app.world.as_ref().map(|w| w.menu), Some(PETS_MENU));
}

/// 消息栏里有没有一条包含 text 的消息。
fn logged(a: &App, text: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(text))
}

/// 当前有几只宠物。
fn pet_count(a: &App) -> usize {
    a.world.as_ref().map_or(0, |w| w.pets.pets.len())
}

/// 背包里有几件物品。
fn bag_count(a: &App) -> usize {
    a.world.as_ref().map_or(0, |w| w.inventory.slots.len())
}

/// 背包某格的物品名（空格为 None）。
fn bag_name(a: &App, slot: u8) -> Option<String> {
    a.world
        .as_ref()?
        .inventory
        .slots
        .iter()
        .find(|s| s.slot == slot)
        .map(|s| s.name.clone())
}

/// 选材料清单里的背包格号（按背包顺序）。
fn merge_slots(a: &App) -> Vec<u8> {
    a.world
        .as_ref()
        .map_or_else(Vec::new, |w| w.merge_materials().iter().map(|s| s.slot).collect())
}

/// 当前的宠物页步骤。
fn pet_step(a: &App) -> Option<&PetStep> {
    a.world.as_ref()?.pet_step.as_ref()
}

/// 结果页的种类和几行字（不在结果页为 None）。
fn merge_report(a: &App) -> Option<(MergeKind, Vec<String>)> {
    match pet_step(a)? {
        PetStep::MergeResult { report, .. } => Some((report.kind, report.lines.clone())),
        _ => None,
    }
}

/// 等待确认的合成：勾选的背包格号（没有等待为 None）。
fn pending_merge(a: &App) -> Option<Vec<u8>> {
    match a.world.as_ref()?.confirm.as_ref()? {
        Confirm::PetMerge { items, .. } => Some(items.clone()),
        _ => None,
    }
}

#[test]
fn pet_merge_ui_and_command_follow_protocol() {
    let mut h = Harness::new("ui_pet_merge", opts);
    login_to_characters(&mut h);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);

    // 1. 进宠物页，等宠物 1 只、背包 6 件。
    open_pets(&mut h);
    h.wait("1 只宠物、6 件背包物品", 5, |a| {
        pet_count(a) == 1 && bag_count(a) == 6
    });

    // 2. 界面合成：Enter 进动作 → 下 5 下到「技能」（骑乘之后）→ Enter → 下 2 下到第 3 行「加工」→ Enter 进选材料。
    h.key(Enter);
    h.repeat(Down, 5);
    h.key(Enter);
    h.repeat(Down, 2);
    h.key(Enter);
    h.wait("进入选材料", 5, |a| {
        matches!(pet_step(a), Some(PetStep::Merge { .. }))
    });
    // 空格勾选 5 号，↓ 到 6 号再勾选；Enter 进入默认取消的确认。
    h.key(Char(' '));
    h.key(Down);
    h.key(Char(' '));
    h.key(Enter);
    h.wait("等待确认合成", 5, |a| pending_merge(a).is_some());
    assert_eq!(pending_merge(&h.app), Some(vec![5, 6]), "确认的是 5、6 号格");
    // 确认层默认取消：← 切到确认按钮，Enter 确认。
    h.key(Left);
    h.key(Enter);
    h.wait("合成结果页（创建）", 5, |a| {
        matches!(merge_report(a), Some((MergeKind::Created, _)))
    });
    let (_, lines) = merge_report(&h.app).expect("结果页");
    assert!(lines[0].contains("合成枪1"), "结果第一行应写出合成枪1：{lines:?}");
    h.wait("5 号格变成合成枪1", 5, |a| {
        bag_name(a, 5).as_deref() == Some("合成枪1")
    });

    // 3. 结果页 Esc 回技能列表，再 Esc、Esc 退回宠物页；命令带格号，冷却中扣料还一件。
    h.key(Esc);
    assert!(
        matches!(pet_step(&h.app), Some(PetStep::Skills { .. })),
        "结果页 Esc 应回技能列表"
    );
    h.keys(&[Esc, Esc]);
    assert!(pet_step(&h.app).is_none(), "两次 Esc 应退回宠物页");
    h.command("pet skill 0 3 7 8");
    h.wait("命令的冷却消息", 5, |a| logged(a, "过於频繁"));

    // 4. 再界面合成：清单是 5、6、9、10 四件，光标 ↓↓ 到第 3 件（9 号格）勾选 9、10。
    open_pets(&mut h);
    h.key(Enter);
    h.repeat(Down, 5);
    h.key(Enter);
    h.repeat(Down, 2);
    h.key(Enter);
    h.wait("选材料清单是 5、6、9、10", 5, |a| {
        merge_slots(a) == vec![5, 6, 9, 10]
    });
    h.keys(&[Down, Down]);
    h.key(Char(' '));
    h.key(Down);
    h.key(Char(' '));
    h.key(Enter);
    h.wait("等待确认第二次合成", 5, |a| pending_merge(a).is_some());
    assert_eq!(pending_merge(&h.app), Some(vec![9, 10]), "确认的是 9、10 号格");
    h.key(Left);
    h.key(Enter);
    h.wait("合成结果页（冷却）", 5, |a| {
        matches!(merge_report(a), Some((MergeKind::Cooldown, _)))
    });
    let (_, lines) = merge_report(&h.app).expect("结果页");
    assert!(lines[0].contains("过於频繁"), "冷却结果第一行应写过於频繁：{lines:?}");
    h.finish();
}
