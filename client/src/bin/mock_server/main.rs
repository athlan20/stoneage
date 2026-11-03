//! 模拟服务端：按协议用例回放，供客户端开发和验收。用法见 `docs/client.md`。
//!
//! 用例里 `player` 连接的 `c2s` 行是期望客户端发来的请求，`s2c` 行是模拟服务端要回的响应或推送。

mod check;
mod party;
mod trade;
mod social;
mod script;

use std::io::{BufRead, BufReader, ErrorKind, Write};
use std::net::{TcpListener, TcpStream};
use std::path::PathBuf;
use std::process::ExitCode;
use std::time::{Duration, Instant};

use clap::Parser;
use script::{Dir, Script, Vars, diff_shape, diff_strict, materialize};
use serde_json::{Value, json};
use stoneage_client::net::compress;
use stoneage_client::proto::redact;

#[derive(Debug, Parser)]
#[command(about = "石器时代模拟服务端：按 docs/protocol/cases/ 的用例回放")]
struct Cli {
    /// 用例名（不带 .jsonl）或文件路径
    #[arg(long)]
    case: Option<String>,
    /// 监听地址
    #[arg(long, default_value = "127.0.0.1:7065")]
    listen: String,
    /// 用例目录
    #[arg(long, default_value = "docs/protocol/cases")]
    cases_dir: PathBuf,
    /// 严格模式：请求里的每个值都要与用例一致（默认只检查字段结构和协议规则）
    #[arg(long)]
    strict: bool,
    /// 用例走完且客户端断开后退出，有不符时退出码为 1
    #[arg(long)]
    once: bool,
    /// 列出所有用例
    #[arg(long)]
    list: bool,
    /// 扮演的连接名（用例里的 conn 字段），默认 player；决斗用例挑战方是 bob、应战方是 alice
    #[arg(long, default_value = "player")]
    conn: String,
}

struct Player {
    script: Script,
    cursor: usize,
    strict: bool,
    failures: usize,
    last_id: u64,
    floor: u32,
    vars: Vars,
    /// hello 声明了 gzip（STAGE-246）：之后用例标了 z 的服务端消息按压缩行发出。
    compress: bool,
    /// 社交开关的当前值（STAGE-276，见 `social.rs`）。
    social: social::Social,
}

fn log(arrow: &str, text: impl AsRef<str>) {
    eprintln!("[mock] {arrow} {}", text.as_ref());
}

fn main() -> ExitCode {
    let cli = Cli::parse();
    if cli.list {
        return list_cases(&cli);
    }
    let Some(case) = cli.case.clone() else {
        eprintln!("请用 --case 指定用例，--list 查看所有用例");
        return ExitCode::from(2);
    };
    let path = if case.ends_with(".jsonl") || case.contains('/') { PathBuf::from(&case) } else { cli.cases_dir.join(format!("{case}.jsonl")) };
    let script = match script::load_as(&path, &cli.conn) {
        Ok(s) => s,
        Err(e) => {
            eprintln!("{e}");
            return ExitCode::from(2);
        }
    };
    let listener = match TcpListener::bind(&cli.listen) {
        Ok(l) => l,
        Err(e) => {
            eprintln!("无法监听 {}: {e}", cli.listen);
            return ExitCode::from(2);
        }
    };
    log("●", format!("用例 {case}：{}", script.desc));
    let bound = listener.local_addr().map_or_else(|_| cli.listen.clone(), |a| a.to_string());
    log("●", format!("监听 {bound}，{}模式", if cli.strict { "严格" } else { "宽松" }));
    let mut p = Player { script, cursor: 0, strict: cli.strict, failures: 0, last_id: 0, floor: 0, vars: Vars::new(), compress: false, social: social::Social::default() };
    for stream in listener.incoming().flatten() {
        log("●", "客户端已连接");
        serve(stream, &mut p);
        let done = p.cursor >= p.script.steps.len();
        log("●", if done { "客户端已断开，用例已走完" } else { "客户端已断开，用例还没走完" });
        if cli.once {
            break;
        }
    }
    if p.failures > 0 {
        log("✗", format!("共 {} 处不符", p.failures));
        ExitCode::from(1)
    } else if p.cursor < p.script.steps.len() {
        log("✗", format!("用例没有走完（停在第 {} / {} 步）", p.cursor + 1, p.script.steps.len()));
        ExitCode::from(1)
    } else {
        log("✓", "全部符合");
        ExitCode::SUCCESS
    }
}

