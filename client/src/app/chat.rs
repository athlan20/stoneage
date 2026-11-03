//! STAGE-278：公开聊天（`docs/chat.md`、`docs/protocol.md` 5.68）。命令 `say`/`world`、聊天输入框的状态与请求、
//! `chat_message` 推送写进消息栏、错误提示。按键见 `chat_keys.rs`，绘制见 `ui/chat_panel.rs`。
//! 发送者自己靠推送回显，这里不在发出时写消息。

use serde_json::{Value, json};

use super::App;
use super::input::TextInput;
use super::messages::Kind;
use super::nav::Layer;
use super::session::{Pending, format_wait};
use crate::proto::{ChatChannel, ChatMessage, ProtoError};

/// 聊天最长字符数（UTF-8 字符，中英文都按 1 个算）。
pub const CHAT_MAX: usize = 70;

/// 聊天输入框的状态。草稿在收起后保留，下次打开还在。
#[derive(Debug)]
pub struct ChatState {
    pub channel: ChatChannel,
    pub input: TextInput,
    /// 输入框下一行的提示（错误原因等）；下一次发送时清掉。
    pub hint: Option<String>,
}

impl Default for ChatState {
    fn default() -> Self {
        Self {
            channel: ChatChannel::Map,
            input: TextInput::new(CHAT_MAX),
            hint: None,
        }
    }
}

/// 把服务端的错误码翻成玩家能看懂的原因（时间、等级、冷却剩余都写清楚）。`now_ms` 用来算禁言还剩多久。
pub fn chat_error_text(channel: ChatChannel, code: &str, message: &str, data: &Value, now_ms: u64) -> String {
    match code {
        "CHAT_MUTED" => {
            let until = data.get("until_ms").and_then(Value::as_u64).unwrap_or(0);
            let left = until.saturating_sub(now_ms).div_ceil(1000);
            if left == 0 {
                "你被管理员禁言，稍后再试".into()
            } else {
                format!("你被管理员禁言，{}后解除", format_wait(left))
            }
        }
        "CHAT_LEVEL_LOW" => {
            let min = data.get("min_level").and_then(Value::as_u64).unwrap_or(5);
            format!("{}频道需要 {min} 级，你的等级还不够", channel.name())
        }
        "RATE_LIMITED" => {
            let wait = data.get("retry_after_seconds").and_then(Value::as_u64).unwrap_or(1);
            format!("{}频道说话太快，还要等 {} 才能再发", channel.name(), format_wait(wait))
        }
        "WRONG_STATE" => "你不在世界中，不能说话".into(),
        "NOT_IN_PARTY" => "只有真人队里能用队伍频道（分身队和没有队伍都不行）".into(),
        "BAD_REQUEST" if !message.is_empty() => message.to_string(),
        "BAD_REQUEST" => "需要 1–70 个字符，且不能含换行".into(),
        _ if !message.is_empty() => message.to_string(),
        _ => format!("发送失败（{code}）"),
    }
}

impl App {
    /// 有聊天请求在途（等响应）时为真，在途时 Enter 不重复发送。看会话里的待响应表，连接断开后自动清掉。
    pub fn chat_busy(&self) -> bool {
        self.session.chat_busy()
    }

    /// `say 文本` / `world 文本`：带文字就直接发；不带文字就打开聊天输入框并选好频道。
    pub(super) fn chat_command(&mut self, rest: &str, channel: ChatChannel) -> bool {
        let text = rest.trim();
        if text.is_empty() {
            self.open_chat(Some(channel));
            return true;
        }
        self.send_chat(channel, text)
    }

    /// 玩家正在聊天：聊天框开着，或命令栏里敲的是聊天命令。战斗日志区这时只显示聊天（STAGE-278 的延伸），
    /// 别让战斗文字把正在聊的内容冲走。
    pub fn chat_focus(&self) -> bool {
        let Some(w) = self.world.as_ref() else { return false };
        if w.nav.is(&Layer::Chat) {
            return true;
        }
        if !w.nav.is(&Layer::Command) {
            return false;
        }
        let line = w.input.text().trim_start();
        let word = line.split_whitespace().next().unwrap_or("");
        let rest = line[word.len()..].trim_start();
        matches!(word, "say" | "s" | "说" | "world" | "w" | "喊" | "party-say" | "队聊" | "ag")
            || (matches!(word, "party" | "队伍") && rest.starts_with("say"))
    }

