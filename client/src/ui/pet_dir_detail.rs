//! STAGE-251：「1级宠物」目录的右侧详情（宽屏才有，`docs/ui.md`「快捷传送页」）。
//! 字段只用 `proto/pet_catalog.rs` 里真有的；数据里没有的（例如候选敌群的构成）一律不显示，不编造。

use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};

use crate::app::pet_dir::{PetDir, PetLevel};
use crate::proto::{PetEntry, PetSpot};

const HEAD: Style = Style::new().fg(Color::Yellow).add_modifier(Modifier::BOLD);

/// 当前层选中项的详情：L1 是宠物，L2/L3 是遇敌点。
pub fn detail_lines(pets: &PetDir) -> Vec<Line<'static>> {
    match pets.level() {
        PetLevel::Pets => pets.selected_pet().map(|p| pet_detail(&p)).unwrap_or_default(),
        PetLevel::Spots { .. } | PetLevel::Actions { .. } => {
            pets.current_spot().map(|s| spot_detail(&s)).unwrap_or_default()
        }
    }
}

fn pet_detail(p: &PetEntry) -> Vec<Line<'static>> {
    vec![
        Line::from(Span::styled(p.name.clone(), HEAD)),
        Line::from(format!(
            "属性 地 {}  水 {}  火 {}  风 {}",
            p.atoms.earth, p.atoms.water, p.atoms.fire, p.atoms.wind
        )),
        Line::from(format!("体型 {}  捕捉难度 {}", p.size, p.catch_difficulty)),
        Line::from(format!("俗称 {}", or_dash(&p.aliases.join("、")))),
        Line::from(format!("系列 {}", or_dash(&p.series))),
        Line::from(format!("可遇点 {} 处", p.spot_count)),
    ]
}

fn spot_detail(s: &PetSpot) -> Vec<Line<'static>> {
    let r = s.rect;
    vec![
        Line::from(Span::styled(format!("{}（{}）", s.map_name, s.floor), HEAD)),
        Line::from(format!("区域 {}", s.number)),
        Line::from(format!("矩形 ({},{})–({},{})", r.x1, r.y1, r.x2, r.y2)),
        Line::from(format!("落点 ({},{})", s.anchor.x, s.anchor.y)),
        Line::from(format!("同场上限 {}，约 {} 格一场", s.max_enemies, s.steps)),
        Line::from(format!("同点其他 1 级宠 {}", or_dash(&s.also_level1.join("、")))),
    ]
}

fn or_dash(text: &str) -> String {
    if text.is_empty() {
        "—".to_string()
    } else {
        text.to_string()
    }
}
