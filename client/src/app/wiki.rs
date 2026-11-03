//! 「百科」页（STAGE-83，`docs/ui.md` 百科页）：浏览状态与 `wiki` 命令。
//! 数据在 `crate::wiki`（客户端本地的嵌入快照），这里只管当前看到什么：
//! 分类、搜索结果、第几页、打开的词条和它的滚动位置。

use super::select::{self, Column, Item, OnEnter};
use super::world::WorldState;
use super::{App, WIKI_MENU};
use crate::wiki::{DetailLine, Hit, wiki};

/// 列表区还没渲染过时的默认页大小。
const DEFAULT_ROWS: usize = 10;

/// 打开的词条：详情文本行和滚动位置（`scroll` 是顶部行号，渲染时按换行后的总行数收口）。
#[derive(Debug)]
pub struct Opened {
    pub hit: Hit,
    pub lines: Vec<DetailLine>,
    pub scroll: usize,
    /// 详情区最近一次渲染的可见行数，翻页步进用。
    pub view_rows: usize,
}

#[derive(Debug)]
pub struct WikiState {
    /// 当前分类；None 表示还在分类目录。
    pub category: Option<usize>,
    /// 搜索词；空表示列出分类的全部词条。
    pub query: String,
    /// 列表里的词条（分类全部或搜索结果）。编号 = 下标 + 1，`wiki open` 用它。
    pub hits: Vec<Hit>,
    pub page: usize,
    /// 列表区最近一次渲染的可见行数，也是每页条数。
    pub list_rows: usize,
    pub open: Option<Opened>,
}

impl Default for WikiState {
    fn default() -> Self {
        Self { category: None, query: String::new(), hits: vec![], page: 0, list_rows: DEFAULT_ROWS, open: None }
    }
}

impl WikiState {
    /// 总页数（至少 1）。
    pub fn pages(&self) -> usize {
        self.hits.len().div_ceil(self.list_rows.max(1)).max(1)
    }

    /// 当前页之前的条数，即本页第一条的下标。
    pub fn page_start(&self) -> usize {
        self.page.min(self.pages() - 1) * self.list_rows.max(1)
    }
}

/// 词条在选择状态里的编号：分类下标乘一百万再加上它在分类里的下标，列表变化后仍指向同一条词条。
pub(super) fn hit_key(hit: Hit) -> i64 {
    (hit.category * 1_000_000 + hit.index) as i64
}

/// 「百科」页的栏（`docs/interaction.md`「页面选择」）：分类目录、词条列表、详情。
/// 列表没有画出来的时候（分类目录、窄屏打开词条）这一栏不接收焦点，选中位置照样记着。
pub(super) fn columns(world: &WorldState) -> Vec<Column> {
    let w = &world.wiki;
    let mut categories: Vec<Item> = wiki()
        .categories()
        .iter()
        .enumerate()
        .map(|(i, _)| Item { key: i as i64, usable: true, enter: Some(OnEnter::Run(format!("wiki {}", i + 1))) })
        .collect();
    categories.push(Item { key: -1, usable: true, enter: Some(OnEnter::Open) });
    let items = w
        .hits
        .iter()
        .enumerate()
        .map(|(i, hit)| Item {
            key: hit_key(*hit),
            usable: true,
            enter: Some(OnEnter::Run(format!("wiki open {}", i + 1))),
        })
        .collect();
    let home = w.category.is_none() && w.query.is_empty();
    let list_shown = !home && !(w.open.is_some() && world.is_narrow());
    vec![
        Column { name: "分类", items: categories, shown: true, detail: false, default_key: None },
        Column { name: "词条", items, shown: list_shown, detail: false, default_key: None },
        Column {
            name: "详情",
            items: vec![Item { key: 0, usable: true, enter: None }],
            shown: w.open.is_some(),
            detail: true,
            default_key: None,
        },
    ]
}

/// 命令执行后，把「百科」页某一栏的选中项定到 `key` 上（`docs/interaction.md`「命令和选中项是同一个东西」）。
fn point(world: &mut WorldState, column: &str, key: i64) {
    let columns = select::page_columns(world);
    select::point_named(&columns, &mut world.select[WIKI_MENU], column, key);
}

/// 列表换了内容（分类、搜索）：词条栏的选中项回到第一条。
fn reset_list(world: &mut WorldState) {
    let columns = select::page_columns(world);
    select::reset_named(&columns, &mut world.select[WIKI_MENU], "词条");
}

impl App {
    /// `wiki [分类 | find 词 | open 编号或 key | next | prev | back]`。
    pub(super) fn wiki_command(&mut self, rest: &str) {
        let (sub, arg) = rest.split_once(char::is_whitespace).map_or((rest, ""), |(a, b)| (a, b.trim()));
        match sub {
            "" => self.wiki_home(),
            "find" | "search" | "搜索" => self.wiki_find(arg),
            "open" | "查看" => self.wiki_open(arg),
            "next" | "下页" => self.wiki_turn(true),
            "prev" | "上页" => self.wiki_turn(false),
            "back" | "返回" => self.wiki_back(),
            _ => self.wiki_category(rest),
        }
    }

