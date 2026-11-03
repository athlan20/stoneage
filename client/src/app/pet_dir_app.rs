//! STAGE-251：「1级宠物」目录的 App 侧：取目录、按键分发、开战/传送的请求与响应。
//! 状态在 `pet_dir.rs`，画面在 `ui/pet_dir_view.rs`；这里只把它们接到协议上（协议见 `docs/protocol.md` 5.62 / 5.63）。

use crossterm::event::{KeyCode, KeyEvent};
use serde_json::{Value, json};

use super::pet_dir::{PetAction, PetDirStatus};
use super::session::Pending;
use super::travel::TravelNav;
use super::App;
use crate::proto::PetCatalog;

impl App {
    /// 打开「1级宠物」（快捷传送根层的一项）。目录没取到过就请求一次，取到之后直接用缓存。
    pub(super) fn open_pets(&mut self) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        world.travel.pets.open();
        if world.travel.pets.catalog.is_none() {
            self.send("pet_encounter_catalog", json!({}), Pending::PetCatalog);
        }
    }

    pub(super) fn on_pet_catalog(&mut self, data: Value) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        match serde_json::from_value::<PetCatalog>(data) {
            Ok(cat) => world.travel.pets.install(cat),
            Err(err) => world.travel.pets.fail(format!("目录解析失败：{err}")),
        }
    }

    pub(super) fn on_pet_catalog_error(&mut self, reason: String) {
        if let Some(world) = self.world.as_mut() {
            world.travel.pets.fail(reason);
        }
    }

    /// `pet_encounter_start` 成功：写消息、退出目录、回场景。`mode` 为 travel 时只写“已传送”。
    pub(super) fn on_pet_start(&mut self, map: &str, mode: &str) {
        let text = if mode == "travel" {
            format!("已传送到 {map}")
        } else {
            format!("已传送到 {map}，开始遇敌")
        };
        self.log.system(text);
        if let Some(world) = self.world.as_mut() {
            world.travel.pets.close();
        }
        self.leave_travel_page();
    }

    /// 目录页打开时的按键（只在 `travel_key` 里、页面已经打开时调用）。返回 true 表示这个键归本页。
    pub(super) fn pet_key(&mut self, key: KeyEvent) -> bool {
        let Some(world) = self.world.as_mut() else {
            return false;
        };
        if world.travel.pets.typing {
            return self.pet_filter_key(key);
        }
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
            world.travel.pets.navigate(nav);
            return true;
        }
        match key.code {
            KeyCode::Enter => {
                if world.travel.pets.status != PetDirStatus::Ready {
                    // 目录没取到：Enter 重新取一次。
                    self.send("pet_encounter_catalog", json!({}), Pending::PetCatalog);
                    if let Some(w) = self.world.as_mut() {
                        w.travel.pets.status = PetDirStatus::Loading;
                    }
                    return true;
                }
                let action = world.travel.pets.enter();
                self.apply_pet_action(action);
                true
            }
            KeyCode::Esc => {
                let action = world.travel.pets.back();
                self.apply_pet_action(action);
                true
            }
            // `/` 或 `f`：把焦点交给筛选框（只在 L1）。
            KeyCode::Char('/') | KeyCode::Char('f') if world.travel.pets.depth() == 1 => {
                world.travel.pets.start_typing();
                true
            }
            _ => false,
        }
    }

    /// 筛选框拿着焦点时的按键：字符进框，Enter 应用，Esc 清空或交回列表。
    fn pet_filter_key(&mut self, key: KeyEvent) -> bool {
        let Some(world) = self.world.as_mut() else {
            return false;
        };
        let pets = &mut world.travel.pets;
        match key.code {
            KeyCode::Char(c) if !c.is_control() => pets.type_char(c),
            KeyCode::Backspace => pets.backspace(),
            KeyCode::Enter => pets.commit_filter(),
            KeyCode::Esc => pets.cancel_filter(),
            KeyCode::Up => pets.navigate(TravelNav::Up),
            KeyCode::Down => pets.navigate(TravelNav::Down),
            _ => return false,
        }
        true
    }

    fn apply_pet_action(&mut self, action: PetAction) {
        match action {
            PetAction::Go { pet_id, spot, mode } => {
                // 上一条还在途时不重复发（快速连按只认第一下）。
                if self.session.pet_start_busy() {
                    return;
                }
                let here = self
                    .world
                    .as_ref()
                    .map(|w| (w.entry.position.floor, w.entry.position.x, w.entry.position.y));
                // 已经在该遇敌点所在区域：只提示，不发请求。
                if let Some((floor, x, y)) = here {
                    if floor == spot.floor && spot.rect.contains(x, y) {
                        self.log.system(format!("已经在这里：{}", spot.map_name));
                        return;
                    }
                }
                let map = spot.map_name.clone();
                let request = json!({ "pet_id": pet_id, "area_id": spot.area_id, "mode": mode });
                self.send(
                    "pet_encounter_start",
                    request,
                    Pending::PetStart { map, mode: mode.to_string() },
                );
            }
            PetAction::None | PetAction::Descend | PetAction::Leave => {}
        }
    }
}
