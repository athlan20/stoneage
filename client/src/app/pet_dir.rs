//! STAGE-251：快捷传送页「1级宠物」目录的状态（`docs/ui.md`「快捷传送页」）。
//! 三层：L1 宠物列表（顶部筛选框）→ L2 遇敌点 → L3 动作。Esc 逐层退回，L1 再退出目录。
//! 只管状态与按键结果，协议与消息在 `pet_dir_app.rs`，画面在 `ui/pet_dir_view.rs`。

use crate::proto::{PetCatalog, PetEntry, PetSpot};

use super::travel::TravelNav;

/// 目录加载状态：没取到就显示“加载中”或原因，不能假装目录是空的。
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub enum PetDirStatus {
    #[default]
    Idle,
    Loading,
    Ready,
    Failed(String),
}

/// 当前所在的层。`Pets` 永远在栈底；`Spots` 记住宠物模板编号，`Actions` 记住选中的遇敌点在 `Spots` 层里的下标。
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PetLevel {
    Pets,
    Spots { pet_id: i64 },
    Actions { pet_id: i64, spot: usize },
}

/// L3 动作菜单的一项。
pub const ACTIONS: [&str; 4] = ["直接开战", "连续遇敌", "只传送不遇敌", "取消"];

/// Enter 的结果，交给 `pet_dir_app.rs` 执行（本模块不发协议）。
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PetAction {
    None,
    /// 进入下一层（已经改了栈）。
    Descend,
    /// 选了 L3 的一项：要去遇敌点开战（`mode` 为 once / infinite）或只传送（travel）。
    Go { pet_id: i64, spot: PetSpot, mode: &'static str },
    /// 退出目录，回到快捷传送根层。
    Leave,
}

#[derive(Debug, Clone, Default)]
pub struct PetDir {
    pub status: PetDirStatus,
    pub catalog: Option<PetCatalog>,
    /// 页面是否打开（进入「1级宠物」后为真）。
    open: bool,
    levels: Vec<PetLevel>,
    cursors: Vec<usize>,
    scrolls: Vec<usize>,
    /// 筛选词；退出目录才清空，返回列表时保留。
    pub filter: String,
    /// 筛选框是否拿着焦点（这时字符都进框，列表的字符键不生效）。
    pub typing: bool,
    /// 上次渲染时一屏能放多少行（PgUp/PgDn 用）。
    pub rows: usize,
}

impl PetDir {
    pub fn is_open(&self) -> bool {
        self.open
    }

    /// 打开目录：回到 L1，光标在第一条。目录已经取到就直接用，否则状态置为加载中（调用方负责发请求）。
    pub fn open(&mut self) {
        self.open = true;
        self.levels = vec![PetLevel::Pets];
        self.cursors = vec![0];
        self.scrolls = vec![0];
        self.typing = false;
        if self.catalog.is_none() {
            self.status = PetDirStatus::Loading;
        }
    }

    /// 退出目录：清空栈与筛选词。
    pub fn close(&mut self) {
        self.open = false;
        self.levels.clear();
        self.cursors.clear();
        self.scrolls.clear();
        self.filter.clear();
        self.typing = false;
    }

    pub fn install(&mut self, catalog: PetCatalog) {
        self.catalog = Some(catalog);
        self.status = PetDirStatus::Ready;
    }

    pub fn fail(&mut self, reason: impl Into<String>) {
        self.status = PetDirStatus::Failed(reason.into());
    }

    pub fn level(&self) -> &PetLevel {
        self.levels.last().unwrap_or(&PetLevel::Pets)
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

    /// 当前层的行数（L1 是筛选后的数量，L2 是遇敌点数，L3 是动作数）。
    pub fn row_count(&self) -> usize {
        match self.level() {
            PetLevel::Pets => self.visible_pets().len(),
            PetLevel::Spots { .. } => self.spots_of(self.selected_pet_id()).len(),
            PetLevel::Actions { .. } => ACTIONS.len(),
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

    /// L1 当前筛选下的宠物（按目录顺序，即服务端给的拼音序）。
    pub fn visible_pets(&self) -> Vec<&PetEntry> {
        let Some(cat) = self.catalog.as_ref() else {
            return Vec::new();
        };
        cat.pets.iter().filter(|p| pet_matches(p, &self.filter)).collect()
    }

    /// L1 光标指着的宠物。
    pub fn selected_pet(&self) -> Option<PetEntry> {
        self.visible_pets().get(self.cursor()).map(|p| (*p).clone())
    }

    /// 当前路径上的宠物模板编号（L2/L3 用）。
    fn selected_pet_id(&self) -> i64 {
        match self.level() {
            PetLevel::Spots { pet_id } | PetLevel::Actions { pet_id, .. } => *pet_id,
            PetLevel::Pets => self.selected_pet().map_or(-1, |p| p.pet_id),
        }
    }

    /// 某只宠物的可遇点（服务端给的顺序：地图、区域 ID）。
    pub fn spots_of(&self, pet_id: i64) -> Vec<&PetSpot> {
        self.catalog
            .as_ref()
            .map(|c| c.spots.iter().filter(|s| s.pet_id == pet_id).collect())
            .unwrap_or_default()
    }

    /// L2 光标指着的遇敌点（进入 L3 之后取的是 `Actions` 里记下的那个）。
    pub fn current_spot(&self) -> Option<PetSpot> {
        match self.level() {
            PetLevel::Spots { pet_id } => self.spots_of(*pet_id).get(self.cursor()).map(|s| (*s).clone()),
            PetLevel::Actions { pet_id, spot } => self.spots_of(*pet_id).get(*spot).map(|s| (*s).clone()),
            PetLevel::Pets => None,
        }
    }

    /// Enter：L1 进 L2，L2 进 L3，L3 执行。
    pub fn enter(&mut self) -> PetAction {
        match self.level().clone() {
            PetLevel::Pets => {
                let Some(pet) = self.selected_pet() else {
                    return PetAction::None;
                };
                if self.spots_of(pet.pet_id).is_empty() {
                    return PetAction::None;
                }
                self.push(PetLevel::Spots { pet_id: pet.pet_id });
                PetAction::Descend
            }
            PetLevel::Spots { pet_id } => {
                let spot = self.cursor();
                if self.spots_of(pet_id).get(spot).is_none() {
                    return PetAction::None;
                }
                self.push(PetLevel::Actions { pet_id, spot });
                PetAction::Descend
            }
            PetLevel::Actions { pet_id, spot } => {
                let Some(found) = self.spots_of(pet_id).get(spot).map(|s| (*s).clone()) else {
                    return PetAction::None;
                };
                let mode = match self.cursor() {
                    0 => "once",
                    1 => "infinite",
                    2 => "travel",
                    _ => return self.back_one(),
                };
                PetAction::Go { pet_id, spot: found, mode }
            }
        }
    }

    /// Esc（非输入态）：有筛选词先清空；否则退一层。L1 再退就离开目录。
    pub fn back(&mut self) -> PetAction {
        if self.depth() == 1 && !self.filter.is_empty() {
            self.filter.clear();
            self.cursors[0] = 0;
            self.scrolls[0] = 0;
            return PetAction::None;
        }
        self.back_one()
    }

    fn back_one(&mut self) -> PetAction {
        if self.levels.len() <= 1 {
            self.close();
            return PetAction::Leave;
        }
        self.levels.pop();
        self.cursors.pop();
        self.scrolls.pop();
        PetAction::Descend
    }

    fn push(&mut self, level: PetLevel) {
        self.levels.push(level);
        self.cursors.push(0);
        self.scrolls.push(0);
    }

    // ---- 筛选框（L1 顶部）----

    /// `/` 或 `f`：把焦点交给筛选框（只在 L1 有意义）。
    pub fn start_typing(&mut self) {
        if self.depth() == 1 {
            self.typing = true;
        }
    }

    pub fn type_char(&mut self, c: char) {
        self.filter.push(c);
        self.reset_list();
    }

    pub fn backspace(&mut self) {
        self.filter.pop();
        self.reset_list();
    }

    /// Enter 应用筛选：焦点回到列表，停在第一条命中。
    pub fn commit_filter(&mut self) {
        self.typing = false;
        self.reset_list();
    }

    /// 输入态里的 Esc：有字先清空，没字就只是把焦点交回列表。
    pub fn cancel_filter(&mut self) {
        self.typing = false;
        if !self.filter.is_empty() {
            self.filter.clear();
            self.reset_list();
        }
    }

    fn reset_list(&mut self) {
        if let Some(c) = self.cursors.first_mut() {
            *c = 0;
        }
        if let Some(s) = self.scrolls.first_mut() {
            *s = 0;
        }
    }
}

/// 筛选词归一化：全角转半角、转小写、去首尾空白（不区分大小写与全半角）。
pub fn normalize(text: &str) -> String {
    text.chars()
        .map(|c| match c {
            '\u{3000}' => ' ',
            '\u{FF01}'..='\u{FF5E}' => char::from_u32(c as u32 - 0xFEE0).unwrap_or(c),
            _ => c,
        })
        .collect::<String>()
        .to_ascii_lowercase()
        .trim()
        .to_string()
}

/// 一只宠物是否命中筛选词：多个词用空格分隔，每个词都要命中名字、俗称或系列名之一。
pub fn pet_matches(pet: &PetEntry, query: &str) -> bool {
    let query = normalize(query);
    if query.is_empty() {
        return true;
    }
    let name = normalize(&pet.name);
    let series = normalize(&pet.series);
    let aliases: Vec<String> = pet.aliases.iter().map(|a| normalize(a)).collect();
    query.split_whitespace().all(|word| {
        name.contains(word) || series.contains(word) || aliases.iter().any(|a| a.contains(word))
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::{PetAtoms, PetPoint, PetRect};

    fn pet(id: i64, name: &str, aliases: &[&str], series: &str) -> PetEntry {
        PetEntry {
            pet_id: id,
            name: name.into(),
            aliases: aliases.iter().map(|s| s.to_string()).collect(),
            series: series.into(),
            atoms: PetAtoms::default(),
            size: 1,
            catch_difficulty: 0,
            spot_count: 1,
        }
    }

    fn spot(pet_id: i64, area: &str) -> PetSpot {
        PetSpot {
            pet_id,
            area_id: area.into(),
            number: 1,
            floor: 100,
            map_name: "萨伊那斯".into(),
            anchor: PetPoint { x: 10, y: 10 },
            rect: PetRect { x1: 0, y1: 0, x2: 20, y2: 20 },
            max_enemies: 2,
            steps: 26,
            also_level1: vec![],
        }
    }

    fn sample() -> PetCatalog {
        PetCatalog {
            catalog_version: "v".into(),
            pets: vec![
                pet(92, "扬奇洛斯", &["蓝人龙"], "人龙系"),
                pet(91, "利则诺顿", &["绿人龙"], "人龙系"),
                pet(234, "卡卡金宝", &["红蛙"], "毒蛙系"),
                pet(292, "克克洛斯", &["金鸡"], ""),
                pet(81, "卡达鲁卡斯", &["穿山甲", "滑板"], ""),
                pet(301, "奥卡洛斯", &["绿暴"], ""),
            ],
            spots: vec![spot(92, "a"), spot(92, "b"), spot(234, "c")],
        }
    }

    #[test]
    fn match_by_name_alias_and_series() {
        let p = pet(92, "扬奇洛斯", &["蓝人龙"], "人龙系");
        assert!(pet_matches(&p, "扬奇"));
        assert!(pet_matches(&p, "蓝人龙"));
        assert!(pet_matches(&p, "人龙"));
        assert!(!pet_matches(&p, "红蛙"));
    }

    #[test]
    fn match_multi_word_needs_all_words() {
        let p = pet(92, "扬奇洛斯", &["蓝人龙"], "人龙系");
        assert!(pet_matches(&p, "蓝 人龙"));
        assert!(!pet_matches(&p, "蓝 红蛙"));
    }

    #[test]
    fn match_ignores_case_and_full_width() {
        let p = pet(81, "卡达鲁卡斯", &["穿山甲", "滑板"], "");
        assert!(pet_matches(&p, "滑板"));
        assert!(pet_matches(&pet(1, "ABC", &[], ""), "ａｂｃ"));
        assert!(pet_matches(&pet(1, "ABC", &[], ""), "abc"));
        assert!(pet_matches(&p, "　"));
    }

    #[test]
    fn filter_hits_expected_pets() {
        let mut d = PetDir::default();
        d.install(sample());
        d.open();
        d.filter = "人龙".into();
        assert_eq!(d.visible_pets().len(), 2);
        d.filter = "红蛙".into();
        assert_eq!(d.visible_pets()[0].name, "卡卡金宝");
        d.filter = "金鸡".into();
        assert_eq!(d.visible_pets()[0].pet_id, 292);
        d.filter = "滑板".into();
        assert_eq!(d.visible_pets()[0].pet_id, 81);
        d.filter = "没有这个".into();
        assert!(d.visible_pets().is_empty());
    }

    #[test]
    fn esc_clears_filter_then_leaves_layers() {
        let mut d = PetDir::default();
        d.install(sample());
        d.open();
        d.start_typing();
        d.type_char('人');
        d.commit_filter();
        // L1 有筛选词：Esc 先清空，不退出。
        assert_eq!(d.back(), PetAction::None);
        assert!(d.filter.is_empty());
        assert!(d.is_open());
        // 没有筛选词：Esc 离开目录。
        assert_eq!(d.back(), PetAction::Leave);
        assert!(!d.is_open());
    }

    #[test]
    fn enter_walks_three_levels_and_back() {
        let mut d = PetDir::default();
        d.install(sample());
        d.open();
        d.filter = "人龙".into();
        assert_eq!(d.enter(), PetAction::Descend); // 扬奇洛斯 → L2
        assert_eq!(d.level(), &PetLevel::Spots { pet_id: 92 });
        assert_eq!(d.enter(), PetAction::Descend); // 第一个遇敌点 → L3
        assert!(matches!(d.level(), PetLevel::Actions { pet_id: 92, spot: 0 }));
        // L3 的“取消”回到 L2，不发请求。
        d.navigate(TravelNav::End);
        assert_eq!(d.cursor(), 3);
        assert_eq!(d.enter(), PetAction::Descend);
        assert_eq!(d.level(), &PetLevel::Spots { pet_id: 92 });
        // 逐层 Esc：L2 → L1，筛选词保留。
        assert_eq!(d.back(), PetAction::Descend);
        assert_eq!(d.level(), &PetLevel::Pets);
        assert_eq!(d.filter, "人龙");
    }

    #[test]
    fn go_carries_mode_and_spot() {
        let mut d = PetDir::default();
        d.install(sample());
        d.open();
        d.enter();
        d.enter();
        d.navigate(TravelNav::Down);
        d.navigate(TravelNav::Down); // “只传送不遇敌”
        match d.enter() {
            PetAction::Go { pet_id, spot, mode } => {
                assert_eq!(pet_id, 92);
                assert_eq!(spot.area_id, "a");
                assert_eq!(mode, "travel");
            }
            other => panic!("期望 Go，实际 {other:?}"),
        }
    }
}