    fn wiki_state(&mut self) -> Option<&mut super::WorldState> {
        let world = self.world.as_mut()?;
        world.menu = WIKI_MENU;
        Some(world)
    }

    /// `wiki`：回到分类目录。
    fn wiki_home(&mut self) {
        let Some(world) = self.wiki_state() else { return };
        world.wiki = WikiState { list_rows: world.wiki.list_rows, ..WikiState::default() };
        reset_list(world);
    }

    /// `wiki 分类`：列出分类的全部词条。
    pub(super) fn wiki_category(&mut self, arg: &str) {
        let w = wiki();
        let Some(c) = w.find_category(arg) else {
            self.log.error(format!("没有叫“{arg}”的分类；输入 wiki 看全部分类"));
            return;
        };
        let name = w.categories()[c].name.clone();
        let hits = w.all(c);
        let Some(world) = self.wiki_state() else { return };
        world.wiki = WikiState { category: Some(c), hits, list_rows: world.wiki.list_rows, ..WikiState::default() };
        reset_list(world);
        point(world, "分类", c as i64);
        self.log.system(format!("{name}：共 {} 条；wiki open 编号 看详情，wiki find 词 搜索", w.categories()[c].count));
    }

    /// `wiki find 词`：在当前分类里搜；还没进分类就搜全部分类。
    pub(super) fn wiki_find(&mut self, query: &str) {
        if query.is_empty() {
            self.log.error("用法：wiki find 关键词（多个词用空格分开，需同时满足）");
            return;
        }
        let Some(world) = self.world.as_ref() else { return };
        let category = world.wiki.category;
        let hits = wiki().search(category, query);
        let n = hits.len();
        let Some(world) = self.wiki_state() else { return };
        world.wiki = WikiState { category, query: query.to_string(), hits, list_rows: world.wiki.list_rows, ..WikiState::default() };
        reset_list(world);
        let scope = category.map_or("全部分类".to_string(), |c| wiki().categories()[c].name.clone());
        if n == 0 {
            self.log.system(format!("在{scope}里没有找到“{query}”"));
        } else {
            self.log.system(format!("在{scope}里找到 {n} 条“{query}”；wiki open 编号 看详情"));
        }
    }

    /// `wiki open 编号|key`：编号是列表里显示的序号，key 如 `pet:1`（详情“相关词条”里给出）。
    pub(super) fn wiki_open(&mut self, arg: &str) {
        let Some(world) = self.world.as_ref() else { return };
        let hit = if let Ok(n) = arg.parse::<usize>() {
            match n.checked_sub(1).and_then(|i| world.wiki.hits.get(i)) {
                Some(h) => *h,
                None => {
                    self.log.error(format!("列表里没有第 {n} 条（共 {} 条）", world.wiki.hits.len()));
                    return;
                }
            }
        } else if !arg.is_empty() {
            match wiki().locate(arg) {
                Some(h) => h,
                None => {
                    self.log.error(format!("找不到词条 {arg}"));
                    return;
                }
            }
        } else {
            self.log.error("用法：wiki open 编号|词条编号（如 pet:1）");
            return;
        };
        let Some(entry) = wiki().entry(hit) else {
            self.log.error("这个词条的详情读取失败");
            return;
        };
        let lines = entry.detail_lines(self.unicode);
        let Some(world) = self.wiki_state() else { return };
        world.wiki.open = Some(Opened { hit, lines, scroll: 0, view_rows: DEFAULT_ROWS });
        point(world, "词条", hit_key(hit));
    }

    /// `wiki next|prev`：列表翻页，选中项落到新一页的第一条。
    fn wiki_turn(&mut self, forward: bool) {
        let Some(world) = self.wiki_state() else { return };
        let w = &mut world.wiki;
        if forward && w.page + 1 < w.pages() {
            w.page += 1;
        } else if !forward && w.page > 0 {
            w.page -= 1;
        } else {
            self.log.system(if forward { "已经是最后一页" } else { "已经是第一页" });
            return;
        }
        if let Some(first) = w.hits.get(w.page_start()).copied() {
            point(world, "词条", hit_key(first));
        }
    }

    /// `wiki back`：一层一层退——先关词条，再清搜索，再回分类目录。
    fn wiki_back(&mut self) {
        let Some(world) = self.wiki_state() else { return };
        let w = &mut world.wiki;
        if w.open.is_some() {
            w.open = None;
        } else if !w.query.is_empty() {
            let category = w.category;
            w.query.clear();
            w.hits = category.map_or(vec![], |c| wiki().all(c));
            w.page = 0;
            reset_list(world);
        } else {
            *w = WikiState { list_rows: w.list_rows, ..WikiState::default() };
            reset_list(world);
        }
    }
}
