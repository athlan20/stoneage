//! 页面的方向键、Tab、Enter（STAGE-113，`docs/interaction.md`「页面选择」「按键表」）：没有模态层时接手。
//! 输入行为空时 `←→` 切菜单页（循环），`↑↓` `Tab` `Home` `End` `Enter` 交给页面的选择（`select.rs`）；
//! `PgUp` `PgDn` 输入行有字时也作用。页面没有能选的项时返回 false，键照常交给原来的处理（`↑↓` 翻历史等）。
//! 焦点在详情栏（任务攻略、百科词条）时 `↑↓` 等键归详情滚动（`scroll.rs`）；命令执行和 Esc 之后焦点落到当前层的栏（`land_focus`）。

use crossterm::event::{KeyCode, KeyEvent};

use super::cursor::PAGE_STEP;
use super::msgview;
use super::scroll;
use super::select::{self, Column, Nav, OnEnter};
use super::wiki::hit_key;
use super::world::{BAG_MENU, CHAR_MENU, MESSAGES_MENU, PETS_MENU, QUEST_MENU, SOCIAL_MENU, WIKI_MENU, WorldState};
use super::App;

impl App {
    /// 没有模态层（战斗、弹窗、确认）时的页面按键。返回 true 表示已处理；false 交给输入行和原来的按键。
    pub(super) fn page_select_key(&mut self, key: KeyEvent) -> bool {
        // 战斗、弹窗、确认开着时，方向键和 Tab 归它们，不切页（`docs/interaction.md`「按键分层」）。
        if self
            .world
            .as_ref()
            .is_some_and(|w| w.battle_front() || w.npc_window.is_some() || w.confirm.is_some())
        {
            return false;
        }
        // 快捷传送页有自己的一层导航（STAGE-161）：方向键、Enter、Esc 先归它。
        if self.travel_key(key) {
            return true;
        }
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        let empty = true; // 页面焦点不受已收起的命令草稿影响。
        // 社交页设置栏：空格切换开关（同 Enter，STAGE-276）；命令草稿里的空格照常是文字。
        if key.code == KeyCode::Char(' ')
            && world.input.is_empty()
            && world.menu == SOCIAL_MENU
            && super::social_settings::on_settings_column(world)
        {
            self.social_toggle();
            return true;
        }
        // 「消息」页没有选择栏：输入行为空时 `↑↓` 逐行滚动（STAGE-116，`docs/interaction.md`「按键表」）；有字时仍是历史。
        if world.menu == MESSAGES_MENU && empty && matches!(key.code, KeyCode::Up | KeyCode::Down) {
            let down = key.code == KeyCode::Down;
            if let Some(world) = self.world.as_mut() {
                msgview::line(&self.log, &mut world.msg_view, down);
            }
            return true;
        }
        let columns = select::page_columns(world);
        let nav = match key.code {
            KeyCode::Left | KeyCode::Right if empty => {
                if key.code==KeyCode::Right {Nav::Tab}else{Nav::BackTab}
            }
            KeyCode::Up if empty => Nav::Up,
            KeyCode::Down if empty => Nav::Down,
            KeyCode::PageUp => Nav::PageUp,
            KeyCode::PageDown => Nav::PageDown,
            KeyCode::Home if empty => Nav::Home,
            KeyCode::End if empty => Nav::End,
            KeyCode::Tab if empty => Nav::Tab,
            KeyCode::BackTab if empty => Nav::BackTab,
            KeyCode::Enter if empty => return self.page_enter(&columns),
            _ => return false,
        };
        let Some(world) = self.world.as_mut() else {
            return false;
        };
        if select::on_detail(&columns, &world.select[world.menu]) && scroll::detail_nav(world, nav) {
            return true;
        }
        let page = page_rows(world);
        let moved = select::navigate_page(&columns, &mut world.select[world.menu], nav, page);
        if moved && !matches!(nav, Nav::Tab | Nav::BackTab) {
            self.follow_detail();
        }
        moved
    }

    /// 详情并排（宽屏）时，列表的选中项变了，详情跟着换成这一条（`docs/ui.md`「页面选择」）。
    /// 只在焦点栏是列表时跟：焦点在详情栏时 `↑↓` 是滚动。窄屏时详情替换列表，选中项换了也不用跟。
    fn follow_detail(&mut self) {
        let Some(world) = self.world.as_ref() else {
            return;
        };
        if world.is_narrow() {
            return;
        }
        let columns = select::page_columns(world);
        let Some((column, index)) = select::locate(&columns, &world.select[world.menu]) else {
            return;
        };
        if columns[column].detail {
            return;
        }
        let key = columns[column].items[index].key;
        match world.menu {
            QUEST_MENU => {
                let Ok(no) = u32::try_from(key) else {
                    return;
                };
                if world.quest_guide.as_ref().is_some_and(|guide| guide.no != no) {
                    self.quest_guide_command(no, "");
                }
            }
            WIKI_MENU => {
                let Some(open) = world.wiki.open.as_ref().map(|o| o.hit) else {
                    return;
                };
                if hit_key(open) == key {
                    return;
                }
                if let Some(index) = world.wiki.hits.iter().position(|h| hit_key(*h) == key) {
                    self.wiki_command(&format!("open {}", index + 1));
                }
            }
            _ => {}
        }
    }

