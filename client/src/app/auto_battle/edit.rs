//! 设置页的光标、草稿与按键（STAGE-173「入口与交互」）。
//!
//! 打开时把当前方案复制成草稿，改的都是草稿；「保存并应用」才写回方案文件并让托管读到新设置。
//! 有改动按 Esc 会问「继续编辑 / 保存并应用 / 放弃改动」，默认停在「继续编辑」上——
//! 玩家误按 Esc 不会丢东西。
//!
//! 编辑器状态放在 `WorldState`（不是 `SystemState`）：切菜单会把 `SystemState` 整个重置，
//! 草稿不能跟着没。按键处理走“先 take 出来、改完放回”，避免同时借 `world` 和 `App` 的其它字段。

use crossterm::event::{KeyCode, KeyEvent};

use super::drop_ask;
use super::fields::{self, Context, Field};
use super::capture_log_view::{self, LogView};
use super::level_input::{self, LevelEdit};
use super::names_input::{self, NameEdit};
use super::store::Store;
use super::AutoProfile;
use crate::app::{App, TextInput};

/// 从哪打开的设置页（STAGE-173）：决定 Esc 之后退到哪。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Origin {
    /// 顶层「自动战斗」页。
    TopPage,
    /// 系统页的那一项。
    SystemPage,
    /// 战斗控制面板（Tab）里的那一项。
    Panel,
}

/// 编辑器里的确认框。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Ask {
    /// 有改动时按 Esc：继续编辑 / 保存并应用 / 放弃改动。
    Exit,
    /// 删除方案。
    Delete,
    /// 战后自动丢弃从关切到开（STAGE-260）：默认「取消」，确认后才写进草稿。
    DropOn,
}

#[derive(Debug)]
pub struct Editor {
    pub column: usize,
    pub row: usize,
    pub draft: AutoProfile,
    pub dirty: bool,
    pub ask: Option<Ask>,
    /// 确认框里的光标：0 是默认项（继续编辑 / 取消）。
    pub ask_choice: usize,
    /// 正在输入新方案的名字。
    pub naming: Option<TextInput>,
    /// 正在编辑的名单（名单类设置按 Enter 打开，见 `names_input`）。
    pub names: Option<NameEdit>,
    /// 正在手输的等级上下限（按 Enter 打开，见 `level_input`）。
    pub level: Option<LevelEdit>,
    /// 抓宠丢弃记录页（STAGE-264 第 5 条），打开时盖住设置行。
    pub log: Option<LogView>,
    /// 上一步的提示（选不了的原因、保存结果都写这里，界面显示在底部）。
    pub hint: Option<String>,
    /// 从哪进来的：退出时按它决定回到哪一层。
    pub origin: Origin,
}

impl Editor {
    pub fn new(p: &AutoProfile, origin: Origin) -> Self {
        Self {
            column: 0,
            row: 0,
            draft: p.clone(),
            dirty: false,
            ask: None,
            ask_choice: 0,
            naming: None,
            names: None,
            level: None,
            log: None,
            hint: None,
            origin,
        }
    }

    pub fn field(&self) -> Field {
        let f = fields::column_fields(self.column);
        f[self.row.min(f.len() - 1)]
    }

    fn change_column(&mut self, dir: i64) {
        let n = fields::columns().len() as i64;
        self.column = (self.column as i64 + dir).rem_euclid(n) as usize;
        self.row = 0;
    }

    fn move_row(&mut self, dir: i64) {
        let len = fields::column_fields(self.column).len() as i64;
        self.row = (self.row as i64 + dir).clamp(0, len - 1) as usize;
    }
}

/// 不在编辑状态时返回 false，让按键继续往下传。
impl App {
    /// 当前角色的方案（没加载到时给一份默认值，只读）。
    pub fn auto_profile(&self) -> &AutoProfile {
        self.auto_store.as_ref().map_or(&super::store::FALLBACK, |s| s.active())
    }

    /// 托管出招前等多久（方案里的「出招延迟」）。
    pub fn auto_delay(&self) -> std::time::Duration {
        self.auto_profile().playback.delay()
    }

    /// 玩家动手之后托管让位多久（方案里的「手动让位」）。
    pub fn auto_hold(&self) -> std::time::Duration {
        self.auto_profile().playback.manual_hold()
    }

    /// 打开设置页（STAGE-173）：顶层「自动战斗」页、系统页那一项、Tab 战斗控制面板三处共用。
    /// 只建草稿，不动导航层——从哪进来的、Esc 退到哪，由各入口自己决定。
    pub fn open_auto_settings(&mut self, origin: Origin) {
        if let Some(err) = self.auto_store.as_ref().and_then(|s| s.load_error.clone()) {
            self.log.system(format!("自动战斗方案：{err}"));
        }
        let draft = self.auto_profile().clone();
        if let Some(w) = self.world.as_mut() {
            w.auto_edit = Some(Editor::new(&draft, origin));
        }
    }

