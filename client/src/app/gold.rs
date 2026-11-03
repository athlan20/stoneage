//! 「丢弃石币」（STAGE-328，`docs/protocol.md` 5.76）：`gold drop <数量> [yes]`。
//! 没有 yes 先问一句，输入 y 才发请求（`Confirm::GoldDrop`，见 `confirm.rs`）；带 yes 直接丢弃。
//! 丢弃就是销毁，不可恢复。石币的变化由 `status_changed` 推送告诉界面，这里只写一句话到消息栏。

use serde_json::json;

use super::App;
use super::commands;
use super::confirm::Confirm;
use super::session::Pending;

/// 用法提示，从命令表取。
fn usage() -> String {
    commands::usage_of("gold")
}

/// `gold drop` 的参数：数量（正整数）和是不是带了 yes。
#[derive(Debug, PartialEq, Eq)]
pub(super) struct GoldDropArgs {
    pub amount: u64,
    pub confirmed: bool,
}

/// 解析 `drop <数量> [yes]`；yes 可以放在最后。数量只在本地判断是不是正整数，够不够持有由服务端判断。
pub(super) fn parse_gold_drop(args: &[&str]) -> Result<GoldDropArgs, String> {
    let (confirmed, args) = match args.split_last() {
        Some((&"yes", rest)) => (true, rest),
        _ => (false, args),
    };
    match args {
        [amount] => match amount.parse::<u64>() {
            Ok(n) if n >= 1 => Ok(GoldDropArgs { amount: n, confirmed }),
            _ => Err("数量要是正整数".to_string()),
        },
        _ => Err(usage()),
    }
}

impl App {
    /// `gold` 命令：目前只有 `gold drop <数量> [yes]`。
    pub(super) fn gold_command(&mut self, rest: &str) {
        let args: Vec<&str> = rest.split_whitespace().collect();
        match args.split_first() {
            Some((&"drop", tail)) => match parse_gold_drop(tail) {
                Ok(GoldDropArgs {
                    amount,
                    confirmed: true,
                }) => self.send_gold_drop(amount),
                Ok(GoldDropArgs {
                    amount,
                    confirmed: false,
                }) => {
                    let text = format!("丢弃后不能恢复：{amount} 石币");
                    self.ask_confirm(Confirm::GoldDrop { amount }, &text);
                }
                Err(msg) => self.log.error(msg),
            },
            _ => self.log.error(usage()),
        }
    }

    /// 发出丢弃石币的请求（直接带 yes，或者确认之后）。
    pub(super) fn send_gold_drop(&mut self, amount: u64) {
        self.send(
            "gold_drop",
            json!({"amount": amount, "confirm": "yes"}),
            Pending::GoldDrop { amount },
        );
    }

    /// 丢弃成功：回复里的 `gold` 是丢弃后的持有量。
    pub(super) fn on_gold_drop(&mut self, amount: u64, data: &serde_json::Value) {
        match data.get("gold").and_then(|v| v.as_u64()) {
            Some(gold) => self.log.system(format!("已丢弃 {amount} 石币，持有 {gold}")),
            None => self.log.system(format!("已丢弃 {amount} 石币")),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_amount_with_optional_yes() {
        assert_eq!(
            parse_gold_drop(&["1000"]),
            Ok(GoldDropArgs {
                amount: 1000,
                confirmed: false
            })
        );
        assert_eq!(
            parse_gold_drop(&["1000", "yes"]),
            Ok(GoldDropArgs {
                amount: 1000,
                confirmed: true
            })
        );
    }

    #[test]
    fn rejects_bad_amount_and_extra_args() {
        for bad in ["0", "-5", "1.5", "abc"] {
            assert!(parse_gold_drop(&[bad, "yes"]).unwrap_err().contains("正整数"), "{bad}");
        }
        assert_eq!(parse_gold_drop(&[]), Err(usage()));
        assert_eq!(parse_gold_drop(&["1", "2", "yes"]), Err(usage()));
    }
}