    /// 打开聊天输入框（场景「聊天…」、`say`/`world` 无文字）。命令栏开着时换掉它，草稿保留。
    pub(super) fn open_chat(&mut self, channel: Option<ChatChannel>) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        if let Some(ch) = channel {
            w.chat.channel = ch;
        }
        if w.nav.is(&Layer::Command) {
            w.nav.close_command();
        }
        if !w.nav.is(&Layer::Chat) {
            w.nav.push(Layer::Chat);
        }
    }

    /// 发出一条公开聊天。本地只做明显不合规的检查（空、过长、控制字符），其余交给服务端。
    /// 发出去返回 true；文字不合规时写一条错误并返回 false。
    pub(super) fn send_chat(&mut self, channel: ChatChannel, text: &str) -> bool {
        if channel == ChatChannel::Party {
            if let Some(reason) = self.world.as_ref().and_then(super::party_rules::chat_reason) {
                self.log.error(reason);
                return false;
            }
        }
        let text = text.trim();
        if text.is_empty() {
            self.log.error("聊天内容不能为空");
            return false;
        }
        if text.chars().count() > CHAT_MAX {
            self.log.error(format!("聊天最多 {CHAT_MAX} 个字符"));
            return false;
        }
        if text.chars().any(char::is_control) {
            self.log.error("聊天不能含换行或制表符");
            return false;
        }
        let payload = json!({"channel": channel.wire(), "text": text});
        let pending = Pending::Chat {
            channel,
            text: text.to_string(),
        };
        if !self.send("chat", payload, pending) {
            return false;
        }
        if let Some(w) = self.world.as_mut() {
            w.chat.hint = None;
        }
        true
    }

    /// `chat_message` 推送：写进消息栏。发送者自己的消息名字写「你」。
    pub(super) fn on_chat_message(&mut self, data: Value) {
        let m: ChatMessage = serde_json::from_value(data).unwrap_or_default();
        let Some(channel) = ChatChannel::from_wire(&m.channel) else {
            return;
        };
        let me = self
            .world
            .as_ref()
            .map(|w| w.entry.character.name.clone())
            .unwrap_or_default();
        let who = if !me.is_empty() && m.from == me {
            "你".to_string()
        } else {
            format!("{}(Lv{})", m.from, m.level)
        };
        // 正文以空格开头：标签 `[本图]` 正好 6 列，和正文之间留一格。
        self.log.push(Kind::Chat(channel), format!(" {who}: {}", m.text));
    }

    /// `chat` 响应成功：只表示服务端收下了。文字还在输入框里（没被改过）就清掉。
    pub(super) fn on_chat_ok(&mut self, text: &str) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        w.chat.hint = None;
        if w.chat.input.text().trim() == text {
            w.chat.input.clear();
        }
    }

    /// `chat` 失败：输入框打开时原因写在输入框下一行、草稿保留；没打开（命令发的）就写进消息栏。
    pub(super) fn on_chat_failed(&mut self, channel: ChatChannel, err: &ProtoError) {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map_or(0, |d| d.as_millis() as u64);
        let text = chat_error_text(channel, &err.code, &err.message, &err.data, now);
        let Some(w) = self.world.as_mut() else {
            return;
        };
        if w.nav.is(&Layer::Chat) {
            w.chat.hint = Some(text);
        } else {
            self.log.error(text);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn mute_shows_remaining_time() {
        let data = json!({"until_ms": 1_000_000 + 125_000});
        assert_eq!(
            chat_error_text(ChatChannel::Map, "CHAT_MUTED", "", &data, 1_000_000),
            "你被管理员禁言，2 分 5 秒后解除"
        );
    }

    #[test]
    fn level_and_cooldown_say_the_real_reason() {
        let lv = chat_error_text(ChatChannel::World, "CHAT_LEVEL_LOW", "", &json!({"min_level": 5}), 0);
        assert!(lv.contains("5 级"), "{lv}");
        let rl = chat_error_text(
            ChatChannel::World,
            "RATE_LIMITED",
            "",
            &json!({"retry_after_seconds": 12}),
            0,
        );
        assert!(rl.contains("12 秒"), "{rl}");
        assert!(rl.starts_with("世界频道"), "{rl}");
    }
}
