//! 「任务」页的数据与命令（STAGE-86，`docs/ui.md` 任务页）：`quests` 响应和 `quests_changed` 推送写进世界状态，
//! `quest` 命令切换显示范围（只看进行中、只看已完成、全部），PgUp/PgDn 翻页。纯查看，agent 操控时也能用。
//! `quest 编号` 打开任务攻略、`quest back` 关掉攻略（STAGE-89，攻略见 `quest_guide.rs`）。

use serde_json::Value;

use super::commands;
use super::select::{self, Column, Item, OnEnter};
use super::world::QUEST_MENU;
use super::{App, WorldState};
use crate::proto::{Quest, Quests};

/// 任务页的显示范围（`quest` 命令的参数）。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum QuestFilter {
    #[default]
    All,
    Doing,
    Done,
}

impl QuestFilter {
    /// 命令参数：不带参数或 all 是全部，doing 是进行中，done 是已完成；其他返回 None。
    pub fn parse(word: &str) -> Option<Self> {
        match word {
            "" | "all" => Some(Self::All),
            "doing" => Some(Self::Doing),
            "done" => Some(Self::Done),
            _ => None,
        }
    }

    /// 页面标题里显示的范围说明；全部时不写。
    pub fn label(self) -> Option<&'static str> {
        match self {
            Self::All => None,
            Self::Doing => Some("只看进行中"),
            Self::Done => Some("只看已完成"),
        }
    }

    fn keeps(self, state: &str) -> bool {
        match self {
            Self::All => true,
            Self::Doing => state == "doing",
            Self::Done => state == "done",
        }
    }
}

impl WorldState {
    /// 当前显示范围内的任务，保持服务端给的顺序（进行中在前，同状态按编号）。
    pub fn visible_quests(&self) -> Vec<&Quest> {
        self.quests.quests.iter().filter(|q| self.quest_filter.keeps(&q.state)).collect()
    }

    /// 进行中和已完成的任务数（页面标题用，不受显示范围影响）。
    pub fn quest_counts(&self) -> (usize, usize) {
        let doing = self.quests.quests.iter().filter(|q| q.state == "doing").count();
        (doing, self.quests.quests.len() - doing)
    }
}

/// 「任务」页的栏（`docs/interaction.md`「页面选择」）：列表，和打开的攻略（详情栏）。
/// 窄屏时攻略替换列表，列表这一栏不画出来，焦点只能在攻略栏上。
pub(super) fn columns(world: &WorldState) -> Vec<Column> {
    let guide = world.quest_guide.is_some();
    let items = world
        .visible_quests()
        .iter()
        .map(|q| Item {
            key: i64::from(q.no),
            usable: true,
            enter: Some(OnEnter::Run(format!("quest {}", q.no))),
        })
        .collect();
    vec![
        Column { name: "任务", items, shown: !(guide && world.is_narrow()), detail: false, default_key: None },
        Column { name: "攻略", items: vec![Item { key: 0, usable: true, enter: None }], shown: guide, detail: true, default_key: None },
    ]
}

/// 命令“看某一项”之后，列表的选中项跟到那一个任务上（`quest 编号`）。
pub(super) fn point_list(world: &mut WorldState, no: u32) {
    let columns = select::page_columns(world);
    select::point_named(&columns, &mut world.select[QUEST_MENU], "任务", i64::from(no));
}

impl App {
    /// `quests` 响应（进入世界后取一次）：整张列表直接替换。
    pub(super) fn on_quests(&mut self, data: Value) {
        let q: Quests = serde_json::from_value(data).unwrap_or_default();
        self.replace_quests(q);
    }

    fn replace_quests(&mut self, q: Quests) {
        if let Some(w) = self.world.as_mut() {
            w.quests = q;
            if matches!(w.browse.step, Some(super::BrowseStep::Quest { .. } | super::BrowseStep::Npcs { .. })) {
                w.browse.step = None;
                w.quest_guide = None;
                while matches!(w.nav.top(), super::Layer::Action | super::Layer::Detail | super::Layer::Form) { w.nav.pop(); }
            }
            if w.browse.quest_visit.as_ref().is_some_and(|q| !w.quests.quests.contains(q)) {
                w.browse.quest_visit = None;
                w.confirm = None;
                if w.nav.is(&super::Layer::Confirm) { w.nav.pop(); }
            }
        }
    }

    /// `quests_changed` 推送：整张列表替换，变化的任务写进消息栏。清除的任务已经不在列表里，不提示。
    pub(super) fn on_quests_changed(&mut self, data: Value) {
        let q: Quests = serde_json::from_value(data).unwrap_or_default();
        let notes: Vec<String> = q
            .changed
            .iter()
            .filter_map(|no| q.quests.iter().find(|t| t.no == *no))
            .map(|t| format!("任务【{}】{}", t.name, if t.state == "done" { "已完成" } else { "进行中" }))
            .collect();
        self.replace_quests(q);
        for note in notes {
            self.log.system(note);
        }
    }

    /// `quest [doing|done|all]`、`quest 编号 [wiki]`、`quest back`：切到「任务」页并设定显示范围或打开攻略。纯本地操作，不发请求。
    pub(super) fn quest_command(&mut self, rest: &str) {
        let rest = rest.trim();
        let (head, tail) = rest.split_once(char::is_whitespace).map_or((rest, ""), |(a, b)| (a, b.trim()));
        if matches!(rest, "back" | "返回") {
            if let Some(w) = self.world.as_mut() {
                w.quest_guide = None;
            }
            return;
        }
        if let Ok(no) = head.parse::<u32>()
            && (tail.is_empty() || tail == "wiki")
        {
            self.quest_guide_command(no, tail);
            return;
        }
        let Some(filter) = QuestFilter::parse(rest) else {
            self.log.error(commands::usage_of("quest"));
            return;
        };
        if let Some(w) = self.world.as_mut() {
            w.quest_filter = filter;
            w.quest_scroll = 0;
            w.quest_guide = None;
            w.menu = QUEST_MENU;
        }
    }
}
