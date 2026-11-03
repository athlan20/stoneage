//! 抓宠丢弃记录（STAGE-264 第 5 条）：抓到但资质被 `keep` 筛掉、放生了的宠物，按角色记下来，只读。
//!
//! 文件是方案目录里的 `capture-log.json`（与方案文件同一个服务器目录）：
//! `{"version": 1, "characters": {"角色名": [记录, ...]}}`，记录按时间从旧到新排，每个角色最多 50 条，
//! 超出丢最旧的。记录不进方案、不推给服务端，重启客户端后仍在。
//! 文件读不出来（损坏）当空记录；写的时候遇到坏文件先改名成 `capture-log.json.corrupt`，再写新的。

use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};

use super::capture_keep::{Stat, tier_sign};
use crate::app::auto_battle::store::Store;
use crate::proto::CaptureDiscard;

/// 每个角色最多保留的记录条数。
pub const MAX_RECORDS: usize = 50;
const FILE_NAME: &str = "capture-log.json";
const VERSION: u32 = 1;

/// 一条丢弃记录。`at` 是 UTC 时间（`2026-10-21 04:12:33`）。
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CaptureRecord {
    pub at: String,
    pub name: String,
    pub level: u32,
    /// 四项资质，顺序同协议：vital、strength、toughness、dexterity。
    #[serde(default)]
    pub quality: Vec<Quality>,
    pub reason: String,
}

/// 一项资质：`stat` 用协议键名（如 `vital`），显示时换成中文。
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct Quality {
    pub stat: String,
    pub points: i64,
    pub tier: i64,
}

#[derive(Debug, Default, Serialize, Deserialize)]
struct LogFile {
    #[serde(default)]
    version: u32,
    #[serde(default)]
    characters: BTreeMap<String, Vec<CaptureRecord>>,
}

/// 记录文件的位置：方案目录下的 `capture-log.json`。
pub fn path(store: &Store) -> PathBuf {
    store.base.join(&store.server).join(FILE_NAME)
}

/// 把放生的宠物变成一条记录；`at` 由调用方给（便于测试）。
pub fn from_discard(at: String, d: &CaptureDiscard) -> CaptureRecord {
    CaptureRecord {
        at,
        name: d.name.clone(),
        level: d.level,
        quality: d
            .quality
            .iter()
            .map(|q| Quality {
                stat: q.stat.clone(),
                points: q.points,
                tier: q.tier,
            })
            .collect(),
        reason: d.reason.clone(),
    }
}

/// 战斗记录里的一行：`抓到 乌力 Lv1，资质不合格已放生：体力 −1 < 0`。
pub fn battle_line(d: &CaptureDiscard) -> String {
    format!("抓到 {} Lv{}，资质不合格已放生：{}", d.name, d.level, d.reason)
}

/// 资质的一段文字：`体力 22(−1) 腕力 18(−2) 耐久力 20(0) 速度 24(−1)`。
pub fn quality_text(quality: &[Quality]) -> String {
    quality
        .iter()
        .map(|q| format!("{} {}({})", stat_label(&q.stat), q.points, tier_sign(q.tier)))
        .collect::<Vec<_>>()
        .join(" ")
}

/// 协议键名换成界面名字；认不出来就原样显示。
pub fn stat_label(key: &str) -> &str {
    Stat::ALL.iter().find(|s| s.key() == key).map_or(key, |s| s.label())
}

/// 某角色的记录，按时间从旧到新。文件不存在、读不出来都当空。
pub fn load(path: &Path, character: &str) -> Vec<CaptureRecord> {
    read(path)
        .ok()
        .and_then(|mut f| f.characters.remove(character))
        .unwrap_or_default()
}

/// 追加一条记录；超过 50 条丢最旧的。
pub fn append(path: &Path, character: &str, rec: CaptureRecord) -> Result<(), String> {
    update(path, character, |list| list.push(rec))
}

/// 清空某角色的记录。
pub fn clear(path: &Path, character: &str) -> Result<(), String> {
    update(path, character, |list| list.clear())
}

fn read(path: &Path) -> Result<LogFile, String> {
    match fs::read_to_string(path) {
        Ok(text) => serde_json::from_str(&text).map_err(|e| e.to_string()),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(LogFile::default()),
        Err(e) => Err(e.to_string()),
    }
}

fn update(path: &Path, character: &str, edit: impl FnOnce(&mut Vec<CaptureRecord>)) -> Result<(), String> {
    let mut file = match read(path) {
        Ok(f) => f,
        Err(_) => {
            // 坏文件先留一份，免得丢了玩家的记录；再从空记录写起。
            let corrupt = path.with_extension("json.corrupt");
            fs::rename(path, &corrupt).map_err(|e| format!("记录文件读不懂，改名也失败：{e}"))?;
            LogFile::default()
        }
    };
    file.version = VERSION;
    let list = file.characters.entry(character.to_string()).or_default();
    edit(list);
    if list.len() > MAX_RECORDS {
        let extra = list.len() - MAX_RECORDS;
        list.drain(..extra);
    }
    write(path, &file)
}

fn write(path: &Path, file: &LogFile) -> Result<(), String> {
    if let Some(dir) = path.parent() {
        fs::create_dir_all(dir).map_err(|e| format!("建不了目录 {}：{e}", dir.display()))?;
    }
    let text = serde_json::to_string_pretty(file).map_err(|e| e.to_string())?;
    let tmp = path.with_extension("json.tmp");
    fs::write(&tmp, text).map_err(|e| format!("写记录失败：{e}"))?;
    fs::rename(&tmp, path).map_err(|e| format!("写记录失败：{e}"))
}

