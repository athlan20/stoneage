//! NPC 对话窗口（STAGE-69，`docs/ui.md`「服务端驱动的界面」）：服务端推 `npc_window`
//! 弹出居中窗口，`choose N` 或数字键回应，`Esc` 关闭。窗口是服务端的状态，客户端只
//! 显示它；本地的关闭动作都要发 `npc_close` 让服务端同步清状态（协议 5.36）。

use serde_json::json;

use super::App;
use super::session::Pending;
use crate::proto::NpcWindow;

#[derive(Debug)]
pub struct DialogState {
    pub index: usize,
    pub scroll: usize,
    pub input: super::TextInput,
    pub confirm: bool,
}

impl Default for DialogState {
    fn default() -> Self {
        Self {
            index: 0,
            scroll: 0,
            input: super::TextInput::new(220),
            confirm: false,
        }
    }
}

impl App {
    /// 推送 `npc_window`：弹窗（同一个窗口也推给 agent 连接，两边显示一致）。
    pub(super) fn on_npc_window(&mut self, data: serde_json::Value) {
        let w: NpcWindow = serde_json::from_value(data).unwrap_or_default();
        if let Some(world) = self.world.as_mut() {
            world.npc_window = Some(w);
            world.dialog = DialogState::default();
        }
    }

    /// 推送 `npc_window_closed`：窗口关了（自己关的、被传送、战斗开始都走这里）。
    pub(super) fn on_npc_window_closed(&mut self, _data: serde_json::Value) {
        if let Some(world) = self.world.as_mut() {
            world.npc_window = None;
        }
    }

    /// `talk 编号`：和本地图 NPC 对话（协议 5.33）。编号照「地图」页的本地图 NPC 表输入。
    pub(super) fn talk_command(&mut self, rest: &str) {
        let Some(no) = rest.trim().parse::<u32>().ok().filter(|n| *n > 0) else {
            self.log
                .error("用法：talk 编号（编号见「地图」页的本地图 NPC 表）");
            return;
        };
        self.open_npc(no);
    }

    pub(super) fn open_npc(&mut self, no: u32) {
        if self.session.npc_open_busy() || self.observer_blocks() {
            return;
        }
        self.send("npc_open", json!({"npc":no}), Pending::Talk);
    }

    /// `choose 编号`：回应窗口选项（协议 5.35）。本期对话都没有选项，服务端会回
    /// NPC_BAD_CHOICE；命令保留给商店等带选项的窗口。
    pub(super) fn choose_command(&mut self, rest: &str) {
        let Some(no) = rest.trim().parse::<u32>().ok().filter(|n| *n > 0) else {
            self.log
                .error("用法：choose 选项编号（看对话窗口里带编号的选项）");
            return;
        };
        if self
            .world
            .as_ref()
            .is_some_and(|w| w.npc_window.as_ref().is_some_and(|n| n.trade.is_some()))
        {
            if no != 1 && no != 2 {
                self.log.error("报价确认：choose 1 确认，choose 2 取消");
                return;
            }
            self.npc_send("npc_confirm", json!({"confirm":no==1}), Pending::NpcChoose);
        } else {
            self.npc_send("npc_choose", json!({"no": no}), Pending::NpcChoose);
        }
    }

    /// `reply 文本`：对窗口的文字回复（协议 5.37）：商店买东西报数量，任务 NPC 的暗号。
    /// 文字原样发送（暗号要逐字相同，不去掉空格），只去掉行尾的换行。
    pub(super) fn reply_command(&mut self, rest: &str) {
        let text = rest.trim_end_matches(['\r', '\n']);
        if text.trim().is_empty() {
            self.log
                .error("用法：reply 文本（看对话窗口里的提示，比如买东西报数量、说暗号）");
            return;
        }
        self.npc_send("npc_reply", json!({"text": text}), Pending::NpcReply);
    }

    /// 关掉当前窗口：本地清掉并发 `npc_close`（服务端同步清状态并通知其他连接）。
    pub(super) fn close_npc_window(&mut self) {
        if self.world.as_ref().is_some_and(|w| {
            w.npc_window
                .as_ref()
                .is_some_and(|n| !n.window_id.is_empty())
        }) {
            self.npc_send("npc_close", json!({}), Pending::NpcClose);
            return;
        }
        if let Some(world) = self.world.as_mut() {
            world.npc_window = None;
        }
        self.send("npc_close", json!({}), Pending::NpcClose);
    }

    /// 对话窗口打开时的按键。返回 false 表示这个键交给底部输入行（普通字符或别的命令）。
    pub(super) fn dialog_key(&mut self, key: crossterm::event::KeyEvent) -> bool {
        use crossterm::event::KeyCode;
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        let Some(w) = world.npc_window.as_ref() else {
            return false;
        };
        if !w.window_id.is_empty() {
            return self.menu_dialog_key(key);
        }
        // 确认层在弹窗之上（docs/interaction.md「按键分层」）：等 y 的时候数字是文字，不选选项。
        if world.confirm.is_some() {
            return false;
        }
        let empty = world.input.is_empty();
        match key.code {
            // Esc：输入行有字时只清空（交给输入行，返回 false）；空行时关窗（STAGE-103，docs/interaction.md「按键表」）。
            KeyCode::Esc if empty => {
                self.close_npc_window();
                true
            }
            // 数字键：有选项时选选项（`choose N`）；没有选项的窗口按任意数字关掉
            //（`docs/ui.md`「服务端驱动的界面」）。输入行有字时是普通字符；接受文字的窗口（暗号）数字也是文字。
            KeyCode::Char(c @ '1'..='9') if empty && !w.accepts_text => {
                if w.options.is_empty() {
                    self.close_npc_window();
                } else if let Some(no) = c.to_digit(10) {
                    self.choose_by_key(no);
                }
                true
            }
            // 回车在没有选项的窗口上也只是关掉（没有可提交的东西）。
            KeyCode::Enter if empty && w.options.is_empty() => {
                self.close_npc_window();
                true
            }
            _ => false,
        }
    }

