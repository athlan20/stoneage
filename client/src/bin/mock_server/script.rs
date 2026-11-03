//! 协议用例（`docs/protocol/cases/*.jsonl`）的读取与比对，格式见 `docs/protocol.md` 第 6 节。

use std::collections::HashMap;
use std::fs;
use std::path::Path;

use serde_json::{Value, json};

/// 用例里捕获的变量：`s2c` 行的 `"*>名字=样例"` 记下发出去的样例，之后 `c2s` 行的 `"$名字"` 要等于它，
/// `s2c` 行的 `"$名字"` 发出去的也是它（见 `docs/protocol.md` 第 6 节）。
pub type Vars = HashMap<String, Value>;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Dir {
    C2s,
    S2c,
}

#[derive(Debug, Clone)]
pub struct Step {
    pub dir: Dir,
    pub msg: Value,
    pub delay_ms: u64,
    /// s2c：这一行要以压缩行发出（用例里 `"z": "gzip"`，STAGE-246）；连接没协商压缩时仍原样发送。
    pub z: bool,
}

impl Step {
    pub fn ty(&self) -> &str {
        self.msg.get("type").and_then(Value::as_str).unwrap_or("")
    }
}

pub struct Script {
    pub desc: String,
    pub steps: Vec<Step>,
}

/// 读取用例。模拟服务端只扮演 `player` 连接，其余连接（`setup`、`second` 等）的行直接跳过。
pub fn load(path: &Path) -> Result<Script, String> {
    load_as(path, "player")
}

/// 读取用例，扮演 `conn` 指定的连接（决斗用例：`--conn alice` 看应战方，`--conn bob` 看挑战方）。
pub fn load_as(path: &Path, conn: &str) -> Result<Script, String> {
    let text = fs::read_to_string(path).map_err(|e| format!("读取 {} 失败: {e}", path.display()))?;
    let mut script = Script { desc: String::new(), steps: vec![] };
    for (n, line) in text.lines().enumerate() {
        if line.trim().is_empty() {
            continue;
        }
        let v: Value = serde_json::from_str(line).map_err(|e| format!("{}:{}: {e}", path.display(), n + 1))?;
        if let Some(meta) = v.get("meta") {
            script.desc = meta.get("desc").and_then(Value::as_str).unwrap_or_default().to_string();
            continue;
        }
        if v.get("conn").and_then(Value::as_str).unwrap_or("player") != conn {
            continue;
        }
        // close 行（客户端直接断开，如 battle_offline 用例）只在服务端测试里重放，模拟服务端跳过。
        if v.get("dir").and_then(Value::as_str) == Some("close") {
            continue;
        }
        let dir = match v.get("dir").and_then(Value::as_str) {
            Some("c2s") => Dir::C2s,
            Some("s2c") => Dir::S2c,
            _ => return Err(format!("{}:{}: dir 只能是 c2s 或 s2c", path.display(), n + 1)),
        };
        if v.get("raw").is_some() || v.get("raw_len").is_some() {
            return Err(format!("{}:{}: 含 raw/raw_len 行，只用于服务端测试，模拟服务端不支持", path.display(), n + 1));
        }
        // `closed` 行没有 msg：表示服务端在这里关闭连接，用 null 表示。
        let msg = if v.get("closed").and_then(Value::as_bool) == Some(true) {
            Value::Null
        } else {
            v.get("msg").cloned().ok_or_else(|| format!("{}:{}: 缺少 msg", path.display(), n + 1))?
        };
        let delay_ms = v.get("delay_ms").and_then(Value::as_u64).unwrap_or(0);
        let z = v.get("z").is_some();
        script.steps.push(Step { dir, msg, delay_ms, z });
    }
    Ok(script)
}

/// 期望值里的通配写法：`"*"` 表示任意值，`"*=样例"` 表示任意值、模拟服务端发送“样例”，
/// `"*>名字"` 和 `"*>名字=样例"` 同样是通配，同时把发出去的值记成变量“名字”。
fn wildcard(v: &Value) -> Option<Option<&str>> {
    let s = v.as_str()?;
    if s == "*" {
        Some(None)
    } else if let Some(rest) = s.strip_prefix("*?=") {
        Some(Some(rest))
    } else if let Some(rest) = s.strip_prefix("*>") {
        Some(rest.split_once('=').map(|(_, sample)| sample))
    } else {
        s.strip_prefix("*=").map(Some)
    }
}