    /// Enter：选中项有命令才有动作。查看类等价敲了那条命令，不回显、不进历史；操作类只填进输入行，光标在末尾，不执行；
    /// 背包页打开对象动作菜单（STAGE-132）。
    fn page_enter(&mut self, columns: &[Column]) -> bool {
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        let Some(on_enter) = select::enter(columns, &world.select[world.menu]) else {
            return false;
        };
        match on_enter {
            OnEnter::Run(cmd) => {
                self.run_command(&cmd, &cmd, false);
                self.land_focus();
            }
            OnEnter::Fill(cmd) => {
                if let Some(world) = self.world.as_mut() {
                    world.input.set(&cmd);
                    world.history.reset();
                }
            }
            OnEnter::Open => {
                self.open_object_actions();
            }
        }
        true
    }

    /// `OnEnter::Open`：按页面分发到对应的对象动作（STAGE-132 背包、STAGE-133 宠物与角色）。
    fn open_object_actions(&mut self) {
        match self.world.as_ref().map(|w| w.menu) {
            Some(BAG_MENU) => {
                self.bag_open_actions();
            }
            Some(PETS_MENU) => {
                self.pet_open_actions();
            }
            Some(CHAR_MENU) => {
                self.char_open_addpoint();
            }
            Some(SOCIAL_MENU) => {
                self.social_enter();
            }
            _ => {}
        }
    }

    /// Esc（输入行为空）：任务页关攻略，百科页照 `wiki back` 退一层，焦点随之落到该在的栏；退掉了返回 true。
    /// 这一页没有页内层可退时返回 false，交给导航栈往上一层退（STAGE-131）。
    pub(super) fn page_back(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        match world.menu {
            QUEST_MENU if world.quest_guide.is_some() => {
                if let Some(world) = self.world.as_mut() {
                    world.quest_guide = None;
                }
            }
            WIKI_MENU if world.wiki.open.is_some() || world.wiki.category.is_some() || !world.wiki.query.is_empty() => {
                self.wiki_command("back");
            }
            _ => return false,
        }
        self.land_focus();
        true
    }

    /// 命令执行或 Esc 之后，焦点落到当前层该在的那一栏：任务页打开攻略（窄屏）落到攻略栏；
    /// 百科页按打开的层落到分类、词条或详情栏。
    pub(super) fn land_focus(&mut self) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        let columns = select::page_columns(world);
        for name in landing(world) {
            if select::focus_named(&columns, &mut world.select[world.menu], name) {
                break;
            }
        }
    }
}

/// 一次 `PgUp` `PgDn` 翻过的项数：列表页是它可见的行数（翻一屏），其他页用默认步长。
fn page_rows(world: &WorldState) -> usize {
    match world.menu {
        QUEST_MENU => world.quest_rows,
        WIKI_MENU => world.wiki.list_rows,
        _ => PAGE_STEP,
    }
}

/// 当前层应该落在哪些栏（按优先级，栏名）：要的栏没有能选的项时（如打开的词条不在列表里）落到下一个。
/// 没有这种规定的页返回空。
fn landing(world: &WorldState) -> Vec<&'static str> {
    match world.menu {
        QUEST_MENU if world.quest_guide.is_some() && world.is_narrow() => vec!["攻略"],
        QUEST_MENU if world.quest_guide.is_some() => vec!["任务", "攻略"],
        QUEST_MENU => vec!["任务"],
        WIKI_MENU => {
            let w = &world.wiki;
            match w.open {
                Some(_) if world.is_narrow() => vec!["详情", "词条"],
                Some(_) => vec!["词条", "详情"],
                None if w.category.is_some() || !w.query.is_empty() => vec!["词条", "分类"],
                None => vec!["分类"],
            }
        }
        _ => Vec::new(),
    }
}

/// 提示行的 `Esc 返回`：有可退的层时写（任务攻略开着，或百科还没回到分类目录）。
pub(super) fn can_back(world: &WorldState) -> bool {
    match world.menu {
        QUEST_MENU => world.quest_guide.is_some(),
        WIKI_MENU => world.wiki.open.is_some() || world.wiki.category.is_some() || !world.wiki.query.is_empty(),
        _ => false,
    }
}
