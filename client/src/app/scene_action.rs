//! 场景页的三个入口（STAGE-134，`docs/cli-ui-redesign.md`「场景与传送」）：
//! 「快捷传送」打开和游戏菜单同一份层级目录（STAGE-161 起，原来这里是整页的地点列表），
//! 「查找并直接对话」打开 NPC 目录（全部可达 / 本地图两类，可按名字搜索，选候选后对话或传送），
//! 「回城」用服务端给的**存档点落点**回去。
//!
//! 目录用 `npc_list`（协议 5.47）而不是 `npc_find`：后者按名字精确查、有条数上限、也拿不到总数，
//! 拼不出完整目录。跨图对话沿用 `npc_visit.rs` 的正式流程（问一句 → `warp` → `talk`）。
//! 距离只用于显示，排序一律以服务端为准。

use crossterm::event::KeyEvent;

use serde_json::json;

use crate::proto::NpcMatch;

use super::input::TextInput;
use super::session::Pending;
use super::{App, Layer};

/// NPC 目录的分类。
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum CatalogScope {
    /// 全部可达：当前地图的加上能传到（在快捷传送目录里）的地图上的。
    Reachable,
    /// 只在当前地图上找。
    Here,
}

impl CatalogScope {
    pub fn label(self) -> &'static str {
        match self {
            CatalogScope::Reachable => "全部可达",
            CatalogScope::Here => "本地图",
        }
    }

    /// 协议里的取值（`docs/protocol.md` 5.47）。
    pub(super) fn wire(self) -> &'static str {
        match self {
            CatalogScope::Reachable => "reachable",
            CatalogScope::Here => "here",
        }
    }

    pub fn next(self) -> Self {
        match self {
            CatalogScope::Reachable => CatalogScope::Here,
            CatalogScope::Here => CatalogScope::Reachable,
        }
    }
}

/// 搜索词和过滤词最多几个字符。
const TEXT_MAX_CHARS: usize = 20;
/// 一次最多取多少条目录（服务端上限 200，够用；再多就让玩家用搜索缩小范围）。

/// 场景页打开的步骤。
#[derive(Debug)]
pub enum SceneStep {
    /// NPC 目录：搜索词 + 分类 + 服务端返回的候选。
    Catalog {
        text: TextInput,
        scope: CatalogScope,
        items: Vec<NpcMatch>,
        total: usize,
        index: usize,
        /// 搜索词或分类改过、还没重新查。
        dirty: bool,
        /// 正在等 `npc_list` 的响应。
        loading: bool,
    },
}

impl SceneStep {
    /// 这一步的标题（渲染用）。
    pub fn title(&self) -> String {
        match self {
            SceneStep::Catalog { scope, total, .. } => {
                format!("NPC 目录 · {}（共 {total}）", scope.label())
            }
        }
    }
}

impl App {
    /// 场景的「传送」入口（STAGE-161）：打开游戏菜单里的同一个快捷传送目录。
    /// 原来这里另开一份只有 8 个地点的列表，现在两处走同一套层级目录。
    pub(super) fn open_travel_page(&mut self) -> bool {
        if self.world.is_none() {
            return false;
        }
        if let Some(world) = self.world.as_mut() {
            world.menu = super::MAP_MENU;
            world.nav.show_page(super::MAP_MENU);
        }
        // 目录还没拿到（旧服务端、或上次查询失败）就重新取一次；页面上有对应的提示和重试入口。
        if self.world.as_ref().is_some_and(|w| !w.travel.ready()) {
            self.retry_travel_catalog();
        }
        true
    }

    /// 重新取一次快捷传送目录（目录不可用时的重试）。
    pub(super) fn retry_travel_catalog(&mut self) {
        self.send("maps", json!({}), Pending::Maps);
    }

    /// 场景「查找并直接对话」：打开 NPC 目录，进去先按默认分类查一次。
    pub(super) fn scene_open_catalog(&mut self) -> bool {
        if self.world.is_none() {
            return false;
        }
        if let Some(world) = self.world.as_mut() {
            world.scene_step = Some(SceneStep::Catalog {
                text: TextInput::new(TEXT_MAX_CHARS),
                scope: CatalogScope::Reachable,
                items: Vec::new(),
                total: 0,
                index: 0,
                // 一进来就是「脏」的：Enter 会先把整份目录查回来。
                dirty: true,
                loading: false,
            });
            world.nav.push(Layer::Form);
        }
        self.catalog_fetch();
        true
    }

    /// 场景「回城」：用服务端给的存档点落点传送（没有记录过存档点时服务端给的是出生地）。
    pub(super) fn scene_go_home(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        let Some(sp) = world.entry.savepoint.clone() else {
            self.log
                .system("服务端没有给存档点位置，先找个存档点记录一次");
            return true;
        };
        if sp.floor == world.entry.position.floor {
            self.log.system(format!("已经在{}了", sp.map_name));
            return true;
        }
        self.send(
            "warp",
            json!({"floor": sp.floor}),
            Pending::Warp(sp.map_name),
        );
        true
    }

    /// 场景步骤里的按键。返回 true 表示已处理。
    pub(super) fn scene_step_key(&mut self, key: KeyEvent) -> bool {
        let Some(step) = self.world.as_ref().and_then(|w| w.scene_step.as_ref()) else {
            return false;
        };
        match step {
            SceneStep::Catalog { .. } => self.scene_catalog_key(key),
        }
    }

    // ---- NPC 目录 ----


    /// `Esc`：回场景，退到底返回 false（调用方把导航栈上那一层弹掉）。
    pub(super) fn scene_step_back(&mut self) -> bool {
        let Some(world) = self.world.as_mut() else {
            return false;
        };
        // 取消不回滚已经查好的候选，直接回场景层（取消不发任何请求）。
        world.scene_step = None;
        if matches!(world.nav.top(), Layer::Action | Layer::Form) { world.nav.pop(); }
        true
    }

    /// 关掉场景步骤，并弹掉导航栈上那一层。
    pub(super) fn close_scene_step(&mut self) {
        if let Some(world) = self.world.as_mut() {
            world.scene_step = None;
            if matches!(world.nav.top(), Layer::Action | Layer::Form) {
                world.nav.pop();
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn catalog_scope_toggles_between_the_two_lists() {
        assert_eq!(CatalogScope::Reachable.next(), CatalogScope::Here);
        assert_eq!(CatalogScope::Here.next(), CatalogScope::Reachable);
        assert_eq!(CatalogScope::Reachable.wire(), "reachable");
        assert_eq!(CatalogScope::Here.wire(), "here");
    }
}
