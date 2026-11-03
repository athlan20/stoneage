//! STAGE-304：玩家接入 MCP 用的一行命令 `claude mcp add stoneage -e STONEAGE_AGENT_TOKEN=<令牌> -- stoneage-mcp --server <地址>`。
//! 只做文本拼装，不碰网络和状态，便于单测。地址取客户端当前连接用的 `session.addr`（`ws(s)://…/ws` 或 `host:port`），
//! 与 `stoneage-mcp --server` 的约定一致（`docs/mcp.md`「怎么配置」）。

/// 拼出可直接粘贴到终端的接入命令。令牌和地址若含空格或 shell 特殊字符，整段加单引号。
pub fn mcp_add_command(addr: &str, token: &str) -> String {
    format!(
        "claude mcp add stoneage -e {} -- stoneage-mcp --server {}",
        shell_arg(&format!("STONEAGE_AGENT_TOKEN={token}")),
        shell_arg(addr)
    )
}

/// 只含这些字符的参数原样输出，其余用单引号包起来（内部的单引号写成 `'\''`）。空串也要加引号。
fn shell_arg(s: &str) -> String {
    let plain = !s.is_empty()
        && s.chars().all(|c| {
            c.is_ascii_alphanumeric() || matches!(c, '-' | '_' | '.' | '/' | ':' | '@' | '%' | '+' | '=' | ',')
        });
    if plain {
        s.to_string()
    } else {
        format!("'{}'", s.replace('\'', "'\\''"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn websocket_address_is_pasted_as_is() {
        let cmd = mcp_add_command("wss://game.example.com/ws", "sat_abc123");
        assert_eq!(
            cmd,
            "claude mcp add stoneage -e STONEAGE_AGENT_TOKEN=sat_abc123 -- stoneage-mcp --server wss://game.example.com/ws"
        );
    }

    #[test]
    fn tcp_address_is_pasted_as_is() {
        let cmd = mcp_add_command("127.0.0.1:7065", "sat_x");
        assert_eq!(
            cmd,
            "claude mcp add stoneage -e STONEAGE_AGENT_TOKEN=sat_x -- stoneage-mcp --server 127.0.0.1:7065"
        );
    }

    #[test]
    fn address_with_spaces_is_quoted() {
        let cmd = mcp_add_command("my host:7065", "sat_x");
        assert!(cmd.ends_with("--server 'my host:7065'"), "{cmd}");
        assert!(cmd.contains("-e STONEAGE_AGENT_TOKEN=sat_x "), "{cmd}");
    }

    #[test]
    fn single_quote_in_address_is_escaped() {
        assert_eq!(shell_arg("a'b c"), "'a'\\''b c'");
    }

    #[test]
    fn empty_address_is_quoted_so_the_flag_keeps_its_value() {
        assert_eq!(shell_arg(""), "''");
    }

    #[test]
    fn token_with_shell_characters_is_quoted_as_one_word() {
        let cmd = mcp_add_command("127.0.0.1:7065", "sat_a&b");
        assert!(cmd.contains("-e 'STONEAGE_AGENT_TOKEN=sat_a&b'"), "{cmd}");
    }
}
