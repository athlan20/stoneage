//! STAGE-279：快捷传送根层「练级传送」的状态（`docs/ui.md`「根层的「练级传送」」）。
//! 四层：L1 十三个等级段 → L2 练级点 → L3 落点（只有一个落点时跳过）→ L4 动作。Esc 逐层退回，L1 再退出。
//! 只管状态与按键结果，协议在 `leveling_dir_app.rs`，画面在 `ui/leveling_dir_view.rs`。

use crate::proto::{LevelingCatalog, LevelingEntry, LevelingSegment, LevelingSpot};

use super::travel::TravelNav;

/// 段数（1–10 到 121–130）。
pub const SEGMENTS: usize = 13;

/// 目录加载状态：没取到就显示“加载中”或原因，不能假装是空的。
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub enum LevelingStatus {
    #[default]
    Idle,
    Loading,
    Ready,
    Failed(String),
}

/// 当前所在的层。`Segments` 在栈底。
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum LevelingLevel {
    Segments,
    Entries { seg: usize },
    Spots { seg: usize, entry: usize },
    Actions { seg: usize, entry: usize, spot: usize },
}

/// L4 动作菜单的一项（和 1 级宠物相同）。
pub const ACTIONS: [&str; 4] = ["直接开战", "连续遇敌", "只传送不遇敌", "取消"];

/// Enter / Esc 的结果，交给 `leveling_dir_app.rs` 执行（本模块不发协议）。
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum LevelingAction {
    None,
    /// 进入下一层（已经改了栈）。
    Descend,
    /// 选了 L4 的一项：开战（once / infinite）或只传送（travel）。
    Go {
        spot: LevelingSpot,
        mode: &'static str,
    },
    /// 退出目录，回到快捷传送根层。
    Leave,
}

#[derive(Debug, Clone, Default)]
pub struct LevelingDir {
    pub status: LevelingStatus,
    /// 13 段摘要（`leveling_catalog` 的 `{}` 回复）。
    pub summary: Option<Vec<LevelingSegment>>,
    pub version: String,
    /// 每段的练级点，按需取（`{"segment": min}`），没取到是 None。
    cache: Vec<Option<Vec<LevelingEntry>>>,
    /// 正在取的段下标（一次只取一段）。
    pub in_flight: Option<usize>,
    open: bool,
    levels: Vec<LevelingLevel>,
    cursors: Vec<usize>,
    scrolls: Vec<usize>,
    /// 打开目录时角色的等级，用来把光标放到所在段。
    pub player_level: u32,
    /// 上次渲染时一屏能放多少行（PgUp/PgDn 用）。
    pub rows: usize,
}

impl LevelingDir {
    pub fn is_open(&self) -> bool {
        self.open
    }

    /// 打开目录：回到 L1，光标放在角色等级所在段。摘要没取到过时置为加载中（调用方负责发请求）。
    pub fn open(&mut self, player_level: u32) {
        self.open = true;
        self.player_level = player_level;
        let seg = segment_index_of(player_level);
        self.levels = vec![LevelingLevel::Segments];
        self.cursors = vec![seg];
        self.scrolls = vec![0];
        if self.summary.is_none() {
            self.status = LevelingStatus::Loading;
        }
    }

    /// 退出目录：清空栈。已取到的目录缓存保留。
    pub fn close(&mut self) {
        self.open = false;
        self.levels.clear();
        self.cursors.clear();
        self.scrolls.clear();
    }

    /// 摘要回复：记下版本和 13 段；带段内容的回复顺便存进缓存。
    pub fn install(&mut self, cat: LevelingCatalog) {
        self.version = cat.catalog_version.clone();
        if !cat.segments.is_empty() {
            self.summary = Some(cat.segments.clone());
        }
        if self.cache.len() != SEGMENTS {
            self.cache = vec![None; SEGMENTS];
        }
        if let (Some(seg), Some(entries)) = (cat.segment, cat.entries) {
            if let Some(i) = segment_index_of_min(seg.min) {
                self.cache[i] = Some(entries);
                self.in_flight = None;
            }
        }
        if self.summary.is_some() {
            self.status = LevelingStatus::Ready;
        }
    }

    /// 某段的练级点回复（`{"segment": min}`）。
    pub fn install_segment(&mut self, seg: usize, entries: Vec<LevelingEntry>) {
        if self.cache.len() != SEGMENTS {
            self.cache = vec![None; SEGMENTS];
        }
        if seg < SEGMENTS {
            self.cache[seg] = Some(entries);
        }
        if self.in_flight == Some(seg) {
            self.in_flight = None;
        }
    }

    pub fn fail(&mut self, reason: impl Into<String>) {
        self.in_flight = None;
        self.status = LevelingStatus::Failed(reason.into());
    }

    pub fn level(&self) -> &LevelingLevel {
        self.levels.last().unwrap_or(&LevelingLevel::Segments)
    }

    pub fn depth(&self) -> usize {
        self.levels.len()
    }

    pub fn cursor(&self) -> usize {
        self.cursors.last().copied().unwrap_or(0)
    }

    pub fn scroll(&self) -> usize {
        self.scrolls.last().copied().unwrap_or(0)
    }

    pub fn set_scroll(&mut self, value: usize) {
        if let Some(s) = self.scrolls.last_mut() {
            *s = value;
        }
    }

    /// 段的摘要（L1 每行用）。
    pub fn segment_summary(&self, seg: usize) -> Option<&LevelingSegment> {
        self.summary.as_ref().and_then(|s| s.get(seg))
    }