fn list_cases(cli: &Cli) -> ExitCode {
    let Ok(rd) = std::fs::read_dir(&cli.cases_dir) else {
        eprintln!("找不到用例目录 {}", cli.cases_dir.display());
        return ExitCode::from(2);
    };
    let mut paths: Vec<_> = rd.flatten().map(|e| e.path()).filter(|p| p.extension().is_some_and(|e| e == "jsonl")).collect();
    paths.sort();
    for p in paths {
        let desc = script::load(&p).map_or_else(|_| "（服务端专用，模拟服务端不支持）".to_string(), |s| s.desc);
        println!("{:<28} {desc}", p.file_stem().unwrap_or_default().to_string_lossy());
    }
    ExitCode::SUCCESS
}

fn send(stream: &mut TcpStream, v: &Value) -> bool {
    log("→", redact(v).to_string());
    stream.write_all(format!("{v}\n").as_bytes()).is_ok()
}

fn serve(mut stream: TcpStream, p: &mut Player) {
    let _ = stream.set_read_timeout(Some(Duration::from_millis(100)));
    let Ok(read_half) = stream.try_clone() else { return };
    let mut reader = BufReader::new(read_half);
    let mut line = String::new();
    let mut due: Option<Instant> = None;
    loop {
        // 轮到服务端发言的步骤：到点就发。
        while let Some(step) = p.script.steps.get(p.cursor).filter(|s| s.dir == Dir::S2c) {
            let at = *due.get_or_insert_with(|| Instant::now() + Duration::from_millis(step.delay_ms));
            if Instant::now() < at {
                break;
            }
            due = None;
            if step.msg.is_null() {
                p.cursor += 1;
                log("●", "用例要求服务端在这里关闭连接");
                return;
            }
            let z = step.z;
            let mut msg = materialize(&step.msg, &mut p.vars);
            if msg.get("id").is_some() {
                msg["id"] = json!(p.last_id);
            }
            if msg["type"]=="enter_world" || msg["type"]=="status" {
                if let Some(floor)=msg["data"]["position"]["floor"].as_u64(){p.floor=floor as u32;}
            }
            if msg["type"]=="position_changed" {
                if let Some(floor)=msg["data"]["floor"].as_u64(){p.floor=floor as u32;}
            }
            if matches!(msg["type"].as_str(), Some("enter_world" | "status")) && msg["ok"]==true && msg["data"].get("social").is_none() {
                msg["data"]["social"] = p.social.json();
            }
            if msg["type"]=="npcs" && msg["ok"]==true && msg["data"].get("floor").is_none(){msg["data"]["floor"]=json!(p.floor);}
            let closing = msg["type"] == "disconnect" && msg.get("id").is_none();
            p.cursor += 1;
            if !send_step(&mut stream, &msg, z, p.compress) || closing {
                // 推送 disconnect 之后服务端关闭连接；用例里紧跟的 closed 行一并走完。
                while p.script.steps.get(p.cursor).is_some_and(|s| s.dir == Dir::S2c && s.msg.is_null()) {
                    p.cursor += 1;
                }
                return;
            }
        }
        match reader.read_line(&mut line) {
            Ok(0) => return,
            Ok(_) => {
                let text = std::mem::take(&mut line);
                if !text.trim().is_empty() && !handle(&mut stream, p, text.trim()) {
                    return;
                }
            }
            Err(e) if matches!(e.kind(), ErrorKind::WouldBlock | ErrorKind::TimedOut) => {}
            Err(_) => return,
        }
    }
}

