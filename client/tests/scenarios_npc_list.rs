//! STAGE-134 验收（场景）：场景里选「查找并直接对话」打开 NPC 目录页，客户端用 `npc_list`
//! 取目录（不是拿 `npc_find` 拼）；目录里有两个同名候选时，方向键选到第二个，
//! `Enter` 用**它的**全局编号对话——不静默采用第一项。模拟服务端按用例 `ui_npc_list` 回放。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Down, Enter, Esc, Tab};
use stoneage_client::app::{App, CatalogScope, Screen, SceneStep};
use support::{Harness, opts};

/// 登录并等到角色选择页（同其他 scenarios 文件）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 目录页当前的候选条数（没在目录页时是 0）。
fn catalog_len(a: &App) -> usize {
    match a.world.as_ref().and_then(|w| w.scene_step.as_ref()) {
        Some(SceneStep::Catalog { items, .. }) => items.len(),
        _ => 0,
    }
}

/// 目录页的分类。
fn catalog_scope(a: &App) -> Option<CatalogScope> {
    match a.world.as_ref().and_then(|w| w.scene_step.as_ref()) {
        Some(SceneStep::Catalog { scope, .. }) => Some(*scope),
        _ => None,
    }
}

/// 当前对话窗口对着哪个 NPC。
fn window_npc(a: &App) -> Option<u32> {
    a.world.as_ref()?.npc_window.as_ref().map(|w| w.npc)
}

#[test]
fn catalog_lists_npcs_and_talks_to_the_chosen_candidate() {
    let mut h = Harness::new("ui_npc_list", opts);
    login_to_characters(&mut h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);

    // 场景列表：这张地图没有 NPC，所以第一项就是「快捷传送」，第二项是「查找并直接对话」。
    h.key(Down);
    h.key(Enter);
    h.wait("目录回来了两条", 5, |a| catalog_len(a) == 2);
    assert_eq!(catalog_scope(&h.app), Some(CatalogScope::Reachable), "默认是全部可达");

    // 选到第二个同名候选：Enter 发的是它自己的编号（227），不是第一条的 225。
    h.key(Down);
    h.key(Enter);
    h.wait("弹出第二个候选的窗口", 5, |a| window_npc(a) == Some(227));
    // 关掉窗口，用例才走完。
    h.key(Esc);
    h.wait("窗口关掉", 5, |a| window_npc(a).is_none());
    h.finish();
}

