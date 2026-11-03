//! STAGE-136：任务对象动作、正式 NPC 候选和百科搜索表单。
use super::select::{self, PageFocus};
use super::session::Pending;
use super::{App, Layer, MESSAGES_MENU, QUEST_MENU, TextInput, WIKI_MENU};
use crate::proto::{NpcMatch, Quest};
use crossterm::event::{KeyCode, KeyEvent};
use serde::Deserialize;
use serde_json::{Value, json};

#[derive(Debug, Clone, Deserialize)]
pub struct QuestNpc {
    #[serde(flatten)]
    pub npc: NpcMatch,
    pub relation: String,
}

#[derive(Debug)]
pub enum BrowseStep {
    Quest {
        quest: Quest,
        index: usize,
    },
    Npcs {
        quest: Quest,
        items: Vec<QuestNpc>,
        reason: String,
        index: usize,
        loading: bool,
    },
    Search {
        input: TextInput,
    },
}

#[derive(Debug, Default)]
pub struct BrowseState {
    pub query: u64,
    pub quest_visit: Option<Quest>,
    pub step: Option<BrowseStep>,
    pub quest_focus: [PageFocus; 3],
    pub quest_scroll: [usize; 3],
}

impl App {
    pub(super) fn quest_open_actions(&mut self) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let index = super::page_cursor(w, "任务").map_or(0, |(i, _)| i);
        let Some(q) = w.visible_quests().get(index).map(|q| (*q).clone()) else {
            return;
        };
        w.browse.step = Some(BrowseStep::Quest { quest: q, index: 0 });
        w.nav.push(Layer::Action);
    }

    pub(super) fn wiki_search_form(&mut self) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        w.browse.step = Some(BrowseStep::Search {
            input: TextInput::with_text(100, &w.wiki.query),
        });
        w.nav.push(Layer::Form);
    }

    pub(super) fn on_quest_npcs(&mut self, no: u32, state: &str, query: u64, data: Value) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        if w.browse.query != query {
            return;
        }
        if !w
            .quests
            .quests
            .iter()
            .any(|q| q.no == no && q.state == state)
        {
            return;
        }
        let Some(BrowseStep::Npcs {
            quest,
            items,
            reason,
            loading,
            ..
        }) = w.browse.step.as_mut()
        else {
            return;
        };
        if quest.no != no || quest.state != state || data["state"].as_str() != Some(state) {
            return;
        }
        *items = serde_json::from_value(data["npcs"].clone()).unwrap_or_default();
        *reason = data["reason"]
            .as_str()
            .unwrap_or("没有可靠的 NPC 对照，请搜索 NPC")
            .to_string();
        *loading = false;
    }

    pub(super) fn browse_key(&mut self, key: KeyEvent) -> bool {
        let Some(w) = self.world.as_ref() else {
            return false;
        };
        if w.battle_front() || w.npc_window.is_some() || w.confirm.is_some() {
            return false;
        }
        if w.nav.is(&Layer::Detail) && w.quest_guide.is_some() {
            if key.code == KeyCode::Char('/') {
                self.world.as_mut().unwrap().nav.push(Layer::Command);
            } else if key.code == KeyCode::Esc {
                let w = self.world.as_mut().unwrap();
                w.quest_guide = None;
                w.nav.pop();
                w.select[QUEST_MENU].column = 0;
                w.select[QUEST_MENU].pinned = true;
            } else {
                self.browse_scroll(key);
            }
            return true;
        }
        if w.browse.step.is_some() {
            return self.browse_step_key(key);
        }
        if !matches!(w.nav.top(), Layer::Page(_) | Layer::Detail)
            || ![QUEST_MENU, WIKI_MENU, MESSAGES_MENU].contains(&w.menu)
        {
            return false;
        }
        if key.code == KeyCode::Char('/') {
            self.world.as_mut().unwrap().nav.push(Layer::Command);
            return true;
        }
        let menu = w.menu;
        if menu == WIKI_MENU && key.code == KeyCode::Tab && w.wiki.open.is_none() {
            self.wiki_search_form();
            return true;
        }
        if menu == MESSAGES_MENU {
            if key.code == KeyCode::Esc {
                self.leave_page();
                return true;
            }
            match key.code {
                KeyCode::Home | KeyCode::End => self.scroll_edge(key.code == KeyCode::End),
                KeyCode::Up | KeyCode::Down => {
                    let w=self.world.as_mut().unwrap(); super::msgview::line(&self.log,&mut w.msg_view,key.code==KeyCode::Down);
                }
                KeyCode::PageUp | KeyCode::PageDown => {
                    let w=self.world.as_mut().unwrap(); super::msgview::page(&self.log,&mut w.msg_view,key.code==KeyCode::PageDown);
                }
                _ => {}
            }
            return true;
        }
        if key.code == KeyCode::Esc {
            if menu == WIKI_MENU
                && (w.wiki.open.is_some() || w.wiki.category.is_some() || !w.wiki.query.is_empty())
            {
                self.wiki_command("back");
                self.land_focus();
                return true;
            }
            self.leave_page();
            return true;
        }
        if matches!(key.code, KeyCode::Left | KeyCode::Right) && menu == QUEST_MENU {
            self.quest_cycle_filter(key.code == KeyCode::Right);
            return true;
        }
        if key.code == KeyCode::Enter {
            if menu == QUEST_MENU {
                self.quest_open_actions();
            } else {
                self.wiki_browse_enter();
            }
            return true;
        }
        let nav = match key.code {
            KeyCode::Up => select::Nav::Up,
            KeyCode::Down => select::Nav::Down,
            KeyCode::PageUp => select::Nav::PageUp,
            KeyCode::PageDown => select::Nav::PageDown,
            KeyCode::Home => select::Nav::Home,
            KeyCode::End => select::Nav::End,
            // 数字键不在这里吃掉：交给世界层按页号切页（兼容入口，docs/interaction.md 功能页）。
            // 原来 `_ => return true` 会让任务、百科、消息页上的数字键没有任何效果。
            KeyCode::Char('0'..='9') => return false,
            _ => return true,
        };
        let w = self.world.as_mut().unwrap();
        if w.wiki.open.is_some() && menu == WIKI_MENU {
            w.select[WIKI_MENU].column = 2;
            w.select[WIKI_MENU].pinned = true;
            super::scroll::detail_nav(w, nav);
            return true;
        }
        let columns = select::page_columns(w);
        let column = if menu == WIKI_MENU && (w.wiki.category.is_some() || !w.wiki.query.is_empty())
        {
            1
        } else {
            0
        };
        w.select[menu].column = column;
        w.select[menu].pinned = true;
        let rows = if menu == QUEST_MENU {
            w.quest_rows
        } else {
            w.wiki.list_rows
        }
        .max(1);
        select::navigate_page(&columns, &mut w.select[menu], nav, rows);
        true
    }

    fn browse_scroll(&mut self, key: KeyEvent) {
        let nav = match key.code {
            KeyCode::Up => select::Nav::Up,
            KeyCode::Down => select::Nav::Down,
            KeyCode::PageUp => select::Nav::PageUp,
            KeyCode::PageDown => select::Nav::PageDown,
            KeyCode::Home => select::Nav::Home,
            KeyCode::End => select::Nav::End,
            _ => return,
        };
        if let Some(w) = self.world.as_mut() {
            w.select[QUEST_MENU].column = 1;
            w.select[QUEST_MENU].pinned = true;
            super::scroll::detail_nav(w, nav);
        }
    }

    fn quest_cycle_filter(&mut self, forward: bool) {
        use super::quests::QuestFilter;
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let old = filter_index(w.quest_filter);
        w.browse.quest_focus[old] = w.select[QUEST_MENU].clone();
        w.browse.quest_scroll[old] = w.quest_scroll;
        let next = if forward {
            (old + 1) % 3
        } else {
            (old + 2) % 3
        };
        w.quest_filter = [QuestFilter::All, QuestFilter::Doing, QuestFilter::Done][next];
        w.quest_guide = None;
        w.select[QUEST_MENU] = w.browse.quest_focus[next].clone();
        w.quest_scroll = w.browse.quest_scroll[next];
    }

    fn wiki_browse_enter(&mut self) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        if w.wiki.open.is_some() {
            return;
        }
        let home = w.wiki.category.is_none() && w.wiki.query.is_empty();
        if home {
            let i = super::page_cursor(w, "分类").map_or(0, |(i, _)| i);
            if i == crate::wiki::wiki().categories().len() {
                self.wiki_search_form();
            } else {
                self.wiki_category(&(i + 1).to_string());
                self.land_focus();
            }
        } else {
            let i = super::page_cursor(w, "词条").map_or(0, |(i, _)| i);
            if i == w.wiki.hits.len() {
                self.wiki_search_form();
            } else {
                self.wiki_open(&(i + 1).to_string());
                self.land_focus();
                let w = self.world.as_mut().unwrap();
                w.select[WIKI_MENU].column = 2;
                w.select[WIKI_MENU].pinned = true;
            }
        }
    }

    fn browse_step_key(&mut self, key: KeyEvent) -> bool {
        let Some(w) = self.world.as_mut() else {
            return false;
        };
        if key.code == KeyCode::Esc {
            let old = w.browse.step.take();
            w.nav.pop();
            if let Some(BrowseStep::Npcs { quest, .. }) = old {
                w.browse.step = Some(BrowseStep::Quest { quest, index: 1 });
            }
            return true;
        }
        let Some(step) = w.browse.step.as_mut() else {
            return false;
        };
        let (index, len) = match step {
            BrowseStep::Quest { index, .. } => (index, 3),
            BrowseStep::Npcs { index, items, .. } => (index, items.len() + 1),
            BrowseStep::Search { input } => {
                if key.code == KeyCode::Enter {
                    let q = input.text().to_string();
                    if q.trim().is_empty() {
                        self.log.error("请输入搜索词");
                        return true;
                    }
                    w.browse.step = None;
                    w.nav.pop();
                    self.wiki_find(&q);
                    self.land_focus();
                } else {
                    input.handle_key(&key);
                }
                return true;
            }
        };
        match key.code {
            KeyCode::Up => *index = index.saturating_sub(1),
            KeyCode::Down => *index = (*index + 1).min(len.saturating_sub(1)),
            KeyCode::Home => *index = 0,
            KeyCode::End => *index = len.saturating_sub(1),
            KeyCode::PageUp => *index = index.saturating_sub(10),
            KeyCode::PageDown => *index = (*index + 10).min(len.saturating_sub(1)),
            KeyCode::Enter => self.browse_activate(),
            _ => {}
        }
        true
    }

    fn browse_activate(&mut self) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        match w.browse.step.as_ref() {
            Some(BrowseStep::Quest { quest, index }) => {
                let q = quest.clone();
                if !w.quests.quests.contains(&q) {
                    self.log.error("任务已更新，请重新选择");
                    return;
                }
                match index {
                    0 => {
                        self.quest_guide_command(q.no, "");
                        self.land_focus();
                        let w = self.world.as_mut().unwrap();
                        w.nav.push(Layer::Detail);
                        w.select[QUEST_MENU].column = 1;
                        w.select[QUEST_MENU].pinned = true;
                    }
                    1 => {
                        w.browse.step = Some(BrowseStep::Npcs {
                            quest: q.clone(),
                            items: vec![],
                            reason: "正在查询…".into(),
                            index: 0,
                            loading: true,
                        });
                        w.nav.push(Layer::Action);
                        w.browse.query += 1;
                        let query = w.browse.query;
                        self.send(
                            "quest_npcs",
                            json!({"no":q.no}),
                            Pending::QuestNpcs {
                                no: q.no,
                                state: q.state,
                                query,
                            },
                        );
                    }
                    _ => {
                        self.close_browse();
                        self.scene_open_catalog();
                    }
                }
            }
            Some(BrowseStep::Npcs {
                quest,
                items,
                index,
                loading,
                ..
            }) => {
                if *loading {
                    return;
                }
                if !w.quests.quests.contains(quest) {
                    self.log.error("任务已更新，请重新选择");
                    return;
                }
                let Some(npc) = items.get(*index).map(|i| i.npc.clone()) else {
                    self.close_browse();
                    self.scene_open_catalog();
                    return;
                };
                let visit = quest.clone();
                self.close_browse();
                self.world.as_mut().unwrap().browse.quest_visit = Some(visit);
                if npc.floor == self.world.as_ref().unwrap().entry.position.floor {
                    self.open_npc(npc.id);
                } else {
                    let prompt = format!("{} · {}：传送后对话？", npc.map_name, npc.name);
                    self.ask_confirm(super::confirm::Confirm::NpcVisit(npc), &prompt);
                }
            }
            _ => {}
        }
    }

    fn close_browse(&mut self) {
        if let Some(w) = self.world.as_mut() {
            w.browse.step = None;
            while matches!(w.nav.top(), Layer::Action | Layer::Detail | Layer::Form) {
                w.nav.pop();
            }
        }
    }
}

fn filter_index(f: super::quests::QuestFilter) -> usize {
    match f {
        super::quests::QuestFilter::All => 0,
        super::quests::QuestFilter::Doing => 1,
        super::quests::QuestFilter::Done => 2,
    }
}