/// 现在的 UTC 时间文字。
pub fn now_text() -> String {
    let secs = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |d| i64::try_from(d.as_secs()).unwrap_or(0));
    fmt_utc(secs)
}

/// Unix 秒换成 `YYYY-MM-DD HH:MM:SS`（UTC）。
pub fn fmt_utc(secs: i64) -> String {
    let (days, rem) = (secs.div_euclid(86_400), secs.rem_euclid(86_400));
    let (y, m, d) = civil_from_days(days);
    format!(
        "{y:04}-{m:02}-{d:02} {:02}:{:02}:{:02}",
        rem / 3600,
        rem % 3600 / 60,
        rem % 60
    )
}

/// 从 1970-01-01 起的天数换成公历年月日（Howard Hinnant 的算法）。
fn civil_from_days(days: i64) -> (i64, u32, u32) {
    let z = days + 719_468;
    let era = z.div_euclid(146_097);
    let doe = z.rem_euclid(146_097);
    let yoe = (doe - doe / 1460 + doe / 36_524 - doe / 146_096) / 365;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let y = yoe + era * 400 + i64::from(m <= 2);
    (y, m as u32, d as u32)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::proto::CaptureQuality;

    fn tmp(tag: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("stage264-log-{tag}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        dir.join("capture-log.json")
    }

    fn rec(n: usize) -> CaptureRecord {
        CaptureRecord {
            at: fmt_utc(n as i64),
            name: format!("宠{n}"),
            level: 1,
            quality: vec![],
            reason: "体力 −1 < 0".into(),
        }
    }

    fn discard() -> CaptureDiscard {
        let q = |stat: &str, points, tier| CaptureQuality {
            stat: stat.into(),
            points,
            tier,
        };
        CaptureDiscard {
            name: "乌力".into(),
            level: 1,
            quality: vec![
                q("vital", 21, -1),
                q("strength", 10, -4),
                q("toughness", 15, -2),
                q("dexterity", 27, 0),
            ],
            reason: "体力 −1 < 0".into(),
        }
    }

    #[test]
    fn utc_text_matches_known_instants() {
        assert_eq!(fmt_utc(0), "1970-01-01 00:00:00");
        assert_eq!(fmt_utc(1_782_000_000), "2026-06-21 00:00:00");
        assert_eq!(fmt_utc(951_782_400), "2000-02-29 00:00:00", "闰年");
    }

    #[test]
    fn discard_becomes_record_and_battle_line() {
        let r = from_discard("2026-10-21 04:12:33".into(), &discard());
        assert_eq!(r.name, "乌力");
        assert_eq!(r.quality.len(), 4);
        assert_eq!(
            quality_text(&r.quality),
            "体力 21(−1) 腕力 10(−4) 耐久力 15(−2) 速度 27(0)"
        );
        assert_eq!(battle_line(&discard()), "抓到 乌力 Lv1，资质不合格已放生：体力 −1 < 0");
    }

    #[test]
    fn append_then_load_keeps_order_and_characters_apart() {
        let p = tmp("order");
        append(&p, "勇者", rec(1)).unwrap();
        append(&p, "勇者", rec(2)).unwrap();
        append(&p, "法师", rec(9)).unwrap();
        let hero = load(&p, "勇者");
        assert_eq!(hero.iter().map(|r| r.name.as_str()).collect::<Vec<_>>(), ["宠1", "宠2"]);
        assert_eq!(load(&p, "法师").len(), 1);
        assert!(load(&p, "没有这个人").is_empty());
    }

    #[test]
    fn keeps_only_the_newest_fifty_per_character() {
        let p = tmp("cap");
        for n in 0..(MAX_RECORDS + 3) {
            append(&p, "勇者", rec(n)).unwrap();
        }
        let list = load(&p, "勇者");
        assert_eq!(list.len(), MAX_RECORDS);
        assert_eq!(list[0].name, "宠3", "最旧的 3 条被丢掉");
        assert_eq!(list.last().unwrap().name, format!("宠{}", MAX_RECORDS + 2));
    }

    #[test]
    fn clear_only_touches_that_character() {
        let p = tmp("clear");
        append(&p, "勇者", rec(1)).unwrap();
        append(&p, "法师", rec(2)).unwrap();
        clear(&p, "勇者").unwrap();
        assert!(load(&p, "勇者").is_empty());
        assert_eq!(load(&p, "法师").len(), 1);
    }

    #[test]
    fn broken_file_reads_as_empty_and_is_set_aside_on_write() {
        let p = tmp("broken");
        fs::create_dir_all(p.parent().unwrap()).unwrap();
        fs::write(&p, "{ 这不是 json").unwrap();
        assert!(load(&p, "勇者").is_empty());
        append(&p, "勇者", rec(1)).unwrap();
        assert_eq!(load(&p, "勇者").len(), 1);
        assert_eq!(
            fs::read_to_string(p.with_extension("json.corrupt")).unwrap(),
            "{ 这不是 json"
        );
    }

    #[test]
    fn file_format_is_grouped_by_character() {
        let p = tmp("format");
        append(&p, "勇者", rec(1)).unwrap();
        let v: serde_json::Value = serde_json::from_str(&fs::read_to_string(&p).unwrap()).unwrap();
        assert_eq!(v["version"], 1);
        assert_eq!(v["characters"]["勇者"][0]["name"], "宠1");
    }
}
