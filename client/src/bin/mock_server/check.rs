//! 宽松模式下对请求做协议规则检查（`docs/protocol.md`），这样即使玩家填的值和用例不同，
//! 也能发现客户端发出了不合规的请求。

use serde_json::Value;
use stoneage_client::proto::{
    Elements, PROTOCOL_VERSION, STAT_POINTS, check_account, check_character_name, check_password,
};

fn u8_in(v: &Value, key: &str, max: u64, out: &mut Vec<String>, path: &str) -> u8 {
    match v.get(key).and_then(Value::as_u64) {
        Some(n) if n <= max => n as u8,
        _ => {
            out.push(format!("{path}.{key}: 需要 0–{max} 的整数"));
            0
        }
    }
}

pub fn check_request(ty: &str, data: &Value) -> Vec<String> {
    let mut out = vec![];
    let text = |k: &str| data.get(k).and_then(Value::as_str).unwrap_or_default();
    match ty {
        "hello" => {
            if data.get("protocol").and_then(Value::as_u64) != Some(u64::from(PROTOCOL_VERSION)) {
                out.push("data.protocol: 必须是 1".into());
            }
            if data.get("role").and_then(Value::as_str).is_some_and(|r| r != "player") {
                out.push("data.role: 玩家客户端必须是 player".into());
            }
        }
        "register" | "login" => {
            if let Err(e) = check_account(text("account")) {
                out.push(format!("data.account: {e}"));
            }
            if text("account") != text("account").to_lowercase() {
                out.push("data.account: 客户端应先转成小写".into());
            }
            if let Err(e) = check_password(text("password")) {
                out.push(format!("data.password: {e}"));
            }
        }
        "use_exit" => {
            if data.get("source_floor").and_then(Value::as_i64).is_none(){out.push("source_floor 必须为整数".into());}
            if text("exit_id").is_empty(){out.push("exit_id 必须为非空文字".into());}
        }
        "create_character" => check_create(data, &mut out),
        "delete_character" => {
            u8_in(data, "slot", 1, &mut out, "data");
            if text("name").is_empty() {
                out.push("data.name: 删除角色必须带上确认名".into());
            }
        }
        // STAGE-278：公开聊天，频道省略时当作 map；文字 1–70 个字符且不含控制字符（docs/protocol.md 5.68）。
        "chat" => {
            if !matches!(text("channel"), "" | "map" | "world" | "party") {
                out.push("data.channel: 只能是 map、world 或 party".into());
            }
            let t = text("text").trim();
            if !(1..=70).contains(&t.chars().count()) || t.chars().any(char::is_control) {
                out.push("data.text: 需要 1–70 个字符，且不能含控制字符".into());
            }
        }
        "enter_world" => {
            u8_in(data, "slot", 1, &mut out, "data");
        }
        _ => {}
    }
    out
}

fn check_create(data: &Value, out: &mut Vec<String>) {
    u8_in(data, "slot", 1, out, "data");
    if let Err(e) = check_character_name(data.get("name").and_then(Value::as_str).unwrap_or_default()) {
        out.push(format!("data.name: {e}"));
    }
    let ap = data.get("appearance").cloned().unwrap_or(Value::Null);
    u8_in(&ap, "body", 11, out, "data.appearance");
    u8_in(&ap, "color", 3, out, "data.appearance");
    u8_in(&ap, "eye", 4, out, "data.appearance");
    u8_in(&ap, "mouth", 4, out, "data.appearance");

    let stats = data.get("stats").cloned().unwrap_or(Value::Null);
    let total: u32 = ["vital", "strength", "toughness", "dexterity"]
        .iter()
        .map(|k| u32::from(u8_in(&stats, k, u64::from(STAT_POINTS), out, "data.stats")))
        .sum();
    if total > u32::from(STAT_POINTS) {
        out.push(format!("data.stats: 总和 {total} 超过 {STAT_POINTS}"));
    }

    let el = data.get("elements").cloned().unwrap_or(Value::Null);
    let [earth, water, fire, wind] = ["earth", "water", "fire", "wind"].map(|k| u8_in(&el, k, 10, out, "data.elements"));
    if !(Elements { earth, water, fire, wind }).is_valid() {
        out.push(format!(
            "data.elements: 地{earth} 水{water} 火{fire} 风{wind} 不合规（总和必须是 10，最多 2 项非零，地火、水风互斥）"
        ));
    }
    u8_in(data, "hometown", 3, out, "data");
}
