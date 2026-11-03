//! 快捷传送页的按键（STAGE-161，`docs/interaction.md`）：`↑↓` 选、`Enter` 下钻或传送、
//! `Esc` 逐级返回，`Home`/`End`/`PgUp`/`PgDn` 跳转。普通字符不抢页面按键，要打命令按 `/`，
//! 所以这里只认这几个键，别的交回原来的分发。
//!
//! 传送请求在途、角色不在世界里、或者没有控制权时，本页不发请求，只在消息栏说明原因
//! （`docs/protocol.md` 10 章）。目录不可用时 `Enter`/`r` 重新取一次。

use crossterm::event::{KeyCode, KeyEvent};
use serde_json::json;

use super::travel::{Entry, TravelAction, TravelNav};
use super::session::Pending;
use super::{App, Layer, MAP_MENU};

impl App {
    /// 快捷传送页（`MAP_MENU`）的按键。返回 true 表示这个键归本页。
    pub(super) fn travel_key(&mut self, key: KeyEvent) -> bool {
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        if world.menu != MAP_MENU || !world.nav.is(&Layer::Page(MAP_MENU)) {
            return false;
        }
        // 「1级宠物」目录（STAGE-251）开着时归它，不看地图目录是否就绪。
        if self.world.as_ref().is_some_and(|w| w.travel.pets.is_open()) {
            return self.pet_key(key);
        }
        // 「练级传送」（STAGE-279）打开时同样接管按键。
        if self.world.as_ref().is_some_and(|w| w.travel.leveling.is_open()) {
            return self.leveling_key(key);
        }
        // 目录加载中或不可用（旧服务端或查询失败）：根层的「1级宠物」照常能进（STAGE-251，它不依赖地图目录）；
        // Enter（光标不在它上面）/ r 重新取一次，其他键照旧。
        if !world.travel.ready() {
            let on_pets = world.travel.entries().get(world.travel.cursor()) == Some(&Entry::Pets);
            if on_pets && key.code == KeyCode::Enter {
                self.open_pets();
                return true;
            }
            let on_leveling = world.travel.entries().get(world.travel.cursor()) == Some(&Entry::Leveling);
            if on_leveling && key.code == KeyCode::Enter {
                self.open_leveling();
                return true;
            }
            if matches!(
                key.code,
                KeyCode::Enter | KeyCode::Char('r') | KeyCode::Char('R')
            ) {
                self.send("maps", json!({}), Pending::Maps);
                return true;
            }
            return false;
        }
        // 传送在途时不再发第二条，避免连按把同一张图传两次（需求「快速连按不重复发送」）。
        if self.session.scene_operation_busy() {
            return matches!(key.code, KeyCode::Enter | KeyCode::Up | KeyCode::Down);
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
            if let Some(world) = self.world.as_mut() {
                world.travel.navigate(nav);
            }
            return true;
        }
        match key.code {
            KeyCode::Enter => {
                let Some(world) = self.world.as_mut() else {
                    return false;
                };
                let action = world.travel.enter();
                self.apply_travel(action);
                true
            }
            // 根层再退就退出页面：交给导航栈往上一层退。
            KeyCode::Esc => {
                let Some(world) = self.world.as_mut() else {
                    return false;
                };
                matches!(world.travel.back(), TravelAction::Ascend)
            }
            _ => false,
        }
    }

    /// 传送成功后离开快捷传送页、回到场景（只在当前就在这一页时才动导航栈）。
    pub(super) fn leave_travel_page(&mut self) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        if world.nav.is(&Layer::Page(MAP_MENU)) {
            world.scene_step = None;
            world.nav.reset();
        }
    }

    fn apply_travel(&mut self, action: TravelAction) {
        match action {
            TravelAction::OpenPets => self.open_pets(),
            TravelAction::OpenLeveling => self.open_leveling(),
            TravelAction::Warp { floor, name } => {
                self.send("warp", json!({ "floor": floor }), Pending::Warp(name));
            }
            TravelAction::AlreadyHere(name) => {
                self.log.system(format!("已经在这里：{name}"));
            }
            // 下钻/退回只动本页状态，渲染下一帧就会画出来。
            TravelAction::Descend | TravelAction::Ascend | TravelAction::None | TravelAction::Leave => {}
        }
    }
}
