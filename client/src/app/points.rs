//! 加点（STAGE-24，`docs/ui.md` 角色页 B、`docs/protocol.md` 5.21）：`addpoint 体力|腕力|耐久|速度 [点数]`。
//! 能力名在本地换成协议里的能力键，错误的参数在本地就拦下；数值变化由 `status_changed` 推送告诉界面。

use serde_json::{Value, json};

use super::commands;
use super::App;
use super::session::Pending;
use crate::proto::StatusChanged;

/// 用法提示，从命令表取（STAGE-26）。
fn usage() -> String {
    commands::usage_of("addpoint")
}

/// 命令里的能力名 → 界面上的名字和协议里的能力键（`耐久力` 也认，和角色页的写法一致）。
fn stat_key(name: &str) -> Option<(&'static str, &'static str)> {
    match name {
        "体力" => Some(("体力", "vital")),
        "腕力" => Some(("腕力", "strength")),
        "耐久" | "耐久力" => Some(("耐久", "toughness")),
        "速度" => Some(("速度", "dexterity")),
        _ => None,
    }
}

/// 解析 `addpoint` 的参数：能力名、可选的点数（省略时为 1）。返回界面上的名字、协议里的能力键和点数。
fn parse_args(rest: &str) -> Result<(&'static str, &'static str, u32), String> {
    let mut parts = rest.split_whitespace();
    let Some(name) = parts.next() else {
        return Err(usage());
    };
    let points = match parts.next() {
        None => 1,
        Some(text) => match text.parse::<u32>() {
            Ok(n) if n >= 1 => n,
            _ => return Err("点数要是正整数".to_string()),
        },
    };
    if parts.next().is_some() {
        return Err(usage());
    }
    let (label, key) = stat_key(name).ok_or_else(|| format!("没有“{name}”这项能力，可选：体力、腕力、耐久、速度"))?;
    Ok((label, key, points))
}

impl App {
    /// `addpoint` 命令（操作类；观察模式的拦截由命令表驱动，见 `world.rs`，STAGE-26）。
    pub(super) fn addpoint_command(&mut self, rest: &str) {
        match parse_args(rest) {
            Ok((label, key, points)) => {
                let pending = Pending::AddPoint { stat: label.to_string(), points };
                self.send("addpoint", json!({"stat": key, "points": points}), pending);
            }
            Err(msg) => self.log.error(msg),
        }
    }

    /// `addpoint` 成功：数值已经由 `status_changed` 推送更新（推送先于响应到达），这里只写一句话到消息栏。
    pub(super) fn on_add_point(&mut self, stat: &str, points: u32) {
        let left = self.world.as_ref().map_or(0, |w| w.entry.status.skill_points);
        self.log.system(format!("已加点：{stat} +{points}，未分配点数还剩 {left}"));
    }

    /// `status_changed` 推送（`docs/protocol.md` 10.10）：整个 `status` 换成推送的新值。
    /// 未分配点数变多了（管理命令发的点数）就提示一句，怎么加点写在消息里。
    pub(super) fn on_status_changed(&mut self, data: Value) {
        let c: StatusChanged = serde_json::from_value(data).unwrap_or_default();
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let gained = c.status.skill_points.saturating_sub(w.entry.status.skill_points);
        w.entry.status = c.status;
        if gained > 0 {
            self.log.system(format!("获得 {gained} 点未分配点数，用 addpoint 能力 [点数] 加到能力上，例如 addpoint 体力 2"));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_stat_names_and_points() {
        assert_eq!(parse_args("体力 2"), Ok(("体力", "vital", 2)));
        assert_eq!(parse_args("速度"), Ok(("速度", "dexterity", 1)), "省略点数时加 1 点");
        assert_eq!(parse_args("耐久力 3"), Ok(("耐久", "toughness", 3)));
        assert_eq!(parse_args("  腕力   5 "), Ok(("腕力", "strength", 5)));
    }

    #[test]
    fn rejects_bad_arguments() {
        assert_eq!(parse_args(""), Err(usage()));
        assert!(parse_args("力量 1").unwrap_err().contains("没有“力量”"));
        assert!(parse_args("体力 0").unwrap_err().contains("正整数"));
        assert!(parse_args("体力 -1").unwrap_err().contains("正整数"));
        assert!(parse_args("体力 1.5").unwrap_err().contains("正整数"));
        assert_eq!(parse_args("体力 1 2"), Err(usage()));
    }
}
