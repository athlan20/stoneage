//! 本地方案文件（STAGE-173 M15）：按服务器与角色隔离，一个角色一个 JSON。
//!
//! 位置：Windows 是 `%APPDATA%\stoneage\auto-battle\<服务器>\<角色>.json`，其他平台是
//! `$XDG_CONFIG_HOME/stoneage/auto-battle/...`（没有就用 `~/.config`）。不写进仓库的 `runtime/`，
//! 因为那是随工作目录走的运行数据，玩家换个目录启动就丢了设置。
//!
//! 读失败（文件损坏、版本更高）**不覆盖原文件**：先用默认方案顶上，下次保存前把原文件
//! 备份成 `.bak-<时间戳>`。保存走“先写临时文件再改名”，避免写到一半断电留下半个文件。

use std::fs;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};

use super::{AutoProfile, VERSION};

/// 一个角色的全部方案。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Profiles {
    pub version: u32,
    /// 当前使用的方案下标；越界时按 0 处理。
    #[serde(default)]
    pub active: usize,
    pub profiles: Vec<AutoProfile>,
}

impl Default for Profiles {
    fn default() -> Self {
        Self { version: VERSION, active: 0, profiles: vec![AutoProfile::default()] }
    }
}

impl Profiles {
    pub fn active(&self) -> &AutoProfile {
        self.profiles.get(self.active).or_else(|| self.profiles.first()).unwrap_or(&FALLBACK)
    }

    pub fn active_mut(&mut self) -> &mut AutoProfile {
        if self.profiles.is_empty() {
            self.profiles.push(AutoProfile::default());
        }
        let i = self.active.min(self.profiles.len() - 1);
        &mut self.profiles[i]
    }

    /// 换成另一个方案（循环）。
    pub fn step(&mut self, dir: i64) {
        let n = self.profiles.len().max(1) as i64;
        self.active = ((self.active as i64 + dir).rem_euclid(n)) as usize;
    }
}

/// 一个角色都没有方案时的兜底，只用于 `Profiles::active()` 和 `App::auto_profile()` 的返回值。
pub static FALLBACK: AutoProfile = AutoProfile {
    name: String::new(),
    target: super::TargetMode::Combo,
    player: super::PlayerAction { act: super::ActKind::Attack, magic_slot: 0, magic_name: String::new() },
    pet: super::PetAction { act: super::PetActKind::Attack, skill_slot: 1, skill_name: String::new() },
    first: super::FirstTurn {
        enabled: false,
        player: super::PlayerAction { act: super::ActKind::Attack, magic_slot: 0, magic_name: String::new() },
        pet: super::PetAction { act: super::PetActKind::Attack, skill_slot: 1, skill_name: String::new() },
    },
    recover: super::Recover {
        player_heal: true,
        player_heal_pct: 30,
        pet_heal: true,
        pet_heal_pct: 30,
        player_mp: false,
        player_mp_pct: 20,
        item_first: false,
        mp_items: Vec::new(),
        heal_magic_slot: 0,
        heal_magic_name: String::new(),
        revive_on: true,
        revive_source: super::ReviveSource::Auto,
        revive_defend: true,
        post_heal_item: true,
        post_heal_pct: 90,
        post_drop_item: false,
    },
    flee: super::Flee { enabled: false, hp_pct: 15, names: Vec::new() },
    stop: super::StopGuard { no_supply: true, player_level: 0, pet_level: 0, party_min: 0 },
    capture: super::Capture { enabled: false, hp_pct: 20, level_min: 1, level_max: 0, names: Vec::new(), keep: super::capture_keep::CaptureKeep::UNLIMITED, mode: super::CaptureMode::Fight },
    swap: super::Swap { enabled: false, hp_pct: 20, order: Vec::new() },
    stop_on_lose: true,
    playback: super::PlaybackPref { speed: 1, auto_continue: true, delay_ms: 500, manual_hold_ms: 3000 },
};

/// 一个角色的方案文件。
#[derive(Debug, Clone)]
pub struct Store {
    pub base: PathBuf,
    pub server: String,
    pub character: String,
    pub profiles: Profiles,
    /// 读取时的问题（损坏 / 版本更高）；保存前据此备份原文件。
    pub load_error: Option<String>,
}