    /// 设置页的按键。返回 `true` 表示这一下已经被设置页吃掉。
    /// **退出**（无改动按 Esc、或确认框选「保存/放弃」后离开）只把 `auto_edit` 清掉，
    /// 退到哪一层由调用方决定：系统页回行表、顶层页回游戏菜单。
    pub fn auto_settings_key(&mut self, key: KeyEvent) -> bool {
        let ctx = self.auto_context();
        let Some(mut ed) = self.world.as_mut().and_then(|w| w.auto_edit.take()) else {
            return false;
        };
        let keep = self.editor_key(&mut ed, key, &ctx);
        if keep {
            if let Some(w) = self.world.as_mut() {
                w.auto_edit = Some(ed);
            }
            return true;
        }
        self.close_auto_settings(ed.origin);
        true
    }

    /// 关掉设置页（STAGE-173）：退到哪由来源决定，战斗本身不受影响。
    fn close_auto_settings(&mut self, origin: Origin) {
        let in_battle = self.world.as_ref().is_some_and(|w| w.battle.is_some());
        if let Some(w) = self.world.as_mut() {
            w.auto_edit = None;
            match origin {
                Origin::TopPage => {}
                Origin::SystemPage => w.system.step = None,
                Origin::Panel => {
                    w.system.step = None;
                    // 面板只为这一场战斗开；战斗已经结束就把面板一并收起。
                    if !in_battle {
                        w.system.battle_open = false;
                    }
                }
            }
        }
        if origin == Origin::TopPage {
            self.leave_page();
        }
    }


    /// 返回 `false` 表示退出设置页。
    fn editor_key(&mut self, ed: &mut Editor, key: KeyEvent, ctx: &Context) -> bool {
        // 名单编辑框、等级手输框：先把字收完，行表上的按键不抢。
        if ed.names.is_some() {
            names_input::names_key(ed, key);
            return true;
        }
        if ed.level.is_some() {
            level_input::level_key(ed, key);
            return true;
        }
        // 抓宠丢弃记录页（STAGE-264）：自己收按键，Esc 退回设置页。
        if let Some(log) = ed.log.as_mut() {
            if !capture_log_view::key(log, key) {
                ed.log = None;
            }
            return true;
        }
        // 命名输入框：先把文字收完。
        if let Some(input) = ed.naming.as_mut() {
            match key.code {
                KeyCode::Enter => {
                    let name = input.text().trim().to_string();
                    ed.naming = None;
                    if name.is_empty() {
                        ed.hint = Some("方案名不能为空".into());
                    } else {
                        self.add_profile(&name);
                        ed.draft = self.auto_profile().clone();
                        ed.dirty = true;
                        ed.hint = Some(format!("已新建方案「{name}」，保存并应用后才生效"));
                    }
                }
                KeyCode::Esc => {
                    ed.naming = None;
                    ed.hint = Some("已取消新建".into());
                }
                _ => {
                    input.handle_key(&key);
                }
            }
            return true;
        }
        // 确认框：←→ 选项，Enter 执行，Esc 回编辑。
        if let Some(ask) = ed.ask {
            let n = if ask == Ask::Exit { 3 } else { 2 };
            // 确认框的选项：ask_choice 0 是默认项（取消 / 继续编辑），1 是确认。
            if ask == Ask::DropOn {
                drop_ask::key(ed, key.code);
                return true;
            }
            match key.code {
                KeyCode::Left | KeyCode::Right | KeyCode::Tab | KeyCode::BackTab => {
                    ed.ask_choice = (ed.ask_choice + 1) % n;
                }
                KeyCode::Enter => match (ask, ed.ask_choice) {
                    (Ask::Exit, 1) => {
                        ed.ask = None;
                        match self.save_auto_settings(ed.draft.clone()) {
                            Ok(()) => {
                                ed.dirty = false;
                                ed.hint = Some("已保存并应用".into());
                            }
                            Err(e) => {
                                ed.dirty = true;
                                ed.hint = Some(e);
                            }
                        }
                    }
                    (Ask::Exit, 2) => {
                        ed.ask = None;
                        ed.dirty = false;
                        let p = self.auto_profile().clone();
                        ed.draft = p;
                        ed.hint = Some("已放弃改动".into());
                        self.apply_playback();
                    }
                    (Ask::Exit, _) => {
                        ed.ask = None;
                        ed.hint = Some("继续编辑".into());
                    }
                    (Ask::Delete, 1) => {
                        ed.ask = None;
                        ed.hint = Some(self.delete_profile());
                        let p = self.auto_profile().clone();
                        ed.draft = p;
                        ed.dirty = true;
                    }
                    (Ask::Delete, _) => {
                        ed.ask = None;
                        ed.hint = Some("已取消删除".into());
                    }
                    (Ask::DropOn, _) => ed.ask = None,
                },
                KeyCode::Esc => {
                    ed.ask = None;
                    ed.hint = Some("继续编辑".into());
                }
                _ => {}
            }
            return true;
        }
        match key.code {
            KeyCode::Esc => {
                if ed.dirty {
                    ed.ask = Some(Ask::Exit);
                    ed.ask_choice = 0;
                } else {
                    return false;
                }
            }
            KeyCode::Tab => ed.change_column(1),
            KeyCode::BackTab => ed.change_column(-1),
            KeyCode::Up => ed.move_row(-1),
            KeyCode::Down => ed.move_row(1),
            KeyCode::Home => ed.row = 0,
            KeyCode::End => ed.row = fields::column_fields(ed.column).len() - 1,
            KeyCode::Left | KeyCode::Right => {
                let field = ed.field();
                if let Some(why) = field.blocked(&ed.draft, ctx) {
                    ed.hint = Some(why.to_string());
                    return true;
                }
                let dir = i64::from(if key.code == KeyCode::Left { -1 } else { 1 });
                match field {
                    Field::ProfilePick => {
                        if let Some(s) = self.auto_store.as_mut() {
                            s.profiles.step(dir);
                        }
                        let p = self.auto_profile().clone();
                        ed.draft = p;
                        ed.dirty = false;
                        ed.hint = Some("已切换方案（未保存的改动已丢弃）".into());
                        self.apply_playback();
                    }
                    // 战后丢弃从关切到开：先问一句，默认取消（STAGE-260）。
                    Field::PostDrop if !ed.draft.recover.post_drop_item => {
                        ed.ask = Some(Ask::DropOn);
                        ed.ask_choice = 0;
                    }
                    _ => {
                        let mut draft = ed.draft.clone();
                        let msg = field.adjust(&mut draft, dir, ctx);
                        ed.draft = draft;
                        ed.dirty = true;
                        ed.hint = msg;
                    }
                }
            }
            KeyCode::Enter => {
                let field = ed.field();
                if let Some(why) = field.blocked(&ed.draft, ctx) {
                    ed.hint = Some(why.to_string());
                    return true;
                }
                match field {
                    Field::ProfileNew => {
                        let mut input = TextInput::new(24);
                        input.set(&format!("方案 {}", self.profile_count() + 1));
                        ed.naming = Some(input);
                    }
                    Field::ProfileDelete => {
                        ed.ask = Some(Ask::Delete);
                        ed.ask_choice = 0;
                    }
                    Field::SaveApply => match self.save_auto_settings(ed.draft.clone()) {
                        Ok(()) => {
                            ed.dirty = false;
                            ed.hint = Some("已保存并应用".into());
                        }
                        Err(e) => ed.hint = Some(e),
                    },
                    Field::DiscardDraft => {
                        let p = self.auto_profile().clone();
                        ed.draft = p;
                        ed.dirty = false;
                        ed.hint = Some("已放弃改动".into());
                        self.apply_playback();
                    }
                    Field::FleeNames | Field::CaptureNames | Field::MpItems => names_input::open(ed, field),
                    Field::CaptureLvMin | Field::CaptureLvMax => level_input::open(ed, field),
                    Field::CaptureLog => ed.log = Some(LogView::open(self.auto_store.as_ref())),
                    _ => ed.hint = Some(format!("{}：用 ←→ 调整", field.label())),
                }
            }
            _ => {}
        }
        true
    }

