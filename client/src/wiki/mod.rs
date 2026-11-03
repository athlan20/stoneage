//! 石器百科（STAGE-83，`docs/ui.md`「百科页」）：客户端本地读取随程序嵌入的数据快照，
//! 不经过游戏服务端，不用网络协议。快照由 `cmd/stoneage-wiki-index` 生成，格式见
//! `internal/gamewiki/snapshot.go`：分类目录、各分类的列表行（定长数组）、
//! 256 个按词条 key 的 SHA-256 首字节分片的详情。
//! 列表行按分类第一次用到时才解压解析；详情每次从对应分片里取一条。

mod data;
mod entry;
pub mod quest_map;

use std::collections::HashMap;
use std::io::Read;
use std::sync::OnceLock;

use flate2::read::GzDecoder;
use serde::Deserialize;

pub use entry::{DetailLine, Entry, LineStyle};

/// 一个分类（目录里的一项）。
#[derive(Debug, Clone, Deserialize)]
pub struct Category {
    pub id: String,
    pub name: String,
    pub count: usize,
}

/// 列表里的一行（词条摘要）。
#[derive(Debug, Clone)]
pub struct Row {
    pub key: String,
    pub name: String,
    pub description: String,
    pub location: String,
    pub level: String,
    pub hp: String,
    pub group: String,
    /// 搜索用的小写文本：编号、名称、说明、位置、技能等。
    search: String,
    /// 详情所在分片（`SHARDS` 的下标）。
    shard: usize,
}

/// 一次查询命中的词条：分类下标和它在分类列表里的下标。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Hit {
    pub category: usize,
    pub index: usize,
}

/// 左侧目录里不显示的分类（STAGE-185）：词条还在快照里，只是不列进目录。
/// 网页百科 `internal/gamewiki/site/app.js` 的 `HIDDEN_DIRECTORY` 要和它保持一致。
pub const HIDDEN_CATEGORIES: [&str; 3] = ["server_task", "npc_template", "guide"];

pub struct Wiki {
    categories: Vec<Category>,
    rows: Vec<OnceLock<Vec<Row>>>,
}

#[derive(Deserialize)]
struct CatalogFile {
    categories: Vec<Category>,
}

#[derive(Deserialize)]
struct CategoryFile {
    rows: Vec<Vec<String>>,
}

fn gunzip(raw: &[u8]) -> Vec<u8> {
    let mut out = Vec::new();
    GzDecoder::new(raw).read_to_end(&mut out).expect("百科快照不是合法的 gzip");
    out
}

/// 嵌入的快照版本号（`catalog.json.gz` 的 revision）。
pub fn revision() -> &'static str {
    data::REVISION
}

/// 全局只读的百科数据。
pub fn wiki() -> &'static Wiki {
    static WIKI: OnceLock<Wiki> = OnceLock::new();
    WIKI.get_or_init(|| {
        let catalog: CatalogFile = serde_json::from_slice(&gunzip(data::CATALOG)).expect("百科目录不是合法的 JSON");
        let categories: Vec<Category> =
            catalog.categories.into_iter().filter(|c| !HIDDEN_CATEGORIES.contains(&c.id.as_str())).collect();
        let rows = categories.iter().map(|_| OnceLock::new()).collect();
        Wiki { categories, rows }
    })
}

fn parse_row(cols: &[String]) -> Option<Row> {
    // 固定位置：key, kind, name, description, location, level, hp, skills, group, search, shard, image
    let col = |i: usize| cols.get(i).cloned().unwrap_or_default();
    let shard = usize::from_str_radix(cols.get(10)?, 16).ok().filter(|s| *s < 256)?;
    Some(Row {
        key: col(0),
        name: col(2),
        description: col(3),
        location: col(4),
        level: col(5),
        hp: col(6),
        group: col(8),
        search: col(9).to_lowercase(),
        shard,
    })
}

impl Wiki {
    pub fn categories(&self) -> &[Category] {
        &self.categories
    }

    /// 分类的全部列表行（第一次调用时解压解析）。
    pub fn rows(&self, category: usize) -> &[Row] {
        self.rows[category].get_or_init(|| {
            let id = &self.categories[category].id;
            let raw = data::CATEGORIES.iter().find(|(c, _)| c == id).map(|(_, raw)| *raw).expect("目录里的分类没有对应的列表文件");
            let file: CategoryFile = serde_json::from_slice(&gunzip(raw)).expect("百科分类列表不是合法的 JSON");
            file.rows.iter().filter_map(|r| parse_row(r)).collect()
        })
    }

    pub fn row(&self, hit: Hit) -> &Row {
        &self.rows(hit.category)[hit.index]
    }

    /// 按编号（从 1 起）、分类 id（`pet`）、分类名（`宠物与生物`）或名字里包含的字找分类，
    /// 依次尝试；分类 id 和名字不区分大小写。
    pub fn find_category(&self, arg: &str) -> Option<usize> {
        let arg = arg.trim().to_lowercase();
        if arg.is_empty() {
            return None;
        }
        if let Ok(n) = arg.parse::<usize>() {
            return (1..=self.categories.len()).contains(&n).then(|| n - 1);
        }
        let by = |f: &dyn Fn(&Category) -> bool| self.categories.iter().position(f);
        by(&|c| c.id == arg)
            .or_else(|| by(&|c| c.name.to_lowercase() == arg))
            .or_else(|| by(&|c| c.name.to_lowercase().contains(&arg)))
    }