impl Store {
    /// 打开（或准备创建）某个服务器上某个角色的方案。`character` 为空时用 `_` 占位。
    pub fn open(server: &str, character: &str) -> Self {
        Self::open_in(&base_dir(), server, character)
    }

    /// 指定根目录的版本，测试用。
    pub fn open_in(base: &Path, server: &str, character: &str) -> Self {
        let (server, character) = (safe_name(server), safe_name(character));
        let path = path_for(base, &server, &character);
        let mut load_error = None;
        let profiles = match fs::read_to_string(&path) {
            Ok(text) => match serde_json::from_str::<Profiles>(&text) {
                Ok(p) if p.version > VERSION => {
                    load_error = Some(format!("方案文件版本 {} 比客户端新，已按默认方案打开", p.version));
                    Profiles::default()
                }
                Ok(p) => p,
                Err(e) => {
                    load_error = Some(format!("方案文件读不懂（{e}），已按默认方案打开"));
                    Profiles::default()
                }
            },
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Profiles::default(),
            Err(e) => {
                load_error = Some(format!("读不到方案文件（{e}），已按默认方案打开"));
                Profiles::default()
            }
        };
        Self { base: base.to_path_buf(), server, character, profiles, load_error }
    }

    pub fn path(&self) -> PathBuf {
        path_for(&self.base, &self.server, &self.character)
    }

    pub fn active(&self) -> &AutoProfile {
        self.profiles.active()
    }

    /// 写回磁盘。原文件读的时候就有问题就先备份，绝不静默覆盖玩家原有的方案。
    pub fn save(&mut self) -> Result<(), String> {
        let path = self.path();
        if let Some(dir) = path.parent() {
            fs::create_dir_all(dir).map_err(|e| format!("建不了目录 {}：{e}", dir.display()))?;
        }
        if let Some(err) = self.load_error.take() {
            let stamp = SystemTime::now().duration_since(UNIX_EPOCH).map(|d| d.as_secs()).unwrap_or(0);
            let backup = path.with_extension(format!("bak-{stamp}"));
            if path.exists() {
                fs::copy(&path, &backup).map_err(|e| format!("备份原方案失败：{e}"))?;
            }
            let _ = err;
        }
        self.profiles.version = VERSION;
        let text = serde_json::to_string_pretty(&self.profiles).map_err(|e| format!("方案转不成 JSON：{e}"))?;
        let tmp = path.with_extension("json.tmp");
        fs::write(&tmp, text).map_err(|e| format!("写方案失败：{e}"))?;
        fs::rename(&tmp, &path).map_err(|e| {
            let _ = fs::remove_file(&tmp);
            format!("保存方案失败：{e}")
        })
    }
}

/// 方案根目录。
pub fn base_dir() -> PathBuf {
    if let Some(dir) = std::env::var_os("STONEAGE_CONFIG_DIR") {
        return PathBuf::from(dir).join("auto-battle");
    }
    if cfg!(windows) {
        if let Some(appdata) = std::env::var_os("APPDATA") {
            return PathBuf::from(appdata).join("stoneage").join("auto-battle");
        }
    } else if let Some(xdg) = std::env::var_os("XDG_CONFIG_HOME") {
        return PathBuf::from(xdg).join("stoneage").join("auto-battle");
    } else if let Some(home) = std::env::var_os("HOME") {
        return PathBuf::from(home).join(".config").join("stoneage").join("auto-battle");
    }
    PathBuf::from("runtime").join("auto-battle")
}

/// 服务器与角色名当目录/文件名：只留安全字符，其余换成 `_`。
pub fn safe_name(text: &str) -> String {
    let mut out = String::new();
    for c in text.chars() {
        if c.is_alphanumeric() || matches!(c, '-' | '_' | '.') {
            out.push(c);
        } else if matches!(c, ':' | '/' | '\\') {
            out.push('_');
        } else {
            out.push(c);
        }
    }
    let out = out.trim_matches('.').to_string();
    if out.is_empty() { "_".into() } else { out }
}

