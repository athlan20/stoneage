use std::collections::VecDeque;

use ratatui::style::Color;

use crate::proto::ChatChannel;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Kind {
    Command,
    System,
    Error,
    /// agent 私聊（`docs/ui.md`：洋红色）。
    Agent,
    /// 战斗记录（`docs/ui.md`：黄色）。
    Battle,
    /// 队伍（STAGE-162，`docs/ui.md`：绿色）：组队、解散、战后恢复。
    Party,
    /// 交易（STAGE-295，`docs/trade.md` 5.2）：黄色，邀请、答复、结束时的提示。
    Trade,
    /// 交易成交（STAGE-295）：绿色。
    TradeOk,
    /// 装备耐久（STAGE-225）：黄色提示，如“X因过度损坏而消失。”（服务端 `crush[].red` 为假）。
    Durability,
    /// 装备耐久（STAGE-225）：红色提示，如“X是受损的。”“X是毁坏的。”（服务端 `red` 为真）。
    DurabilityRed,
    /// 精工（STAGE-299）：`true` 是成功（绿色），`false` 是部分成功或失败（黄色）。
    Inlay(bool),
    /// 公开聊天（STAGE-278）：`[本图]` 默认色、`[世界]` LightBlue，正文以空格开头。
    Chat(ChatChannel),
}

impl Kind {
    /// 定宽标签（宽度按终端列计），配色见 `docs/ui.md`。
    pub fn label(self) -> &'static str {
        match self {
            Self::Command => "命令  ",
            Self::System => "系统  ",
            Self::Error => "错误  ",
            Self::Agent => "agent ",
            Self::Battle => "战斗  ",
            Self::Party => "队伍  ",
            Self::Trade | Self::TradeOk => "交易  ",
            Self::Durability | Self::DurabilityRed => "耐久  ",
            Self::Inlay(_) => "精工  ",
            Self::Chat(ch) => ch.tag(),
        }
    }

    /// 装备耐久的类型：`red` 为真是红色的“耐久”，否则黄色（与服务端 `crush[].red` 一致，STAGE-248 起战斗日志也用）。
    pub fn durability(red: bool) -> Self {
        if red { Self::DurabilityRed } else { Self::Durability }
    }

    pub fn color(self) -> Color {
        match self {
            Self::Command => Color::Cyan,
            Self::System => Color::Blue,
            Self::Error => Color::Red,
            Self::Agent => Color::Magenta,
            Self::Battle => Color::Yellow,
            Self::Party => Color::Green,
            Self::Trade => Color::Yellow,
            Self::TradeOk => Color::Green,
            Self::Durability => Color::Yellow,
            Self::DurabilityRed => Color::Red,
            Self::Inlay(ok) => if ok { Color::Green } else { Color::Yellow },
            Self::Chat(ChatChannel::Map) => Color::Reset,
            Self::Chat(ChatChannel::World) => Color::LightBlue,
            Self::Chat(ChatChannel::Party) => Color::Green,
        }
    }
}

#[derive(Debug, Clone)]
pub struct Message {
    pub kind: Kind,
    pub text: String,
    /// 递增序号（从 1 开始），消息页用它做滚动锚点，新消息和丢弃最早消息时视图不跳。
    pub seq: u64,
}

const MAX_MESSAGES: usize = 1000;

/// 本次登录以来的消息，只在内存里保存。
#[derive(Debug, Default)]
pub struct MessageLog {
    items: VecDeque<Message>,
    next_seq: u64,
}

impl MessageLog {
    /// 追加一条消息（战斗日志先排队、再由 `flush_battle_log` 按顺序写进来，STAGE-248）。
    pub fn push(&mut self, kind: Kind, text: impl Into<String>) {
        self.next_seq += 1;
        self.items.push_back(Message { kind, text: text.into(), seq: self.next_seq });
        while self.items.len() > MAX_MESSAGES {
            self.items.pop_front();
        }
    }

    pub fn system(&mut self, text: impl Into<String>) {
        self.push(Kind::System, text);
    }

    pub fn error(&mut self, text: impl Into<String>) {
        self.push(Kind::Error, text);
    }

    pub fn agent(&mut self, text: impl Into<String>) {
        self.push(Kind::Agent, text);
    }

    pub fn command(&mut self, text: impl Into<String>) {
        self.push(Kind::Command, text);
    }

    pub fn chat(&mut self, channel: ChatChannel, text: impl Into<String>) {
        self.push(Kind::Chat(channel), text);
    }

    pub fn battle(&mut self, text: impl Into<String>) {
        self.push(Kind::Battle, text);
    }

    pub fn party(&mut self, text: impl Into<String>) {
        self.push(Kind::Party, text);
    }

    /// 装备耐久的提示（STAGE-225）：`red` 为真用红色，否则黄色，与服务端 `crush[].red` 一致。
    pub fn crush(&mut self, text: impl Into<String>, red: bool) {
        self.push(Kind::durability(red), text);
    }

    /// 全部现存消息，最早在前。
    pub fn all(&self) -> &VecDeque<Message> {
        &self.items
    }

    /// 最近 `n` 条，最早的在前（底部消息栏用）。
    pub fn recent(&self, n: usize) -> impl Iterator<Item = &Message> {
        self.items.iter().skip(self.items.len().saturating_sub(n))
    }

    pub fn len(&self) -> usize {
        self.items.len()
    }

    pub fn is_empty(&self) -> bool {
        self.items.is_empty()
    }

    /// 最早一条消息；空日志返回 `None`。
    pub fn first(&self) -> Option<&Message> {
        self.items.front()
    }

    /// 最新一条的序号；空日志为 0。
    pub fn latest_seq(&self) -> u64 {
        self.items.back().map_or(0, |m| m.seq)
    }

    /// 序号从 `seq` 起（含）的消息，最早的在前。
    pub fn from_seq(&self, seq: u64) -> impl Iterator<Item = &Message> {
        self.items.iter().skip_while(move |m| m.seq < seq)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn seq_increases_from_one() {
        let mut log = MessageLog::default();
        log.system("第一条");
        log.error("第二条");
        assert_eq!(log.all().iter().map(|m| m.seq).collect::<Vec<_>>(), vec![1, 2]);
        assert_eq!(log.latest_seq(), 2);
    }

    #[test]
    fn cap_drops_oldest() {
        let mut log = MessageLog::default();
        for i in 1..=1200 {
            log.system(format!("第{i}条"));
        }
        assert_eq!(log.len(), 1000);
        assert_eq!(log.first().unwrap().seq, 201);
        assert_eq!(log.latest_seq(), 1200);
        // 现存消息的序号连续，没有跳变。
        let seqs: Vec<u64> = log.all().iter().map(|m| m.seq).collect();
        assert!(seqs.windows(2).all(|w| w[1] == w[0] + 1));
    }

    #[test]
    fn recent_takes_last_n_in_order() {
        let mut log = MessageLog::default();
        for i in 1..=10 {
            log.system(format!("第{i}条"));
        }
        let texts: Vec<_> = log.recent(3).map(|m| m.text.clone()).collect();
        assert_eq!(texts, vec!["第8条", "第9条", "第10条"]);
    }
}