/// 处理一条客户端请求；返回 false 表示连接已不可用。
/// 发一条服务端消息；步骤标了 z 且连接已协商压缩时，按压缩行发出（STAGE-246）。
fn send_step(stream: &mut TcpStream, v: &Value, z: bool, negotiated: bool) -> bool {
    if z && negotiated {
        log("→", format!("{} （压缩）", redact(v)));
        return stream.write_all(format!("{}\n", compress::gzip_line(&v.to_string())).as_bytes()).is_ok();
    }
    send(stream, v)
}

fn handle(stream: &mut TcpStream, p: &mut Player, text: &str) -> bool {
    let Ok(req) = serde_json::from_str::<Value>(text) else {
        log("✗", "收到无法解析的行");
        p.failures += 1;
        return send(stream, &json!({"id": 0, "type": "error", "ok": false, "error": {"code": "BAD_REQUEST", "message": "无法解析"}}));
    };
    log("←", redact(&req).to_string());
    let id = req.get("id").and_then(Value::as_u64).unwrap_or(0);
    let ty = req.get("type").and_then(Value::as_str).unwrap_or("").to_string();
    let data = req.get("data").cloned().unwrap_or(Value::Null);
    // hello 声明 gzip 时，之后标了 z 的服务端消息按压缩行发出（与服务端的协商一致）。
    if ty == "hello" && data.get("compress").and_then(Value::as_array).is_some_and(|a| a.iter().any(|c| c == "gzip")) {
        p.compress = true;
    }
    let expected = p.script.steps.get(p.cursor).filter(|s| s.dir == Dir::C2s);

    // 心跳不属于用例时自动应答。
    if ty == "ping" && expected.is_none_or(|s| s.ty() != "ping") {
        let now = std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).map_or(0, |d| d.as_millis() as u64);
        return send(stream, &json!({"id": id, "type": "ping", "ok": true, "data": {"server_time_ms": now}}));
    }
    // 客户端进入世界后会自动取一次地点表（STAGE-22）；用例没写这一步时，模拟服务端按 docs/protocol.md 5.15 的一期地点表自动应答。
    if ty == "places" && expected.is_none_or(|s| s.ty() != "places") {
        return send(stream, &json!({"id": id, "type": "places", "ok": true, "data": {"places": default_places()}}));
    }
    // 进入世界后还会取一次能传到的地图（STAGE-83，用来按地图名搜索和补全）；用例没写时按一小份地图表自动应答（docs/protocol.md 5.42）。
    if ty == "maps" && expected.is_none_or(|s| s.ty() != "maps") {
        return send(stream, &json!({"id": id, "type": "maps", "ok": true, "data": {
            "catalog_version": "mock-1",
            "maps": default_maps(),
            "nodes": default_catalog()
        }}));
    }
    // 客户端进入世界后也会取一次背包（STAGE-28）；用例没写时按新角色的空背包自动应答。
    if ty == "inventory" && expected.is_none_or(|s| s.ty() != "inventory") {
        return send(stream, &json!({"id": id, "type": "inventory", "ok": true, "data": {"gold": 1000000, "slots": []}}));
    }
    // 进入世界后还会取一次宠物（STAGE-31）；用例没写时按新角色的空宠物列表自动应答。
    if ty == "pets" && expected.is_none_or(|s| s.ty() != "pets") {
        return send(stream, &json!({"id": id, "type": "pets", "ok": true, "data": {"max_slots": 5, "pets": []}}));
    }
    // 进入世界后还会取一次任务进度（STAGE-86）；用例没写时按新角色的空任务列表自动应答（docs/protocol.md 5.44）。
    if ty == "quests" && expected.is_none_or(|s| s.ty() != "quests") {
        return send(stream, &json!({"id": id, "type": "quests", "ok": true, "data": {"quests": []}}));
    }
    // 进入世界后还会取一次本地图 NPC、换地图时重取（STAGE-68）；大多数用例不关心 NPC，
    // 用例没写时按空列表自动应答（docs/protocol.md 5.32）。
    if ty == "npcs" && expected.is_none_or(|s| s.ty() != "npcs") {
        return send(stream, &json!({"id": id, "type": "npcs", "ok": true, "data": {"floor":p.floor,"npcs": []}}));
    }
    if ty == "exits" && expected.is_none_or(|s| s.ty() != "exits") {
        return send(stream, &json!({"id":id,"type":"exits","ok":true,"data":{"floor":p.floor,"exits":[],"server_time_ms":0,"next_change_ms":null}}));
    }
    // 进世界、换地图会跟着取一次本地图遇敌区域（STAGE-152；STAGE-160 起与出口/NPC 一起预加载）。
    // 多数用例不关心遇敌，用例没写时按空列表自动应答（docs/protocol.md 5.50）。
    if ty == "encounter_areas" && expected.is_none_or(|s| s.ty() != "encounter_areas") {
        return send(stream, &json!({"id":id,"type":"encounter_areas","ok":true,"data":{"floor":p.floor,"areas":[],"server_time_ms":0}}));
    }
    // STAGE-273：进世界、换地图会取一次本地图附近玩家；多数用例不关心，用例没写时按空列表自动应答（docs/protocol.md 5.66）。
    if ty == "players" && expected.is_none_or(|s| s.ty() != "players") {
        return send(stream, &json!({"id":id,"type":"players","ok":true,"data":{"floor":p.floor,"truncated":false,"players":[]}}));
    }
    // STAGE-276：社交开关用例没写时按 docs/protocol.md 5.69 自动应答（值变了先推 social_changed）。
    if ty == "social_settings" && expected.is_none_or(|s| s.ty() != ty) {
        let fail = std::env::var_os("STONEAGE_MOCK_SOCIAL_FAIL").is_some();
        let (push, reply) = social::settings(id, &data, &mut p.social, fail);
        if let Some(push) = push {
            if !send(stream, &push) {
                return false;
            }
        }
        return send(stream, &reply);
    }
    // STAGE-295：交易请求用例没写时，按 docs/protocol.md 5.72 回错误码。
    if trade::is_trade_request(&ty) && expected.is_none_or(|s| s.ty() != ty) {
        return send(stream, &trade::auto_reply(id, &ty));
    }
    // STAGE-274：真人组队请求用例没写时，单玩家连接找不到对方，按 docs/protocol.md 5.67 自动回错误。
    if party::is_party_request(&ty) && expected.is_none_or(|s| s.ty() != ty) {
        return send(stream, &party::auto_reply(id, &ty));
    }
    // STAGE-277：队伍频道的聊天，用例没写时按没有队伍回 NOT_IN_PARTY（docs/protocol.md 5.68）。
    if ty == "chat" && party::is_party_chat(&data) && expected.is_none_or(|s| s.ty() != ty) {
        return send(stream, &json!({"id": id, "type": ty, "ok": false, "error": {"code": "NOT_IN_PARTY", "message": "你没有队伍"}}));
    }
    // talk / npc_choose / npc_close（STAGE-69）：窗口由推送带来，用例没写时不弹窗，
    // 兜底应答成功让客户端流程继续（docs/protocol.md 5.33–5.36）。
    if matches!(ty.as_str(), "talk" | "npc_choose" | "npc_close" | "npc_reply") && expected.is_none_or(|s| s.ty() != ty) {
        return send(stream, &json!({"id": id, "type": ty, "ok": true, "data": {}}));
    }
    let Some(step) = expected else {
        p.failures += 1;
        log("✗", format!("用例里没有这一步：收到 {ty}"));
        return send(stream, &err_reply(id, &ty, "UNEXPECTED", "模拟服务端：用例里没有这一步请求"));
    };
    if step.ty() != ty {
        p.failures += 1;
        log("✗", format!("期望请求 {}，实际收到 {ty}", step.ty()));
        return send(stream, &err_reply(id, &ty, "UNEXPECTED", &format!("模拟服务端：这一步应当是 {}", step.ty())));
    }
    let mut diffs = vec![];
    let exp_data = step.msg.get("data").cloned().unwrap_or(Value::Null);
    // STAGE-246：客户端的 hello 总会声明 compress，旧用例没写这个字段时不算不符（写了的照常比对）。
    let mut data = data;
    if ty == "hello" && exp_data.get("compress").is_none() {
        if let Some(o) = data.as_object_mut() {
            o.remove("compress");
        }
    }
    if p.strict {
        diff_strict(&exp_data, &data, "data", &mut diffs, &p.vars);
    } else {
        diff_shape(&exp_data, &data, "data", &mut diffs);
        diffs.extend(check::check_request(&ty, &data));
    }
    for d in &diffs {
        log("✗", format!("{ty}: {d}"));
    }
    p.failures += diffs.len();
    p.last_id = id;
    p.cursor += 1;
    true
}

