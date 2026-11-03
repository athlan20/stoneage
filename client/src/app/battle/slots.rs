//! 战场的站位与编号（`docs/battle-ui.md`「总体布局」「单位序号」）。
//! 协议里的单位号（0–19）和界面上的编号（前排 1–5、后排 6–10）是两套，这里负责换算，按键和画面共用。

/// 逻辑一排的站位顺序（左右战场中从上到下显示）（原版顺序：中间的 0 号最先安排，向两边展开）。
pub const ROW_ORDER: [u8; 5] = [3, 1, 0, 2, 4];

/// 战场上的一个格子：哪一方、前排还是后排、逻辑一排里的序号（左右战场中从上到下，0–4）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Cell {
    pub side: u8,
    pub front: bool,
    pub col: usize,
}

/// 单位号所在的格子。`no / 10` 是哪一方；`no % 10 >= 5` 是前排（宠物），否则是后排（人）。
pub fn cell_of(no: u8) -> Cell {
    let within = no % 10;
    let slot = within % 5;
    let col = ROW_ORDER.iter().position(|&s| s == slot).unwrap_or(0);
    Cell { side: no / 10, front: within >= 5, col }
}

/// 格子里的单位号。
pub fn no_at(cell: Cell) -> u8 {
    cell.side * 10 + if cell.front { 5 } else { 0 } + ROW_ORDER[cell.col]
}

/// 界面上的编号：前排 1–5、后排 6–10，从上到下。
pub fn number_of(cell: Cell) -> u8 {
    cell.col as u8 + if cell.front { 1 } else { 6 }
}

/// 按界面编号（1–10）找格子；编号不在范围内返回 None。
pub fn cell_of_number(side: u8, number: u8) -> Option<Cell> {
    match number {
        1..=5 => Some(Cell { side, front: true, col: usize::from(number - 1) }),
        6..=10 => Some(Cell { side, front: false, col: usize::from(number - 6) }),
        _ => None,
    }
}

/// 另一方（0 和 1 互换）。
pub fn other_side(side: u8) -> u8 {
    1 - side
}

/// 界面上的名字，例如“我 3”“敌 6”。`me_side` 是己方（`you / 10`）。
pub fn label(me_side: u8, no: u8) -> String {
    let cell = cell_of(no);
    let who = if cell.side == me_side { "我" } else { "敌" };
    format!("{who} {}", number_of(cell))
}

/// 字幕里用的紧凑编号，例如“我3”“敌6”（没有空格，`docs/battle-ui.md`「中线字幕」）。
pub fn short_label(me_side: u8, no: u8) -> String {
    label(me_side, no).replace(' ', "")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn numbers_follow_the_row_order() {
        // 敌方前排（宠物）左起第 1 个是 3 号位（协议 15 号），中间的 0 号位在第 3 列。
        assert_eq!(number_of(cell_of(15)), 3);
        assert_eq!(number_of(cell_of(16)), 2);
        // 后排（人）的 3 号位在第 1 列，编号是 6。
        assert_eq!(number_of(cell_of(3)), 6);
        assert_eq!(label(0, 5), "我 3");
        assert_eq!(label(0, 15), "敌 3");
        assert_eq!(label(0, 0), "我 8");
    }

    #[test]
    fn number_round_trips_for_every_unit() {
        for no in 0..20u8 {
            let cell = cell_of(no);
            assert_eq!(no_at(cell), no);
            assert_eq!(cell_of_number(cell.side, number_of(cell)), Some(cell));
        }
    }

    #[test]
    fn numbers_out_of_range_are_rejected() {
        assert_eq!(cell_of_number(1, 0), None);
        assert_eq!(cell_of_number(1, 11), None);
    }
}

/// STAGE-167：左右战场的物理列：敌后、敌前、我前、我后。逻辑编号不变。
pub fn screen_column(me_side: u8, no: u8) -> usize {
    match (no / 10 == me_side, no % 10 >= 5) {
        (false, false) => 0,
        (false, true) => 1,
        (true, true) => 2,
        (true, false) => 3,
    }
}