//! 快捷传送目录的状态与层级导航（STAGE-161，`docs/protocol.md` 5.42）。
//!
//! 服务端给的是一棵扁平的节点表（`id` / `parent_id` / `name` / `floor` / `order`），这里建成
//! 「父 → 子」索引，用一条**路径栈**记录玩家逐级下钻到哪，每层各留一个光标和滚动起点，
//! 所以 Esc 退回上一层时能恢复上次的选择（`docs/interaction.md`）。
//!
//! 三条规则来自需求，写在这里免得散落到 UI：
//!   - 有子节点的地图，下一层首项是「传送到本图 · 地图名」，其余是它的下级；
//!   - 没有子节点的地图，Enter 直接传送；纯分组不能传送；
//!   - 选中的就是当前所在地时只提示「已经在这里」，不发传送请求。

use std::collections::HashMap;

use super::leveling_dir::LevelingDir;
use super::pet_dir::PetDir;
use crate::proto::TravelNode;

mod nav;
mod view;
pub use view::RowView;

/// 目录加载状态。`Unavailable` 时页面要显示原因和重试入口，不能假装目录是空的。
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub enum TravelStatus {
    /// 还没请求过（刚进世界，请求在途）。
    #[default]
    Loading,
    /// 拿到了目录树。
    Ready,
    /// 服务端没给节点树（旧服务端）或者响应解析失败，附带原因。
    Unavailable(String),
}

/// 当前层的一项：可能是真实的目录节点，也可能是「传送到本图」那个虚拟首项。
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Entry {
    /// 当前正在浏览的地图本身（只有带子节点的地图节点下面才有这一项）。
    Here(String),
    /// 目录里的一个节点。
    Node(String),
    /// 根层末尾的「1级宠物」入口（STAGE-251）：不是地图，只在根层出现，与地图目录是否就绪无关。
    Pets,
    /// 根层的「练级传送」入口（STAGE-279）：同上，紧跟在「1级宠物」之后。
    Leveling,
}

/// Enter 的结果，交给 `App` 执行（本模块不直接发协议）。
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum TravelAction {
    /// 没变化（例如光标已经在边界上，或者选中的纯分组没有子项）。
    None,
    /// 进入下一层。
    Descend,
    /// 退回上一层。
    Ascend,
    /// 传送到这张地图。
    Warp { floor: u32, name: String },
    /// 选中的就是当前所在地，已经在这里了。
    AlreadyHere(String),
    /// 打开根层的「1级宠物」目录（STAGE-251）。
    OpenPets,
    OpenLeveling,
    /// 当前层都退完了，应该退出这个页面。
    Leave,
}

/// 分页跳转（PgUp/PgDn 翻一屏，Home/End 到头尾）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TravelNav {
    Up,
    Down,
    PageUp,
    PageDown,
    Home,
    End,
}

#[derive(Debug, Clone, Default)]
pub struct TravelState {
    pub status: TravelStatus,
    /// 目录版本号（服务端给的，用来判断缓存是否过期）。
    pub version: String,
    /// 当前所在的地图 floor，用来标「已经在这里」。
    pub here_floor: Option<u32>,
    nodes: HashMap<String, TravelNode>,
    children: HashMap<String, Vec<String>>,
    roots: Vec<String>,
    /// 已经下钻进入的节点 id（不含根层）。
    stack: Vec<String>,
    /// 每层的光标：长度总是 `stack.len() + 1`。
    cursors: Vec<usize>,
    /// 每层的滚动起点，同上。
    scrolls: Vec<usize>,
    /// 上次渲染时一屏能放多少行（PgUp/PgDn 用它）。
    pub rows: usize,
    /// 根层「1级宠物」目录的状态（STAGE-251）；目录打开时由它接管按键和画面。
    pub pets: PetDir,
    /// 根层「练级传送」的状态（STAGE-279）；打开时由它接管按键和画面。
    pub leveling: LevelingDir,
}

impl TravelState {
    /// 用服务端的节点表重建索引，并回到根层。
    pub fn install(&mut self, version: String, nodes: &[TravelNode]) {
        self.nodes.clear();
        self.children.clear();
        self.roots.clear();
        self.version = version;
        if nodes.is_empty() {
            self.status = TravelStatus::Unavailable("服务端没有返回传送目录".into());
            self.stack.clear();
            self.cursors = vec![0];
            self.scrolls = vec![0];
            return;
        }
        for node in nodes {
            if node.id.is_empty() {
                continue;
            }
            self.nodes.insert(node.id.clone(), node.clone());
        }
        // 按 order 排序，同级同 order 时保持服务端给的顺序（稳定排序）。
        for node in nodes {
            if node.id.is_empty() {
                continue;
            }
            match &node.parent_id {
                None => self.roots.push(node.id.clone()),
                Some(parent) => {
                    if !self.nodes.contains_key(parent) {
                        // 父节点不存在的节点当不成孤儿进树：跳过，避免画出一棵假树。
                        continue;
                    }
                    self.children
                        .entry(parent.clone())
                        .or_default()
                        .push(node.id.clone())
                }
            }
        }
        self.sort_layer(None);
        let parents: Vec<String> = self.children.keys().cloned().collect();
        for parent in parents {
            self.sort_layer(Some(&parent));
        }
        self.status = TravelStatus::Ready;
        self.stack.clear();
        self.cursors = vec![0];
        self.scrolls = vec![0];
    }