    /// 分类里的全部词条。
    pub fn all(&self, category: usize) -> Vec<Hit> {
        (0..self.rows(category).len()).map(|index| Hit { category, index }).collect()
    }

    /// 搜索：关键词按空白拆开，每个词都要出现在词条的编号、名称、说明、位置等文本里。
    /// `category` 为 None 时搜全部分类。
    pub fn search(&self, category: Option<usize>, query: &str) -> Vec<Hit> {
        let terms: Vec<String> = query.split_whitespace().map(str::to_lowercase).collect();
        let cats: Vec<usize> = category.map_or_else(|| (0..self.categories.len()).collect(), |c| vec![c]);
        let mut hits = vec![];
        for c in cats {
            for (index, row) in self.rows(c).iter().enumerate() {
                if terms.iter().all(|t| row.search.contains(t.as_str())) {
                    hits.push(Hit { category: c, index });
                }
            }
        }
        hits
    }

    /// 按词条 key（如 `pet:1`）找到它在列表里的位置；key 的冒号前是分类 id。
    pub fn locate(&self, key: &str) -> Option<Hit> {
        let (kind, _) = key.split_once(':')?;
        let category = self.categories.iter().position(|c| c.id == kind)?;
        let index = self.rows(category).iter().position(|r| r.key == key)?;
        Some(Hit { category, index })
    }

    /// 读一个词条的详情：解压它所在的分片，取出这一条。
    pub fn entry(&self, hit: Hit) -> Option<Entry> {
        let row = self.row(hit);
        let mut shard: HashMap<String, Entry> = serde_json::from_slice(&gunzip(data::SHARDS[row.shard])).ok()?;
        shard.remove(&row.key)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn catalog_lists_the_visible_eleven_categories() {
        let w = wiki();
        assert_eq!(w.categories().len(), 11);
        assert_eq!(w.categories()[0].id, "quest");
        // 最后一类是 Agent（STAGE-118）；“本服任务验证”“未放置 NPC 模板”“新增与修改指南”不在目录里（STAGE-185）。
        assert_eq!(w.categories()[10].id, "agent");
        assert!(w.categories().iter().all(|c| !HIDDEN_CATEGORIES.contains(&c.id.as_str())));
        let total: usize = w.categories().iter().map(|c| c.count).sum();
        assert!(total > 20000, "词条总数 {total}");
    }

    #[test]
    fn category_row_counts_match_catalog() {
        let w = wiki();
        for (i, c) in w.categories().iter().enumerate() {
            assert_eq!(w.rows(i).len(), c.count, "分类 {} 的列表行数和目录不一致", c.id);
        }
    }

    #[test]
    fn finds_category_by_number_id_and_name() {
        let w = wiki();
        let pet = w.find_category("pet").unwrap();
        assert_eq!(w.find_category("2"), Some(pet));
        assert_eq!(w.find_category("宠物"), Some(pet));
        assert_eq!(w.find_category("PET"), Some(pet));
        let npc = w.find_category("npc").unwrap();
        assert_eq!(w.categories()[npc].id, "npc", "id 精确匹配优先于名字包含");
        assert_eq!(w.find_category("0"), None);
        assert_eq!(w.find_category("11"), Some(10), "11 是最后一类（Agent）");
        assert_eq!(w.find_category("12"), None, "目录只有 11 类");
        assert_eq!(w.find_category("新增与修改指南"), None, "隐藏的分类不能再用名字进");
        assert_eq!(w.find_category("没有这个"), None);
    }

    #[test]
    fn every_entry_has_a_detail_shard() {
        let w = wiki();
        for (c, cat) in w.categories().iter().enumerate() {
            // 每个分类抽首、中、尾三条，完整验证 2 万条太慢
            let n = w.rows(c).len();
            for index in [0, n / 2, n - 1] {
                let hit = Hit { category: c, index };
                let e = w.entry(hit).unwrap_or_else(|| panic!("{} 第 {index} 条读不到详情", cat.id));
                assert_eq!(e.key, w.row(hit).key);
            }
        }
    }

    #[test]
    fn searches_by_name_and_id_and_locates_by_key() {
        let w = wiki();
        let pet = w.find_category("pet").unwrap();
        let hits = w.search(Some(pet), "乌力");
        assert!(hits.iter().any(|h| w.row(*h).key == "pet:1"), "宠物分类里搜“乌力”要命中 pet:1");
        assert!(w.search(None, "乌力").len() >= hits.len(), "全站搜索不少于分类内搜索");
        assert!(w.search(Some(pet), "乌力 不存在的词").is_empty(), "多个词要同时满足");
        let hit = w.locate("pet:1").unwrap();
        assert_eq!(w.row(hit).name, "乌力");
        assert!(w.locate("pet:999999999").is_none());
        assert!(w.locate("没有冒号").is_none());
    }
}
