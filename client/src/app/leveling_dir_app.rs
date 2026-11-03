//! STAGE-279：「练级传送」的 App 侧：取目录（摘要与按段）、按键分发、开战/传送的请求与响应。
//! 状态在 `leveling_dir.rs`，画面在 `ui/leveling_dir_view.rs`；协议见 `docs/protocol.md` 5.70 / 5.71。

use crossterm::event::{KeyCode, KeyEvent};
use serde_json::{Value, json};

use super::App;
use super::leveling_dir::{LevelingAction, LevelingStatus, segment_min};
use super::session::Pending;
use super::travel::TravelNav;
use crate::proto::{LevelingCatalog, LevelingSpot};

impl App {
    /// 打开「练级传送」（快捷传送根层的一项）。摘要没取到过就请求一次，之后用缓存。
    pub(super) fn open_leveling(&mut self) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        let level = world.entry.status.level;
        world.travel.leveling.open(level);
        if world.travel.leveling.summary.is_none() {
            self.send(
                "leveling_catalog",
                json!({}),
                Pending::LevelingCatalog { segment: None },
            );
        }
    }

    pub(super) fn on_leveling_catalog(&mut self, data: Value, segment: Option<usize>) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        match serde_json::from_value::<LevelingCatalog>(data) {
            Ok(cat) => match segment {
                Some(seg) => {
                    let entries = cat.entries.clone().unwrap_or_default();
                    world.travel.leveling.install_segment(seg, entries);
                }
                None => world.travel.leveling.install(cat),
            },
            Err(err) => world.travel.leveling.fail(format!("目录解析失败：{err}")),
        }
    }

    pub(super) fn on_leveling_catalog_error(&mut self, reason: String) {
        if let Some(world) = self.world.as_mut() {
            world.travel.leveling.fail(reason);
        }
    }

    /// `leveling_start` 成功：写消息、退出目录、回场景。
    pub(super) fn on_leveling_start(&mut self, map: &str, mode: &str) {
        let text = if mode == "travel" {
            format!("已传送到 {map}")
        } else {
            format!("已传送到 {map}，开始遇敌")
        };
        self.log.system(text);
        if let Some(world) = self.world.as_mut() {
            world.travel.leveling.close();
        }
        self.leave_travel_page();
    }

    /// 目录页打开时的按键（`travel_key` 里、页面已经打开时调用）。返回 true 表示这个键归本页。
    pub(super) fn leveling_key(&mut self, key: KeyEvent) -> bool {
        let Some(world) = self.world.as_mut() else {
            return false;
        };
        let nav = match key.code {
            KeyCode::Up => Some(TravelNav::Up),
            KeyCode::Down => Some(TravelNav::Down),
            KeyCode::PageUp => Some(TravelNav::PageUp),
            KeyCode::PageDown => Some(TravelNav::PageDown),
            KeyCode::Home => Some(TravelNav::Home),
            KeyCode::End => Some(TravelNav::End),
            _ => None,
        };
        if let Some(nav) = nav {
            world.travel.leveling.navigate(nav);
            return true;
        }
        match key.code {
            KeyCode::Enter => {
                if world.travel.leveling.status != LevelingStatus::Ready {
                    // 摘要没取到：Enter 重新取一次。
                    self.send(
                        "leveling_catalog",
                        json!({}),
                        Pending::LevelingCatalog { segment: None },
                    );
                    if let Some(w) = self.world.as_mut() {
                        w.travel.leveling.status = LevelingStatus::Loading;
                    }
                    return true;
                }
                let action = world.travel.leveling.enter();
                self.apply_leveling_action(action);
                true
            }
            KeyCode::Esc => {
                let action = world.travel.leveling.back();
                self.apply_leveling_action(action);
                true
            }
            _ => false,
        }
    }

    fn apply_leveling_action(&mut self, action: LevelingAction) {
        match action {
            LevelingAction::Descend => self.fetch_pending_segment(),
            LevelingAction::Go { spot, mode } => self.start_leveling(spot, mode),
            LevelingAction::None | LevelingAction::Leave => {}
        }
    }

    /// 进入某段时它的练级点还没取到：按段取一次（`{"segment": 段下限}`）。
    fn fetch_pending_segment(&mut self) {
        let Some(seg) = self.world.as_ref().and_then(|w| w.travel.leveling.pending_segment()) else {
            return;
        };
        if let Some(w) = self.world.as_mut() {
            w.travel.leveling.in_flight = Some(seg);
        }
        let request = json!({ "segment": segment_min(seg) });
        self.send(
            "leveling_catalog",
            request,
            Pending::LevelingCatalog { segment: Some(seg) },
        );
    }

    fn start_leveling(&mut self, spot: LevelingSpot, mode: &'static str) {
        // 上一条还在途时不重复发（快速连按只认第一下）。
        if self.session.leveling_start_busy() {
            return;
        }
        let here = self
            .world
            .as_ref()
            .map(|w| (w.entry.position.floor, w.entry.position.x, w.entry.position.y));
        // 已经在该落点所在区域：只提示，不发请求。
        if let Some((floor, x, y)) = here {
            if floor == spot.floor && spot.rect.contains(x, y) {
                self.log.system(format!("已经在这里：{}", spot.map_name));
                return;
            }
        }
        let map = spot.map_name.clone();
        let request = json!({ "area_id": spot.area_id, "mode": mode });
        self.send(
            "leveling_start",
            request,
            Pending::LevelingStart {
                map,
                mode: mode.to_string(),
            },
        );
    }
}
