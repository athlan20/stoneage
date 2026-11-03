//! 抓到又被资质筛选放生的宠物（STAGE-264 第 4、5 条）：不当作抓到宠物（不占宠位、不进宠物列表），
//! 只写一行战斗记录，并进抓宠丢弃记录。协议 10.7 的 `capture` 事件带 `discarded`，此时没有 `pet_slot`。

use serde_json::Value;

use crate::app::App;
use crate::app::auto_battle::capture_log;
use crate::proto::CaptureDiscard;

/// 一条 `battle_round` 里所有放生的宠物。
pub fn discards_in_round(data: &Value) -> Vec<CaptureDiscard> {
    data.get("events")
        .and_then(Value::as_array)
        .into_iter()
        .flatten()
        .filter_map(|ev| ev.get("discarded"))
        .filter(|d| !d.is_null())
        .filter_map(|d| serde_json::from_value::<CaptureDiscard>(d.clone()).ok())
        .collect()
}

impl App {
    /// 把放生的宠物写进丢弃记录。写不进去只在消息栏提示，不影响战斗。
    pub(crate) fn log_capture_discards(&mut self, discards: Vec<CaptureDiscard>) {
        if discards.is_empty() {
            return;
        }
        let Some(store) = self.auto_store.as_ref() else {
            return;
        };
        let path = capture_log::path(store);
        let character = store.character.clone();
        for d in &discards {
            let rec = capture_log::from_discard(capture_log::now_text(), d);
            if let Err(e) = capture_log::append(&path, &character, rec) {
                self.log.error(format!("抓宠丢弃记录写不进去：{e}"));
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::app::battle::BattleState;

    #[test]
    fn finds_only_the_discarded_captures_in_a_round() {
        let data = json!({"events": [
            {"kind": "attack", "actor": 0},
            {"kind": "capture", "actor": 0, "ok": true, "target": 15, "left": [15]},
            {"kind": "capture", "actor": 0, "ok": true, "target": 15, "left": [15], "discarded": {
                "name": "乌力", "level": 1,
                "quality": [{"stat": "vital", "points": 21, "tier": -1}],
                "reason": "体力 −1 < 0"}}
        ]});
        let found = discards_in_round(&data);
        assert_eq!(found.len(), 1);
        assert_eq!(found[0].name, "乌力");
        assert_eq!(found[0].quality[0].tier, -1);
    }

    #[test]
    fn a_round_without_events_has_nothing() {
        assert!(discards_in_round(&json!({})).is_empty());
        assert!(discards_in_round(&json!({"events": [{"kind": "capture", "discarded": null}]})).is_empty());
    }

    #[test]
    fn released_capture_is_logged_and_does_not_add_a_pet() {
        use crate::app::auto_battle::capture_log;
        use crate::app::auto_battle::store::Store;
        use crate::app::{App, Options, Screen, WorldState};
        use crate::proto::{BattleStart, WorldEntry};

        let mut a = App::new(Options::default());
        a.screen = Screen::World;
        let mut w = WorldState::new(WorldEntry::default());
        w.battle = Some(BattleState::from_start(BattleStart::default()));
        a.world = Some(w);
        let dir = std::env::temp_dir().join(format!("stage264-discard-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        a.auto_store = Some(Store::open_in(&dir, "测试服", "勇者"));

        let round = json!({"turn": 1, "events": [{"kind": "capture", "actor": 0, "ok": true, "target": 15,
            "left": [15], "discarded": {"name": "乌力", "level": 1,
            "quality": [{"stat": "vital", "points": 21, "tier": -1}], "reason": "体力 −1 < 0"}}]});
        a.on_battle_push("battle_round", round);

        let path = capture_log::path(a.auto_store.as_ref().unwrap());
        let recs = capture_log::load(&path, "勇者");
        assert_eq!(recs.len(), 1, "进了丢弃记录");
        assert_eq!(recs[0].reason, "体力 −1 < 0");
        assert!(a.world.as_ref().unwrap().pets.pets.is_empty(), "不当作抓到宠物");
    }

    #[test]
    fn released_capture_reads_as_released_in_the_battle_log() {
        use crate::proto::{BattleStart, CaptureDiscard};

        let b = BattleState::from_start(BattleStart::default());
        let ev = crate::proto::BattleEvent {
            kind: "capture".into(),
            actor: Some(0),
            ok: Some(true),
            discarded: Some(CaptureDiscard {
                name: "乌力".into(),
                level: 1,
                quality: vec![],
                reason: "体力 −1 < 0".into(),
            }),
            ..Default::default()
        };
        assert_eq!(
            b.describe(&ev),
            vec!["抓到 乌力 Lv1，资质不合格已放生：体力 −1 < 0".to_string()]
        );
    }
}