fn capture_name(v: &Value) -> Option<&str> {
    let rest = v.as_str()?.strip_prefix("*>")?;
    Some(rest.split('=').next().unwrap_or(rest)).filter(|n| !n.is_empty())
}

/// `"$名字"`：引用之前捕获的变量。
fn var_name(v: &Value) -> Option<&str> {
    v.as_str()?.strip_prefix('$').filter(|n| !n.is_empty())
}

fn sample_value(sample: Option<&str>) -> Value {
    match sample {
        None => Value::String("（模拟）".into()),
        Some(s) => serde_json::from_str(s).unwrap_or_else(|_| Value::String(s.into())),
    }
}

/// 把期望值里的通配换成能发出去的具体值；遇到捕获就记下变量，遇到 `$名字` 就换成记下的值。
/// key 是所在字段的名字：时间戳字段（`deadline_ms`、`server_time_ms`，docs/protocol.md 10.4）
/// 用模拟服务端的当前时间，样例值只是占位——用例是按真实输出记录的，绝对时间戳会过期。
pub fn materialize(v: &Value, vars: &mut Vars) -> Value {
    materialize_at(v, "", vars)
}

fn now_ms() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_millis() as u64)
        .unwrap_or(0)
}

fn materialize_at(v: &Value, key: &str, vars: &mut Vars) -> Value {
    match key {
        "deadline_ms" if wildcard(v).is_some() => return json!(now_ms() + 30_000),
        "server_time_ms" if wildcard(v).is_some() => return json!(now_ms()),
        _ => {}
    }
    if let Some(sample) = wildcard(v) {
        let val = sample_value(sample);
        if let Some(name) = capture_name(v) {
            vars.insert(name.to_string(), val.clone());
        }
        return val;
    }
    if let Some(name) = var_name(v) {
        return vars.get(name).cloned().unwrap_or_else(|| Value::String(format!("（没有捕获 {name}）")));
    }
    match v {
        Value::Object(m) => Value::Object(m.iter().map(|(k, x)| (k.clone(), materialize_at(x, k, vars))).collect()),
        Value::Array(a) => Value::Array(a.iter().map(|x| materialize_at(x, "", vars)).collect()),
        other => other.clone(),
    }
}

/// 严格比对：除通配外，值必须相等；返回差异说明。
pub fn diff_strict(expected: &Value, actual: &Value, path: &str, out: &mut Vec<String>, vars: &Vars) {
    if wildcard(expected).is_some() {
        if actual.is_null() && !expected.as_str().is_some_and(|s|s.starts_with("*?=")) {
            out.push(format!("{path}: 缺少字段"));
        }
        return;
    }
    if let Some(name) = var_name(expected) {
        match vars.get(name) {
            Some(want) if want == actual => {}
            Some(want) => out.push(format!("{path}: 期望 ${name}（{want}），实际 {actual}")),
            None => out.push(format!("{path}: 引用了还没有捕获的变量 ${name}")),
        }
        return;
    }
    match (expected, actual) {
        (Value::Object(e), Value::Object(a)) => {
            for (k, ev) in e {
                match a.get(k) {
                    Some(av) => diff_strict(ev, av, &format!("{path}.{k}"), out, vars),
                    None => out.push(format!("{path}.{k}: 缺少字段")),
                }
            }
            for k in a.keys().filter(|k| !e.contains_key(*k)) {
                out.push(format!("{path}.{k}: 用例里没有这个字段"));
            }
        }
        (Value::Array(e), Value::Array(a)) if e.len() == a.len() => {
            for (i, (ev, av)) in e.iter().zip(a).enumerate() {
                diff_strict(ev, av, &format!("{path}[{i}]"), out, vars);
            }
        }
        (e, a) if e == a => {}
        (e, a) => out.push(format!("{path}: 期望 {e}，实际 {a}")),
    }
}

