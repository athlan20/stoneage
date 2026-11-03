//! 任务攻略（STAGE-89，`docs/ui.md`「任务页」）：`quest 编号` 把任务编号对到百科的攻略词条（`wiki/quest_map.rs`），
//! 生成详情行，任务页右边显示（窄屏时替换列表）。数据都在客户端本地，不发请求，agent 操控时也能用。

use super::quests;
use super::world::QUEST_MENU;
use super::App;
use crate::wiki::{DetailLine, LineStyle, quest_map, wiki};

/// 任务页打开的攻略：任务编号和名字、详情行、滚动位置（`scroll` 是顶部行号，渲染时收口）。
/// 滚动由页面的详情栏处理（`scroll.rs` 的 `detail_nav`）。
#[derive(Debug)]
pub struct QuestGuide {
    pub no: u32,
    pub name: String,
    pub lines: Vec<DetailLine>,
    pub scroll: usize,
    /// 详情区最近一次渲染的可见行数，翻页步进用。
    pub view_rows: usize,
}

/// 详情区还没渲染过时的默认可见行数。
const DEFAULT_ROWS: usize = 10;

impl App {
    /// `quest 编号 [wiki]`：只能查任务列表里有的任务（任务页上的那一条）。`wiki` 跳到百科页打开第一篇攻略。
    pub(super) fn quest_guide_command(&mut self, no: u32, tail: &str) {
        let Some(world) = self.world.as_ref() else { return };
        let Some(quest) = world.quests.quests.iter().find(|q| q.no == no) else {
            self.log.error(format!("任务列表里没有编号 {no} 的任务；输入 quest 看全部任务"));
            return;
        };
        let name = quest.name.clone();
        if tail == "wiki" {
            match quest_map::keys(no).first().cloned() {
                Some(key) => self.wiki_command(&format!("open {key}")),
                None => self.log.error(format!("任务【{name}】在百科里没有攻略")),
            }
            return;
        }
        let lines = self.guide_lines(no, &name);
        if let Some(world) = self.world.as_mut() {
            world.menu = QUEST_MENU;
            world.quest_guide = Some(QuestGuide { no, name, lines, scroll: 0, view_rows: DEFAULT_ROWS });
            quests::point_list(world, no);
        }
    }

    /// 攻略的详情行：任务名、进度（同一篇攻略有多个步骤时）、每篇词条的详情（多篇时标序号）。
    /// 没有攻略时是灰色提示和 `wiki find` 建议。
    fn guide_lines(&self, no: u32, name: &str) -> Vec<DetailLine> {
        let line = |style, text: String| DetailLine { style, text };
        let keys = quest_map::keys(no);
        if keys.is_empty() {
            let word = name.split_whitespace().next().unwrap_or(name);
            return vec![
                line(LineStyle::Title, name.to_string()),
                line(LineStyle::Dim, "百科里暂时没有这个任务的攻略。".to_string()),
                line(LineStyle::Dim, format!("可以试试：wiki find {word}")),
            ];
        }
        let mut lines = vec![line(LineStyle::Title, name.to_string())];
        if let Some((step, total)) = quest_map::step(no).filter(|(_, total)| *total > 1) {
            lines.push(line(LineStyle::Body, format!("当前进度：第 {step} 步 / 共 {total} 步（同一篇攻略的步骤，按任务编号排）")));
        }
        for (i, key) in keys.iter().enumerate() {
            if keys.len() > 1 {
                lines.push(line(LineStyle::Section, format!("攻略 {}/{}", i + 1, keys.len())));
            }
            match wiki().locate(key).and_then(|hit| wiki().entry(hit)) {
                Some(entry) => lines.extend(entry.detail_lines(self.unicode)),
                None => lines.push(line(LineStyle::Dim, format!("词条 {key} 读取失败"))),
            }
        }
        lines.push(line(LineStyle::Dim, format!("在百科里打开第一篇：quest {no} wiki")));
        lines
    }
}