    fn sort_layer(&mut self, parent: Option<&str>) {
        match parent {
            None => {
                let nodes = &self.nodes;
                self.roots.sort_by_key(|id| nodes.get(id).map_or(0, |n| n.order));
            }
            Some(parent) => {
                let nodes = &self.nodes;
                if let Some(list) = self.children.get_mut(parent) {
                    list.sort_by_key(|id| nodes.get(id).map_or(0, |n| n.order));
                }
            }
        }
    }

    /// 目录能用了没有。
    pub fn ready(&self) -> bool {
        self.status == TravelStatus::Ready
    }

    /// 当前层有哪些项：带子节点的地图先放「传送到本图」，再放它的下级。
    pub fn entries(&self) -> Vec<Entry> {
        let mut out = Vec::new();
        if let Some(current) = self.stack.last() {
            if self.nodes.get(current).is_some_and(|n| n.is_map()) {
                out.push(Entry::Here(current.clone()));
            }
        }
        out.extend(self.child_ids().into_iter().map(Entry::Node));
        // 根层末尾固定一项「1级宠物」（STAGE-251）：它不依赖地图目录，目录加载中或不可用时也在。
        if self.stack.is_empty() {
            out.push(Entry::Pets);
            out.push(Entry::Leveling);
        }
        out
    }

    fn child_ids(&self) -> Vec<String> {
        match self.stack.last() {
            None => self.roots.clone(),
            Some(id) => self.children.get(id).cloned().unwrap_or_default(),
        }
    }

    /// 当前层的光标。
    pub fn cursor(&self) -> usize {
        self.cursors.last().copied().unwrap_or(0)
    }

    pub fn set_cursor(&mut self, index: usize) {
        let len = self.entries().len();
        if len == 0 {
            if let Some(last) = self.cursors.last_mut() {
                *last = 0;
            }
            return;
        }
        if let Some(last) = self.cursors.last_mut() {
            *last = index.min(len - 1);
        }
    }

    /// 当前层的滚动起点。
    pub fn scroll(&self) -> usize {
        self.scrolls.last().copied().unwrap_or(0)
    }

    pub fn set_scroll(&mut self, value: usize) {
        if let Some(last) = self.scrolls.last_mut() {
            *last = value;
        }
    }

    /// 光标移动。返回 true 表示位置真的变了。
    pub fn navigate(&mut self, nav: TravelNav) -> bool {
        let len = self.entries().len();
        if len == 0 {
            return false;
        }
        let step = self.rows.max(1);
        let before = self.cursor();
        let next = match nav {
            TravelNav::Up => before.saturating_sub(1),
            TravelNav::Down => (before + 1).min(len - 1),
            TravelNav::PageUp => before.saturating_sub(step),
            TravelNav::PageDown => (before + step).min(len - 1),
            TravelNav::Home => 0,
            TravelNav::End => len - 1,
        };
        self.set_cursor(next);
        self.cursor() != before
    }

    /// 选中项对应的节点 id（「传送到本图」返回当前地图的节点 id）。
    pub fn selected_id(&self) -> Option<String> {
        match self.entries().get(self.cursor())? {
            Entry::Here(id) | Entry::Node(id) => Some(id.clone()),
            Entry::Pets | Entry::Leveling => None,
        }
    }

    /// 选中项的节点（「传送到本图」就是当前地图本身）。
    pub fn selected(&self) -> Option<&TravelNode> {
        self.nodes.get(&self.selected_id()?)
    }

    /// 当前路径（根 → 当前层）的显示名，面包屑用。
    pub fn path(&self) -> Vec<String> {
        self.stack
            .iter()
            .filter_map(|id| self.nodes.get(id).map(|n| n.name.clone()))
            .collect()
    }

    /// 当前层的标题：根层写「全部地图」，否则是正在浏览的地图或分组名。
    pub fn title(&self) -> String {
        match self.stack.last() {
            None => "全部地图".to_string(),
            Some(id) => self
                .nodes
                .get(id)
                .map(|n| n.name.clone())
                .unwrap_or_else(|| "全部地图".to_string()),
        }
    }

    /// 当前层的项数（含「传送到本图」）。
    pub fn len(&self) -> usize {
        self.entries().len()
    }

}

#[cfg(test)]
#[path = "travel_tests.rs"]
mod tests;