/// 宽松比对：只比字段结构和值的类型，不比具体值。用于手工操作客户端时，玩家填什么都行。
pub fn diff_shape(expected: &Value, actual: &Value, path: &str, out: &mut Vec<String>) {
    match (expected, actual) {
        (Value::Object(e), Value::Object(a)) => {
            for (k, ev) in e {
                match a.get(k) {
                    Some(av) => diff_shape(ev, av, &format!("{path}.{k}"), out),
                    None => out.push(format!("{path}.{k}: 缺少字段")),
                }
            }
            for k in a.keys().filter(|k| !e.contains_key(*k)) {
                out.push(format!("{path}.{k}: 用例里没有这个字段"));
            }
        }
        (e, a) if wildcard(e).is_some() || var_name(e).is_some() => {
            if a.is_null() && !e.as_str().is_some_and(|s|s.starts_with("*?=")) {
                out.push(format!("{path}: 缺少字段"));
            }
        }
        (e, a) if std::mem::discriminant(e) == std::mem::discriminant(a) => {}
        (e, a) => out.push(format!("{path}: 类型不对，期望 {e}，实际 {a}")),
    }
}

#[cfg(test)]
mod tests {
    use super::*;


    #[test]
    fn nullable_wildcard_requires_presence_but_accepts_null() {
        let exp=json!({"reason":"*?=当前关闭"});let vars=Vars::new();let mut d=vec![];
        diff_strict(&exp,&json!({"reason":null}),"data",&mut d,&vars);assert!(d.is_empty());
        diff_shape(&exp,&json!({"reason":null}),"data",&mut d);assert!(d.is_empty());
        diff_strict(&exp,&json!({}),"data",&mut d,&vars);assert!(!d.is_empty());
        let mut vars=Vars::new();assert_eq!(materialize(&exp,&mut vars),json!({"reason":"当前关闭"}));
    }
    #[test]
    fn wildcard_sample_is_typed() {
        let mut v = Vars::new();
        assert_eq!(materialize(&json!("*=897"), &mut v), json!(897));
        assert_eq!(materialize(&json!("*=true"), &mut v), json!(true));
        assert_eq!(materialize(&json!("*=账号已存在"), &mut v), json!("账号已存在"));
        assert_eq!(materialize(&json!({"a": ["*=1"]}), &mut v), json!({"a": [1]}));
    }

    #[test]
    fn captures_and_references() {
        let mut v = Vars::new();
        // s2c 里捕获：发样例并记下；之后 $名字 发出去的、c2s 要等于的都是它。
        assert_eq!(materialize(&json!({"token": "*>tok=sat_abc", "n": "*>n"}), &mut v), json!({"token": "sat_abc", "n": "（模拟）"}));
        assert_eq!(materialize(&json!({"token_id": "$tok"}), &mut v), json!({"token_id": "sat_abc"}));
        let mut d = vec![];
        diff_strict(&json!({"t": "$tok"}), &json!({"t": "sat_abc"}), "data", &mut d, &v);
        assert!(d.is_empty(), "{d:?}");
        diff_strict(&json!({"t": "$tok"}), &json!({"t": "别的"}), "data", &mut d, &v);
        diff_strict(&json!({"t": "$nope"}), &json!({"t": "x"}), "data", &mut d, &v);
        assert_eq!(d.len(), 2, "{d:?}");
        // 宽松模式：$名字 和通配一样，只看有没有这个字段。
        let mut d = vec![];
        diff_shape(&json!({"t": "$tok"}), &json!({"t": "随便"}), "data", &mut d);
        assert!(d.is_empty(), "{d:?}");
    }

    #[test]
    fn strict_and_shape() {
        let exp = json!({"name": "勇者", "n": "*", "x": 1});
        let mut d = vec![];
        let vars = Vars::new();
        diff_strict(&exp, &json!({"name": "勇者", "n": 7, "x": 1}), "data", &mut d, &vars);
        assert!(d.is_empty(), "{d:?}");
        diff_strict(&exp, &json!({"name": "别人", "x": 1}), "data", &mut d, &vars);
        assert_eq!(d.len(), 2, "{d:?}");
        let mut d = vec![];
        diff_shape(&exp, &json!({"name": "别人", "n": 7, "x": 9}), "data", &mut d);
        assert!(d.is_empty(), "{d:?}");
        diff_shape(&exp, &json!({"name": 5, "n": 7, "x": 9, "extra": 1}), "data", &mut d);
        assert_eq!(d.len(), 2, "{d:?}");
    }
}
