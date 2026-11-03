//! STAGE-278：公开聊天（`docs/protocol.md` 5.68）。请求 `chat {channel, text}`；推送 `chat_message` 同时是发送者自己的回显。

use serde::Deserialize;

/// 频道：`map` 本图（默认）、`world` 世界，`party` 队伍（STAGE-277，只发给本队真人成员）。
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub enum ChatChannel {
    #[default]
    Map,
    World,
    Party,
}

impl ChatChannel {
    /// 协议里的频道名。
    pub fn wire(self) -> &'static str {
        match self {
            Self::Map => "map",
            Self::World => "world",
            Self::Party => "party",
        }
    }

    /// 协议里认识的频道名；不认识的返回 None。
    pub fn from_wire(s: &str) -> Option<Self> {
        match s {
            "map" => Some(Self::Map),
            "world" => Some(Self::World),
            "party" => Some(Self::Party),
            _ => None,
        }
    }

    /// 消息记录的标签（固定 6 列，同 `LABEL_WIDTH`）。`[队]` 占 4 列，后面补 2 格对齐。
    pub fn tag(self) -> &'static str {
        match self {
            Self::Map => "[本图]",
            Self::World => "[世界]",
            Self::Party => "[队]  ",
        }
    }

    /// Tab 切到下一个频道：本图 → 世界 → 队伍 → 本图；`party_ok` 为假（不在真人队里）时跳过队伍。
    pub fn next(self, party_ok: bool) -> Self {
        match self {
            Self::Map => Self::World,
            Self::World if party_ok => Self::Party,
            Self::World | Self::Party => Self::Map,
        }
    }

    /// 频道的中文名（提示文字用）。
    pub fn name(self) -> &'static str {
        match self {
            Self::Map => "本图",
            Self::World => "世界",
            Self::Party => "队伍",
        }
    }
}

/// `chat_message` 推送。`from` 是角色名，`level` 是发出时的等级。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct ChatMessage {
    #[serde(default)]
    pub channel: String,
    #[serde(default)]
    pub from: String,
    #[serde(default)]
    pub level: u32,
    #[serde(default)]
    pub text: String,
    #[serde(default)]
    pub time_ms: u64,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn chat_message_parses_and_channels_round_trip() {
        let v = serde_json::json!({"channel": "world", "from": "阿牛", "level": 30, "text": "你好", "time_ms": 1760000000000u64});
        let m: ChatMessage = serde_json::from_value(v).unwrap();
        assert_eq!(ChatChannel::from_wire(&m.channel), Some(ChatChannel::World));
        assert_eq!(m.from, "阿牛");
        assert_eq!(m.level, 30);
        assert_eq!(ChatChannel::from_wire("party"), Some(ChatChannel::Party));
        assert_eq!(ChatChannel::Map.next(false), ChatChannel::World);
        assert_eq!(ChatChannel::World.next(false), ChatChannel::Map, "没有真人队时跳过队伍");
        assert_eq!(ChatChannel::World.next(true), ChatChannel::Party);
        assert_eq!(ChatChannel::Party.next(true), ChatChannel::Map);
        assert_eq!(ChatChannel::Map.tag().chars().count(), 4);
    }
}
