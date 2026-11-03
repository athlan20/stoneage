//! 界面绘制。风格沿用 `demo/ratatui-demo`：带边框的面板、16 色具名颜色、Tabs 菜单；
//! 各页布局见 `docs/login.md` 与 `docs/ui.md`。

mod battle;
mod autosettings;
mod autosettings_input;
mod capture_log_page;
mod characters;
pub mod charpage;
mod charpage_magic;
mod create;
mod dialog;
mod dialog_menu;
mod dialog_ranking;
mod durability;
mod browse;
mod exit;
mod gamemenu;
mod inventpage;
mod inventpage_inlay;
mod login;
mod messagepage;
mod petdetail;
mod petmerge;
mod pet_dir_detail;
mod pet_dir_view;
mod leveling_dir_view;
mod petpage;
mod petsummary;
mod questpage;
mod scene;
mod scene_encounter;
mod scene_players;
mod chat_panel;
mod scene_root;
mod party_popup;
mod trade_window;
mod socialpage;
mod social_settings;
mod system;
mod travelpage;
mod widgets;
mod wikipage;
mod world;
mod worldmap;
mod worldmap_data;
// 世界图入口对照表：STAGE-161 起地图页改成快捷传送目录，这张表暂时只有出生地选择页和以后的
// 主界面会用到，先留着不删（生成文件，由 tools/gen-worldmap.py 产出）。
#[allow(dead_code)]
mod worldmap_floors;

use ratatui::Frame;
use ratatui::layout::Alignment;
use ratatui::widgets::Paragraph;

use crate::app::{App, Screen};

pub const MIN_WIDTH: u16 = 80;
pub const MIN_HEIGHT: u16 = 24;

pub fn render(frame: &mut Frame, app: &mut App) {
    let area = frame.area();
    if area.width < MIN_WIDTH || area.height < MIN_HEIGHT {
        let text = format!(
            "窗口太小（至少 {MIN_WIDTH}×{MIN_HEIGHT}，当前 {}×{}）",
            area.width, area.height
        );
        let y = widgets::centered(area, area.width, 1);
        frame.render_widget(Paragraph::new(text).alignment(Alignment::Center), y);
        return;
    }
    if app.exit_dialog.is_some() { exit::render(frame,app,area);return; }
    match app.screen {
        Screen::Login => login::render_login(frame, app, area),
        Screen::Register => login::render_register(frame, app, area),
        Screen::Characters => characters::render(frame, app, area),
        Screen::Create => create::render(frame, app, area),
        Screen::World => {
            world::render(frame, app, area);
            chat_panel::draw(frame, app, area);
        }
    }
    if app.screen == Screen::World {
        party_popup::render(frame, app, area);
        trade_window::render(frame, app, area);
    }
    exit::render(frame,app,area);
}