fn default_places() -> Value {
    json!([
        {"id": 1, "name": "萨姆吉尔村", "floor": 1000, "x": 50, "y": 116},
        {"id": 2, "name": "玛丽娜丝渔村", "floor": 2000, "x": 72, "y": 33},
        {"id": 3, "name": "加加村", "floor": 3000, "x": 74, "y": 54},
        {"id": 4, "name": "卡鲁它那村", "floor": 4000, "x": 100, "y": 96},
        {"id": 5, "name": "萨姆吉尔近郊", "floor": 100, "x": 600, "y": 550},
        {"id": 6, "name": "玛丽娜丝近郊", "floor": 100, "x": 30, "y": 580},
        {"id": 7, "name": "加加村近郊", "floor": 200, "x": 760, "y": 445},
        {"id": 8, "name": "卡鲁它那近郊", "floor": 200, "x": 275, "y": 575}
    ])
}

/// 模拟服务端的地图表：四张地图，其中两张同名（“梦幻的洞窟”），用来覆盖同名歧义的提示。
fn default_maps() -> Value {
    json!([
        {"floor": 40, "name": "奇努伊关卡"},
        {"floor": 41, "name": "梦幻的洞窟"},
        {"floor": 42, "name": "梦幻的洞窟"},
        {"floor": 1006, "name": "萨姆吉尔的村长家"}
    ])
}