    /// 数字键选窗口选项（STAGE-104，docs/interaction.md「快捷键与命令」）：消息栏回显等价的 `choose N`，
    /// 观察模式下拦下，不发 `npc_choose`。
    fn choose_by_key(&mut self, no: u32) {
        self.log.command(&format!("choose {no}"));
        if self.observer_blocks() {
            return;
        }
        self.npc_send("npc_choose", json!({"no": no}), Pending::NpcChoose);
    }

    fn npc_send(&mut self, ty: &str, mut data: serde_json::Value, pending: Pending) {
        if self.session.npc_busy() {
            return;
        }
        if ty != "npc_close" && self.observer_blocks() {
            return;
        }
        if let Some(id) = self
            .world
            .as_ref()
            .and_then(|w| w.npc_window.as_ref())
            .map(|n| &n.window_id)
            .filter(|id| !id.is_empty())
        {
            data["window_id"] = json!(id);
        }
        self.send(ty, data, pending);
    }

    fn menu_dialog_key(&mut self, key: crossterm::event::KeyEvent) -> bool {
        use crossterm::event::{KeyCode, KeyEventKind};
        if key.kind == KeyEventKind::Repeat {
            return true;
        }
        if self
            .world
            .as_ref()
            .is_some_and(|w| w.nav.is(&super::Layer::Command))
        {
            return false;
        }
        let Some(world) = self.world.as_mut() else {
            return false;
        };
        let Some(npc) = world.npc_window.as_ref() else {
            return false;
        };
        let text_input = npc.input.is_some();
        let trade = npc.trade.is_some();
        match key.code {
            KeyCode::Esc => {
                if trade {
                    self.npc_send("npc_confirm", json!({"confirm":false}), Pending::NpcChoose);
                } else if npc.stage == "buy_quantity" || npc.stage == "sell_quantity" || npc.stage == "buy_list" || npc.stage == "sell_list" {
                    self.npc_send("npc_back", json!({}), Pending::NpcChoose);
                } else {
                    self.close_npc_window();
                }
            }
            KeyCode::Char('/') if !text_input => {
                world.nav.push(super::Layer::Command);
            }
            KeyCode::Left | KeyCode::Right | KeyCode::Tab | KeyCode::BackTab if trade => {
                world.dialog.confirm = !world.dialog.confirm;
            }
            KeyCode::Up if !text_input => {
                world.dialog.index = world.dialog.index.saturating_sub(1);
            }
            KeyCode::Down if !text_input => {
                world.dialog.index =
                    (world.dialog.index + 1).min(npc.options.len().saturating_sub(1));
            }
            KeyCode::PageUp => {
                world.dialog.scroll = world.dialog.scroll.saturating_sub(5);
                world.dialog.index = world.dialog.index.saturating_sub(5);
            }
            KeyCode::PageDown => {
                world.dialog.scroll += 5;
                world.dialog.index =
                    (world.dialog.index + 5).min(npc.options.len().saturating_sub(1));
            }
            KeyCode::Home if !text_input => {
                world.dialog.index = 0;
                world.dialog.scroll = 0;
            }
            KeyCode::End if !text_input => {
                world.dialog.index = npc.options.len().saturating_sub(1);
            }
            KeyCode::Enter if trade => {
                let confirm = world.dialog.confirm;
                self.npc_send(
                    "npc_confirm",
                    json!({"confirm":confirm}),
                    Pending::NpcChoose,
                );
            }
            KeyCode::Char(c @ '1'..='9') if !text_input && !trade => {
                if npc.options.is_empty() { self.close_npc_window(); }
                else if let Some(no)=c.to_digit(10) { self.npc_send("npc_choose",json!({"no":no}),Pending::NpcChoose); }
            }
            KeyCode::Enter if text_input => {
                let text = world.dialog.input.text().to_string();
                if let Some(input) = &npc.input {
                    if input.kind == "integer"
                        && !text
                            .parse::<u32>()
                            .is_ok_and(|n| n >= input.min && n <= input.max)
                    {
                        self.log
                            .error(format!("数量必须为 {}–{} 的整数", input.min, input.max));
                        return true;
                    }
                    if text.is_empty()
                        || (input.max_chars > 0 && text.chars().count() > input.max_chars)
                    {
                        self.log.error("输入内容为空或过长");
                        return true;
                    }
                }
                self.npc_send("npc_reply", json!({"text":text}), Pending::NpcReply);
            }
            KeyCode::Enter => {
                if let Some(o) = npc.options.get(world.dialog.index) {
                    let no = o.no;
                    self.npc_send("npc_choose", json!({"no":no}), Pending::NpcChoose);
                } else {
                    self.close_npc_window();
                }
            }
            _ if text_input => {
                world.dialog.input.handle_key(&key);
            }
            _ => {}
        }
        true
    }
}
