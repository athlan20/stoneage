//! 捕捉的资质筛选（STAGE-264 第 4 条）：方案 `capture.keep`，四项各一个“最差不低于”的档位。
//!
//! 档位 −4…0，0 为满；不限（缺省）和 −4 都等于不限。全部有阈值的项都满足才留下（AND）。
//! 只有 −3…0 的阈值才发给服务端（协议 10.5 的 `capture.keep`）；全不限时不带 `keep` 字段。

use serde::{Deserialize, Serialize};
use serde_json::{Map, Value, json};

/// 四项资质，顺序与协议 10.7 的 `quality` 一致。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Stat {
    Vital,
    Strength,
    Toughness,
    Dexterity,
}

impl Stat {
    pub const ALL: [Stat; 4] = [Stat::Vital, Stat::Strength, Stat::Toughness, Stat::Dexterity];

    /// 协议里的键名。
    pub fn key(self) -> &'static str {
        match self {
            Stat::Vital => "vital",
            Stat::Strength => "strength",
            Stat::Toughness => "toughness",
            Stat::Dexterity => "dexterity",
        }
    }

    /// 界面上的名字（设置页的行名、丢弃记录里的资质）。
    pub fn label(self) -> &'static str {
        match self {
            Stat::Vital => "体力",
            Stat::Strength => "腕力",
            Stat::Toughness => "耐久力",
            Stat::Dexterity => "速度",
        }
    }
}

/// 每项的档位阈值；`None` 是不限。文件里缺字段就是不限，保存时四项都写出来（不限写成省略的键）。
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct CaptureKeep {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub vital: Option<i8>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub strength: Option<i8>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub toughness: Option<i8>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub dexterity: Option<i8>,
}

/// `←→` 的循环：不限 → 0 → −1 → −2 → −3 → 不限（−4 不单列，等同不限）。
const CYCLE: [Option<i8>; 5] = [None, Some(0), Some(-1), Some(-2), Some(-3)];

impl CaptureKeep {
    /// 四项都不限（方案的默认值；常量写法，给 `static` 用）。
    pub const UNLIMITED: CaptureKeep = CaptureKeep {
        vital: None,
        strength: None,
        toughness: None,
        dexterity: None,
    };

    pub fn get(&self, stat: Stat) -> Option<i8> {
        match stat {
            Stat::Vital => self.vital,
            Stat::Strength => self.strength,
            Stat::Toughness => self.toughness,
            Stat::Dexterity => self.dexterity,
        }
    }

    fn slot(&mut self, stat: Stat) -> &mut Option<i8> {
        match stat {
            Stat::Vital => &mut self.vital,
            Stat::Strength => &mut self.strength,
            Stat::Toughness => &mut self.toughness,
            Stat::Dexterity => &mut self.dexterity,
        }
    }

    /// 这一项真正起作用的阈值：−3…0 才算，不限和 −4 都当没有。
    fn effective(&self, stat: Stat) -> Option<i8> {
        self.get(stat).filter(|v| (-3..=0).contains(v))
    }

    /// 四项都是不限。
    pub fn is_unlimited(&self) -> bool {
        Stat::ALL.iter().all(|s| self.effective(*s).is_none())
    }

    /// `←→` 调一项。`dir` 为负往左（更严），为正往右。
    pub fn step(&mut self, stat: Stat, dir: i64) {
        let at = |v: Option<i8>| match v {
            Some(0) => 1,
            Some(-1) => 2,
            Some(-2) => 3,
            Some(-3) => 4,
            _ => 0,
        };
        let next = (at(self.get(stat)) as i64 + dir).rem_euclid(CYCLE.len() as i64) as usize;
        *self.slot(stat) = CYCLE[next];
    }

    /// 发给服务端的 `keep`：只带有阈值的项；全不限返回 `None`，调用方就不带这个字段。
    pub fn wire(&self) -> Option<Value> {
        let mut map = Map::new();
        for stat in Stat::ALL {
            if let Some(v) = self.effective(stat) {
                map.insert(stat.key().into(), json!(v));
            }
        }
        (!map.is_empty()).then_some(Value::Object(map))
    }
}

/// 把 `keep` 写进 `battle_command` 的数据：全不限就什么也不加（与没有这个功能时逐字一致）。
pub fn attach_keep(data: &mut Value, keep: &CaptureKeep) {
    if let (Some(wire), Some(obj)) = (keep.wire(), data.as_object_mut()) {
        obj.insert("keep".into(), wire);
    }
}

/// 档位的显示：不限，或 `−2 档`（0 写成 `0 档（满）`）。
pub fn tier_text(v: Option<i8>) -> String {
    match v.filter(|v| (-3..=0).contains(v)) {
        None => "不限".into(),
        Some(0) => "0 档（满）".into(),
        Some(v) => format!("{} 档", tier_sign(i64::from(v))),
    }
}

/// 设置页一行的值与括注：括注写明语义。
pub fn value_text(keep: &CaptureKeep, stat: Stat) -> (String, String) {
    match keep.effective(stat) {
        None => (tier_text(None), "不筛选这一项".into()),
        Some(0) => (tier_text(Some(0)), "只留资质满的（0 档）".into()),
        Some(v) => (
            tier_text(Some(v)),
            format!("最差不低于 {} 档；0 为满", tier_sign(i64::from(v))),
        ),
    }
}

