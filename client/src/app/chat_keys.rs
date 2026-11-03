//! STAGE-278：聊天输入框的按键（`docs/interaction.md`「聊天输入框」）。输入框打开时它独占按键：
//! 字符进框，Tab 切频道，Enter 发送并保持打开，Esc 收起并保留草稿。

use crossterm::event::{KeyCode, KeyEvent};

use super::App;

impl App {
    pub(super) fn chat_layer_key(&mut self, key: KeyEvent) {
        match key.code {
            // 收起只退这一层，草稿留在 `ChatState` 里，下次打开还在。
            KeyCode::Esc => {
                if let Some(w) = self.world.as_mut() {
                    w.nav.pop();
                }
            }
            KeyCode::Tab => {
                if let Some(w) = self.world.as_mut() {
                    let party_ok = super::party_rules::party_chat_ok(w);
                    w.chat.channel = w.chat.channel.next(party_ok);
                }
            }
            KeyCode::Enter => {
                let Some((channel, text)) = self
                    .world
                    .as_ref()
                    .map(|w| (w.chat.channel, w.chat.input.text().trim().to_string()))
                else {
                    return;
                };
                // 在途时不重复发送；空文字不发，框里留着。
                if !self.chat_busy() && !text.is_empty() {
                    self.send_chat(channel, &text);
                }
            }
            _ => {
                if let Some(w) = self.world.as_mut() {
                    w.chat.input.handle_key(&key);
                }
            }
        }
    }
}