    /// 把方案里的播放偏好同步到运行时（系统页的「默认回放倍速」也读它）。
    pub fn apply_playback(&mut self) {
        // 回放速度只在系统页设（STAGE-173），方案里只带“结果窗口自动继续”。
        self.auto_continue = self.auto_profile().playback.auto_continue;
    }

    /// 现在的角色现状（可选精灵、出战宠技能、等级、队伍人数）。
    pub fn auto_context(&self) -> Context {
        let Some(w) = self.world.as_ref() else {
            return Context::default();
        };
        let magics =
            w.entry.status.magic.iter().map(|m| (m.slot, m.name.clone(), m.mp, m.target_type)).collect();
        let battle_pet = w.pets.pets.iter().find(|p| p.state == "battle");
        let (pet_name, pet_skills) = match battle_pet {
            Some(p) => (p.name.clone(), p.skills.iter().map(|s| (s.slot, s.name.clone())).collect()),
            None => (String::new(), Vec::new()),
        };
        Context {
            magics,
            pet_skills,
            pet_name,
            player_level: w.entry.status.level,
            party_size: w.entry.party.as_ref().map_or(1, |p| p.members.len()),
            turn: w.battle.as_ref().map(|b| b.turn),
            // 连续遇敌与选区是只读展示（M09）：值直接取真实状态，不另存一份。
            encounter_on: w.entry.encounter,
            encounter_busy: w.halt.busy(),
            encounter_area: w.scene.areas.get(w.scene.area_index).map(|a| a.label()).unwrap_or_default(),
        }
    }

    /// 进入世界后按“服务器 + 角色”打开方案文件（STAGE-173 M15：切角色互不串）。
    pub fn load_auto_store(&mut self, server: &str, character: &str) {
        self.auto_store = Some(Store::open(server, character));
        self.apply_playback();
        if let Some(err) = self.auto_store.as_ref().and_then(|s| s.load_error.clone()) {
            self.log.system(format!("自动战斗方案：{err}"));
        }
    }
}