fn path_for(base: &Path, server: &str, character: &str) -> PathBuf {
    base.join(server).join(format!("{character}.json"))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn tmp(tag: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("stage173-{tag}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    #[test]
    fn round_trips_a_profile() {
        let dir = tmp("roundtrip");
        let mut s = Store::open_in(&dir, "127.0.0.1:7065", "勇者");
        s.profiles.active_mut().recover.player_heal_pct = 45;
        s.profiles.active_mut().name = "练级".into();
        s.save().unwrap();
        let back = Store::open_in(&dir, "127.0.0.1:7065", "勇者");
        assert_eq!(back.active().recover.player_heal_pct, 45);
        assert_eq!(back.active().name, "练级");
        assert!(back.load_error.is_none());
    }

    #[test]
    fn separates_servers_and_characters() {
        let dir = tmp("separate");
        let mut a = Store::open_in(&dir, "a.example:7065", "甲");
        a.profiles.active_mut().recover.player_heal_pct = 11;
        a.save().unwrap();
        let b = Store::open_in(&dir, "b.example:7065", "甲");
        assert_eq!(b.active().recover.player_heal_pct, 30, "换服务器不串方案");
        let c = Store::open_in(&dir, "a.example:7065", "乙");
        assert_eq!(c.active().recover.player_heal_pct, 30, "换角色不串方案");
    }

    #[test]
    fn broken_file_is_backed_up_not_lost() {
        let dir = tmp("broken");
        let path = path_for(&dir, "127.0.0.1_7065", "勇者");
        fs::create_dir_all(path.parent().unwrap()).unwrap();
        fs::write(&path, "{ 这不是 json").unwrap();
        let mut s = Store::open_in(&dir, "127.0.0.1:7065", "勇者");
        assert!(s.load_error.is_some(), "读坏了要说出来");
        s.save().unwrap();
        let backups: Vec<_> = fs::read_dir(path.parent().unwrap())
            .unwrap()
            .filter_map(|e| e.ok())
            .filter(|e| e.file_name().to_string_lossy().contains("bak-"))
            .collect();
        assert_eq!(backups.len(), 1, "原文件要先备份再覆盖");
        assert_eq!(fs::read(backups[0].path()).unwrap(), b"{ \xe8\xbf\x99\xe4\xb8\x8d\xe6\x98\xaf json");
    }

    #[test]
    fn future_version_is_not_treated_as_current() {
        let dir = tmp("future");
        let path = path_for(&dir, "_", "勇者");
        fs::create_dir_all(path.parent().unwrap()).unwrap();
        fs::write(&path, r#"{"version":99,"active":0,"profiles":[]}"#).unwrap();
        let s = Store::open_in(&dir, "", "勇者");
        assert!(s.load_error.as_deref().is_some_and(|e| e.contains("比客户端新")));
        assert_eq!(s.active().name, "默认方案");
    }

    #[test]
    fn active_falls_back_when_index_is_out_of_range() {
        let mut p = Profiles::default();
        p.active = 7;
        assert_eq!(p.active().name, "默认方案");
    }

    /// STAGE-254：新建方案、内置兜底方案都是「先精灵后道具」。
    #[test]
    fn new_profiles_try_heal_magic_before_items() {
        assert!(!AutoProfile::default().recover.item_first);
        assert!(!Profiles::default().active().recover.item_first);
        assert!(!FALLBACK.recover.item_first);
    }

    /// STAGE-254：存档里显式写的「先道具」照存档，不被新默认值覆盖。
    #[test]
    fn saved_item_first_is_kept_as_written() {
        let mut v = serde_json::to_value(AutoProfile::default()).unwrap();
        v["recover"]["item_first"] = serde_json::json!(true);
        let p: AutoProfile = serde_json::from_value(v).unwrap();
        assert!(p.recover.item_first);
    }

    /// STAGE-262 B 段：旧存档（没有复活三项）能读，按默认值生效：自动复活开、先精灵后道具、优先防御开。
    #[test]
    fn old_profile_without_revive_fields_loads_with_defaults() {
        let mut v = serde_json::to_value(AutoProfile::default()).unwrap();
        for key in ["revive_on", "revive_source", "revive_defend"] {
            v["recover"].as_object_mut().unwrap().remove(key);
        }
        let p: AutoProfile = serde_json::from_value(v).unwrap();
        assert!(p.recover.revive_on);
        assert_eq!(p.recover.revive_source, super::super::ReviveSource::Auto);
        assert!(p.recover.revive_defend);
    }

    /// STAGE-262 B 段：保存后文件里带上复活三项，改过的值照存档走。
    #[test]
    fn saved_revive_fields_are_written_and_kept() {
        let mut p = AutoProfile::default();
        p.recover.revive_on = false;
        p.recover.revive_source = super::super::ReviveSource::ItemOnly;
        let v = serde_json::to_value(&p).unwrap();
        assert_eq!(v["recover"]["revive_on"], serde_json::json!(false));
        assert_eq!(v["recover"]["revive_source"], serde_json::json!("item_only"));
        assert_eq!(v["recover"]["revive_defend"], serde_json::json!(true));
        let back: AutoProfile = serde_json::from_value(v).unwrap();
        assert!(!back.recover.revive_on);
        assert_eq!(back.recover.revive_source, super::super::ReviveSource::ItemOnly);
    }

    /// 满血抓的 100 能存能读；老存档里 5–95 的削血档位读进来不变。
    #[test]
    fn capture_hp_full_hp_value_round_trips_and_old_values_stay() {
        let mut v = serde_json::to_value(AutoProfile::default()).unwrap();
        v["capture"]["hp_pct"] = serde_json::json!(100);
        let p: AutoProfile = serde_json::from_value(v).unwrap();
        assert_eq!(p.capture.hp_pct, 100);
        let back = serde_json::to_value(&p).unwrap();
        assert_eq!(back["capture"]["hp_pct"], serde_json::json!(100));

        let mut old = serde_json::to_value(AutoProfile::default()).unwrap();
        old["capture"]["hp_pct"] = serde_json::json!(20);
        let p: AutoProfile = serde_json::from_value(old).unwrap();
        assert_eq!(p.capture.hp_pct, 20);
    }

    #[test]
    fn revive_defaults_are_on_auto_and_defend_first() {
        for p in [AutoProfile::default(), FALLBACK.clone(), Profiles::default().active().clone()] {
            assert!(p.recover.revive_on);
            assert_eq!(p.recover.revive_source, super::super::ReviveSource::Auto);
            assert!(p.recover.revive_defend);
        }
    }

    /// STAGE-260：战后三项的默认值：吃肉开、门槛 90%、丢弃关；新建方案、内置方案都一样。
    #[test]
    fn post_battle_defaults_are_heal_on_90_drop_off() {
        for p in [AutoProfile::default(), FALLBACK.clone(), Profiles::default().active().clone()] {
            assert!(p.recover.post_heal_item);
            assert_eq!(p.recover.post_heal_pct, 90);
            assert!(!p.recover.post_drop_item);
        }
    }

    /// STAGE-260：旧存档（没有战后三项）能读，按默认值生效。
    #[test]
    fn old_profile_without_post_battle_fields_loads_with_defaults() {
        let mut v = serde_json::to_value(AutoProfile::default()).unwrap();
        for key in ["post_heal_item", "post_heal_pct", "post_drop_item"] {
            v["recover"].as_object_mut().unwrap().remove(key);
        }
        let p: AutoProfile = serde_json::from_value(v).unwrap();
        assert!(p.recover.post_heal_item);
        assert_eq!(p.recover.post_heal_pct, 90);
        assert!(!p.recover.post_drop_item);
    }

    /// STAGE-260：方案写进文件后带着三项，读回来不变。
    #[test]
    fn saved_post_battle_fields_are_written_and_kept() {
        let mut p = AutoProfile::default();
        p.recover.post_heal_item = false;
        p.recover.post_heal_pct = 60;
        p.recover.post_drop_item = true;
        let v = serde_json::to_value(&p).unwrap();
        assert_eq!(v["recover"]["post_heal_item"], serde_json::json!(false));
        assert_eq!(v["recover"]["post_heal_pct"], serde_json::json!(60));
        assert_eq!(v["recover"]["post_drop_item"], serde_json::json!(true));
        let back: AutoProfile = serde_json::from_value(v).unwrap();
        assert!(!back.recover.post_heal_item);
        assert_eq!(back.recover.post_heal_pct, 60);
        assert!(back.recover.post_drop_item);
    }
}