/// 档位的带符号写法，负号用全角的 `−`，和战斗里的资质显示一致。
pub fn tier_sign(v: i64) -> String {
    if v < 0 {
        format!("−{}", v.unsigned_abs())
    } else {
        v.to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_is_unlimited_and_sends_nothing() {
        let k = CaptureKeep::default();
        assert!(k.is_unlimited());
        assert_eq!(k.wire(), None);
        assert_eq!(serde_json::to_value(&k).unwrap(), json!({}));
    }

    #[test]
    fn old_profile_without_keep_reads_as_unlimited() {
        let k: CaptureKeep = serde_json::from_value(json!({})).unwrap();
        assert!(k.is_unlimited());
    }

    #[test]
    fn round_trips_only_the_set_items() {
        let mut k = CaptureKeep::default();
        k.step(Stat::Vital, -1); // 不限 → −3（往左一步）
        k.step(Stat::Strength, 1); // 不限 → 0
        let v = serde_json::to_value(&k).unwrap();
        assert_eq!(v, json!({"vital": -3, "strength": 0}));
        let back: CaptureKeep = serde_json::from_value(v).unwrap();
        assert_eq!(back, k);
    }

    #[test]
    fn wire_carries_only_thresholds_and_skips_minus_four() {
        let mut k = CaptureKeep {
            vital: Some(-2),
            strength: Some(-4),
            dexterity: Some(0),
            ..Default::default()
        };
        assert_eq!(k.wire(), Some(json!({"vital": -2, "dexterity": 0})));
        k.vital = None;
        k.dexterity = None;
        assert_eq!(k.wire(), None, "−4 等于不限，全不限就不带 keep");
    }

    #[test]
    fn step_cycles_both_ways_through_the_five_stops() {
        let mut k = CaptureKeep::default();
        let seen: Vec<Option<i8>> = (0..5)
            .map(|_| {
                k.step(Stat::Toughness, 1);
                k.get(Stat::Toughness)
            })
            .collect();
        assert_eq!(seen, vec![Some(0), Some(-1), Some(-2), Some(-3), None]);
        k.step(Stat::Toughness, -1);
        assert_eq!(k.get(Stat::Toughness), Some(-3));
    }

    #[test]
    fn minus_four_in_a_file_reads_as_unlimited_on_screen() {
        let k = CaptureKeep {
            vital: Some(-4),
            ..Default::default()
        };
        assert_eq!(value_text(&k, Stat::Vital).0, "不限");
        let mut k2 = k.clone();
        k2.step(Stat::Vital, 1);
        assert_eq!(k2.vital, Some(0));
    }

    #[test]
    fn attach_adds_keep_only_when_something_is_set() {
        let base = json!({"turn": 1, "actor": "player", "action": "capture", "target": 15});
        let mut plain = base.clone();
        attach_keep(&mut plain, &CaptureKeep::default());
        assert_eq!(plain, base, "全不限：字段不出现");
        let mut set = base.clone();
        attach_keep(
            &mut set,
            &CaptureKeep {
                dexterity: Some(-1),
                ..Default::default()
            },
        );
        assert_eq!(set["keep"], json!({"dexterity": -1}), "只带有阈值的项");
    }

    #[test]
    fn text_says_what_the_threshold_means() {
        let k = CaptureKeep {
            vital: Some(-2),
            dexterity: Some(0),
            ..Default::default()
        };
        assert_eq!(
            value_text(&k, Stat::Vital),
            ("−2 档".into(), "最差不低于 −2 档；0 为满".into())
        );
        assert_eq!(value_text(&k, Stat::Dexterity).0, "0 档（满）");
        assert_eq!(value_text(&k, Stat::Strength).0, "不限");
    }
}

#[cfg(test)]
mod persist_tests {
    use crate::app::auto_battle::store::{Profiles, Store};

    #[test]
    fn old_file_without_keep_loads_unlimited_and_saves_keep() {
        let dir = std::env::temp_dir().join(format!("stage264-keep-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        let probe = Store::open_in(&dir, "测试服", "勇者");
        let mut v = serde_json::to_value(Profiles::default()).unwrap();
        v["profiles"][0]["capture"].as_object_mut().unwrap().remove("keep");
        std::fs::create_dir_all(probe.path().parent().unwrap()).unwrap();
        std::fs::write(probe.path(), v.to_string()).unwrap();

        let mut s = Store::open_in(&dir, "测试服", "勇者");
        assert!(s.active().capture.keep.is_unlimited(), "旧存档缺字段按不限");
        s.profiles.active_mut().capture.keep.vital = Some(-2);
        s.save().unwrap();
        let text = std::fs::read_to_string(s.path()).unwrap();
        assert!(text.contains("\"vital\": -2"), "保存后文件里带上 keep");
        let back = Store::open_in(&dir, "测试服", "勇者");
        assert_eq!(back.active().capture.keep.vital, Some(-2));
    }
}
