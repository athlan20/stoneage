//! STAGE-170：`agent` 命令生成的接入说明。令牌原文只在生成时有，所以说明和令牌一起写进消息栏，
//! 玩家整段复制给自己的 agent 即可。这里只做文本拼装，不碰网络和状态，便于单测。

use super::agent_mcp;
use crate::net::Endpoint;

/// 手册、参考脚本和 WebSocket 入口都在服务端的同一个端口上（`docs/server.md`「agent 手册」）。
/// `ws_url` 是 agent 连接用的 `ws(s)://…/ws`，`http_base` 是手册与脚本的 `http(s)://主机[:端口]`。
struct Entry {
    ws_url: String,
    http_base: String,
    /// 当前线路是裸 TCP 地址，WebSocket 端口只能猜：说明里要提醒玩家核对。
    guessed: bool,
}

fn entry_of(addr: &str) -> Entry {
    match Endpoint::parse(addr) {
        Ok(Endpoint::Ws { url, .. }) => {
            let (scheme, rest) = url.split_once("://").unwrap_or(("ws", url.as_str()));
            let authority = rest.split('/').next().unwrap_or(rest);
            let http = if scheme == "wss" { "https" } else { "http" };
            Entry { ws_url: url.clone(), http_base: format!("{http}://{authority}"), guessed: false }
        }
        // 裸 TCP（本机开发）：服务端的 WebSocket 入口默认示例端口是 7067（`.env.example`）。
        _ => {
            let host = addr.trim().rsplit_once(':').map_or(addr.trim(), |(h, _)| h);
            let host = if host.is_empty() { "127.0.0.1" } else { host };
            Entry { ws_url: format!("ws://{host}:7067/ws"), http_base: format!("http://{host}:7067"), guessed: true }
        }
    }
}

/// 接入说明的各行，每行一条消息，复制时从「复制下面」到「结束」之间的内容给 agent。
pub fn guide_lines(addr: &str, token: &str) -> Vec<String> {
    let e = entry_of(addr);
    let mut lines = vec!["—— 复制下面整段发给你的 agent ——".to_string()];
    lines.push(format!(
        "你是我的石器时代 agent。请先读手册 {base}/agent.md ，再下载参考脚本 {base}/agent-connect.py ，\
         用它连接 {ws} 并以 agent 身份登录，代我游玩。",
        base = e.http_base,
        ws = e.ws_url
    ));
    lines.push(format!("令牌（只显示这一次，勿外传）：{token}"));
    lines.push("启动示例：mkfifo agent.in".to_string());
    lines.push(format!("export STONEAGE_AGENT_TOKEN={token}"));
    lines.push(format!("python3 agent-connect.py --url {} --input agent.in > agent.out 2>&1 &", e.ws_url));
    lines.push("—— 结束 ——".to_string());
    if e.guessed {
        lines.push(format!(
            "提示：当前线路是 TCP 地址，WebSocket 地址按 {} 推测（端口取自 .env.example 的 STONEAGE_WS_LISTEN）；\
             与服务端实际配置不同时请自行替换。",
            e.ws_url
        ));
    }
    // STAGE-304：Claude Code 的一行接入命令放在「结束」之后，不进剪贴板（剪贴板只给通用 agent 用）。
    lines.push("接入 Claude Code（MCP）：先装好 stoneage-mcp（发布页「让 AI 玩石器（MCP）」一节），再在终端执行这一行：".to_string());
    lines.push(agent_mcp::mcp_add_command(addr, token));
    lines
}

/// 要放进剪贴板的部分：「复制下面」和「结束」两条分隔行之间的内容。
pub fn copy_text(lines: &[String]) -> String {
    let start = lines.iter().position(|l| l.contains("复制下面")).map_or(0, |i| i + 1);
    let end = lines
        .iter()
        .position(|l| l.contains("—— 结束 ——"))
        .unwrap_or(lines.len());
    lines[start.min(end)..end].join("\n")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn wss_route_gives_https_manual_and_wss_url() {
        let lines = guide_lines("wss://game.example.com/ws", "sat_abc");
        let all = lines.join("\n");
        assert!(all.contains("https://game.example.com/agent.md"), "{all}");
        assert!(all.contains("https://game.example.com/agent-connect.py"), "{all}");
        assert!(all.contains("--url wss://game.example.com/ws"), "{all}");
        assert!(all.contains("令牌（只显示这一次，勿外传）：sat_abc"), "{all}");
        assert!(!all.contains("推测"), "wss 地址不用猜：{all}");
    }

    #[test]
    fn ws_route_keeps_port_and_uses_http() {
        let all = guide_lines("ws://127.0.0.1:7067", "sat_x").join("\n");
        assert!(all.contains("http://127.0.0.1:7067/agent.md"), "{all}");
        assert!(all.contains("ws://127.0.0.1:7067/ws"), "{all}");
    }

    #[test]
    fn tcp_route_guesses_ws_port_and_says_so() {
        let all = guide_lines("127.0.0.1:7065", "sat_x").join("\n");
        assert!(all.contains("ws://127.0.0.1:7067/ws"), "{all}");
        assert!(all.contains("http://127.0.0.1:7067/agent.md"), "{all}");
        assert!(all.contains("推测"), "{all}");
    }

    #[test]
    fn copy_text_excludes_markers_and_the_tcp_hint() {
        let lines = guide_lines("127.0.0.1:7065", "sat_x");
        let text = copy_text(&lines);
        assert!(text.contains("sat_x") && text.contains("agent.md"), "{text}");
        assert!(
            !text.contains("复制下面") && !text.contains("结束") && !text.contains("推测"),
            "{text}"
        );
    }

    #[test]
    fn mcp_command_is_after_the_copy_block_and_uses_the_connection_address() {
        let lines = guide_lines("wss://game.example.com/ws", "sat_m");
        assert!(!copy_text(&lines).contains("claude mcp add"), "剪贴板不带 MCP 命令");
        assert_eq!(
            lines.last().unwrap(),
            "claude mcp add stoneage -e STONEAGE_AGENT_TOKEN=sat_m -- stoneage-mcp --server wss://game.example.com/ws"
        );
    }

    #[test]
    fn markers_surround_the_copyable_block() {
        let lines = guide_lines("wss://h/ws", "sat_x");
        assert!(lines.first().unwrap().contains("复制下面"));
        assert!(lines.iter().any(|l| l.contains("结束")));
    }
}