/// 模拟的快捷传送目录（STAGE-161）：萨伊那斯 → 萨姆吉尔村 → 村长家，外加一个纯分组，
/// 覆盖「有下级的地图首项是传送到本图」「纯分组不能传送」两种情况。
fn default_catalog() -> Value {
    json!([
        {"id": "group:sainasu", "parent_id": null, "name": "萨伊那斯", "floor": 100, "order": 0},
        {"id": "map:1000", "parent_id": "group:sainasu", "name": "萨姆吉尔村", "floor": 1000, "order": 0},
        {"id": "map:1006", "parent_id": "map:1000", "name": "萨姆吉尔的村长家", "floor": 1006, "order": 0},
        {"id": "group:sainasu.dungeons", "parent_id": "group:sainasu", "name": "洞窟与通路", "floor": null, "order": 1},
        {"id": "map:10001", "parent_id": "group:sainasu.dungeons", "name": "阿布的洞窟地下１楼", "floor": 10001, "order": 0},
        {"id": "group:seimu", "parent_id": null, "name": "沙姆岛", "floor": 400, "order": 1},
        {"id": "map:40", "parent_id": "group:seimu", "name": "奇努伊关卡", "floor": 40, "order": 0}
    ])
}

fn err_reply(id: u64, ty: &str, code: &str, message: &str) -> Value {
    json!({"id": id, "type": ty, "ok": false, "error": {"code": code, "message": message}})
}