    /// 某段已取到的练级点；没取到或空段返回空切片（用 `is_segment_loaded` 区分）。
    pub fn entries_of(&self, seg: usize) -> &[LevelingEntry] {
        self.cache.get(seg).and_then(|c| c.as_deref()).unwrap_or(&[])
    }

    pub fn is_segment_loaded(&self, seg: usize) -> bool {
        self.cache.get(seg).is_some_and(|c| c.is_some())
    }

    /// 当前层需要但还没取到的段（进入 L2 时取一次）。
    pub fn pending_segment(&self) -> Option<usize> {
        if self.in_flight.is_some() {
            return None;
        }
        match self.level() {
            LevelingLevel::Entries { seg } if !self.is_segment_loaded(*seg) => Some(*seg),
            _ => None,
        }
    }

    /// 当前层的行数（L1 固定 13 段，L2 是段内练级点数，L3 是落点数，L4 是动作数）。
    pub fn row_count(&self) -> usize {
        match self.level() {
            LevelingLevel::Segments => SEGMENTS,
            LevelingLevel::Entries { seg } => self.entries_of(*seg).len(),
            LevelingLevel::Spots { seg, entry } => self.entry(*seg, *entry).map_or(0, |e| e.spots.len()),
            LevelingLevel::Actions { .. } => ACTIONS.len(),
        }
    }

    pub fn navigate(&mut self, nav: TravelNav) {
        let len = self.row_count();
        let page = self.rows.max(1);
        let Some(cursor) = self.cursors.last_mut() else {
            return;
        };
        *cursor = match nav {
            _ if len == 0 => 0,
            TravelNav::Up => cursor.saturating_sub(1),
            TravelNav::Down => (*cursor + 1).min(len - 1),
            TravelNav::PageUp => cursor.saturating_sub(page),
            TravelNav::PageDown => (*cursor + page).min(len - 1),
            TravelNav::Home => 0,
            TravelNav::End => len - 1,
        };
    }

    fn entry(&self, seg: usize, entry: usize) -> Option<&LevelingEntry> {
        self.entries_of(seg).get(entry)
    }

    /// L3 或 L4 上记住的落点（L4 用它；L3 用光标）。
    pub fn current_spot(&self) -> Option<LevelingSpot> {
        match self.level() {
            LevelingLevel::Spots { seg, entry } => self.entry(*seg, *entry)?.spots.get(self.cursor()).cloned(),
            LevelingLevel::Actions { seg, entry, spot } => self.entry(*seg, *entry)?.spots.get(*spot).cloned(),
            _ => None,
        }
    }

    /// L2 光标指着的练级点（右侧详情用）。
    pub fn selected_entry(&self) -> Option<&LevelingEntry> {
        match self.level() {
            LevelingLevel::Entries { seg } => self.entries_of(*seg).get(self.cursor()),
            LevelingLevel::Spots { seg, entry } | LevelingLevel::Actions { seg, entry, .. } => self.entry(*seg, *entry),
            LevelingLevel::Segments => None,
        }
    }

    /// Enter：L1 进 L2，L2 进 L3（或只有一个落点时直接进 L4），L3 进 L4，L4 执行。
    pub fn enter(&mut self) -> LevelingAction {
        match self.level().clone() {
            LevelingLevel::Segments => {
                let seg = self.cursor();
                self.push(LevelingLevel::Entries { seg });
                LevelingAction::Descend
            }
            LevelingLevel::Entries { seg } => {
                let entry = self.cursor();
                let Some(e) = self.entry(seg, entry) else {
                    return LevelingAction::None;
                };
                let single = e.spots.len() == 1;
                self.push(if single {
                    LevelingLevel::Actions { seg, entry, spot: 0 }
                } else {
                    LevelingLevel::Spots { seg, entry }
                });
                LevelingAction::Descend
            }
            LevelingLevel::Spots { seg, entry } => {
                let spot = self.cursor();
                if self.current_spot().is_none() {
                    return LevelingAction::None;
                }
                self.push(LevelingLevel::Actions { seg, entry, spot });
                LevelingAction::Descend
            }
            LevelingLevel::Actions { .. } => {
                let mode = match self.cursor() {
                    0 => "once",
                    1 => "infinite",
                    2 => "travel",
                    _ => return self.back_one(),
                };
                match self.current_spot() {
                    Some(spot) => LevelingAction::Go { spot, mode },
                    None => LevelingAction::None,
                }
            }
        }
    }

    /// Esc：退一层；L1 再退就离开目录。
    pub fn back(&mut self) -> LevelingAction {
        self.back_one()
    }

    fn back_one(&mut self) -> LevelingAction {
        if self.levels.len() <= 1 {
            self.close();
            return LevelingAction::Leave;
        }
        self.levels.pop();
        self.cursors.pop();
        self.scrolls.pop();
        LevelingAction::Descend
    }

    fn push(&mut self, level: LevelingLevel) {
        self.levels.push(level);
        self.cursors.push(0);
        self.scrolls.push(0);
    }
}

/// 等级所在的段下标（1–10 → 0，…，121 以上 → 12）。
pub fn segment_index_of(level: u32) -> usize {
    let l = level.max(1) as usize;
    ((l - 1) / 10).min(SEGMENTS - 1)
}

/// 段下限（1、11、…）对应的段下标；不是合法段下限返回 None。
pub fn segment_index_of_min(min: i64) -> Option<usize> {
    if min < 1 || (min - 1) % 10 != 0 {
        return None;
    }
    let i = ((min - 1) / 10) as usize;
    (i < SEGMENTS).then_some(i)
}

/// 段下标对应的段下限（1、11、…）。
pub fn segment_min(seg: usize) -> i64 {
    (seg as i64) * 10 + 1
}
