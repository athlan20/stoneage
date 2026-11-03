//! STAGE-277：队伍聊天的命令入口（`docs/chat.md` 1.5，`docs/protocol.md` 5.68「队伍频道」）。
//! `party say 文本`（别名「队聊」）带文字直接发到队伍；不带文字打开聊天输入框并选好队伍频道。
//! 发送、推送回显和错误文字沿用 `chat.rs`，这里只做命令入口的本地检查。

use super::App;
use super::party_rules as rules;
use crate::proto::ChatChannel;

/// 取出 `party say` 后面的原文（保留文字里的空格）。不是 say 子命令返回 None。
pub fn say_text(rest: &str) -> Option<&str> {
    let body = rest.trim_start().strip_prefix("say")?;
    (body.is_empty() || body.starts_with(char::is_whitespace)).then(|| body.trim())
}

impl App {
    /// `party say 文本` / `队聊 文本`：不在真人队里说明原因、不发请求；带文字就发，不带就打开输入框。
    pub(super) fn party_say(&mut self, text: &str) {
        let Some(reason) = self.world.as_ref().and_then(rules::chat_reason) else {
            let text = text.trim();
            if text.is_empty() {
                self.open_chat(Some(ChatChannel::Party));
            } else {
                self.send_chat(ChatChannel::Party, text);
            }
            return;
        };
        self.log.error(reason);
    }
}

#[cfg(test)]
mod tests {
    use super::say_text;

    #[test]
    fn say_keeps_the_text_and_needs_a_word_boundary() {
        assert_eq!(say_text("say 大家好 来打怪"), Some("大家好 来打怪"));
        assert_eq!(say_text("  say  你好  "), Some("你好"));
        assert_eq!(say_text("say"), Some(""));
        assert_eq!(say_text("saying hi"), None);
        assert_eq!(say_text("clone"), None);
    }
}
