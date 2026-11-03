//! 动作层的网格布局（STAGE-153，`docs/battle-ui.md`「指令输入」）：人物 4 列 × 2 行、宠物 2 列 × 2 行。
//! 绘制、高亮、方向键导航、数字激活共用这一份布局，不再各写一份列数（原来画的是每行 4 项，
//! 导航却按一条线走，上下和左右混在一起）。
//!
//! 规则：左右只在当前行换列，到边界停住、不循环；上下到相邻行的同一列，那一列没有条目时
//! 取最近的有效列（同距取靠左），整行没有条目就停住。

/// 人物指令网格的列数：第一行「攻击、精灵、道具、防御」，第二行「捕捉、逃跑、换宠、自动挂机」。
pub const PLAYER_COLS: usize = 4;

/// 宠物指令网格的列数：「攻击、技能 / 防御、自动挂机」。
pub const PET_COLS: usize = 2;

/// 方向（只用于网格导航）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Dir {
    Left,
    Right,
    Up,
    Down,
}

/// 一层的网格：列数和条目数。行数由它算出来，最后一行可以不满。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Grid {
    pub cols: usize,
    pub len: usize,
}

impl Grid {
    pub fn new(cols: usize, len: usize) -> Self {
        Self { cols: cols.max(1), len }
    }

    /// 一共几行（最后一个不满的行也算一行）。
    pub fn rows(&self) -> usize {
        self.len.div_ceil(self.cols)
    }

    /// 这个下标在第几行、第几列。
    pub fn cell(&self, index: usize) -> Option<(usize, usize)> {
        (index < self.len).then(|| (index / self.cols, index % self.cols))
    }

    /// 第 `row` 行第 `col` 列有没有条目；有就返回下标。
    pub fn index(&self, row: usize, col: usize) -> Option<usize> {
        if col >= self.cols {
            return None;
        }
        let index = row * self.cols + col;
        (index < self.len).then_some(index)
    }

    /// 这一行实际有几个条目（最后一行可能不满）。
    pub fn row_len(&self, row: usize) -> usize {
        self.len.saturating_sub(row * self.cols).min(self.cols)
    }

    /// 从 `index` 往 `dir` 移动一格；移动不了（边界、目标行没有条目）就停在原地。
    pub fn step(&self, index: usize, dir: Dir) -> usize {
        let Some((row, col)) = self.cell(index) else {
            return index;
        };
        match dir {
            Dir::Left => {
                let next = col.saturating_sub(1);
                self.index(row, next).unwrap_or(index)
            }
            Dir::Right => self.index(row, col + 1).unwrap_or(index),
            Dir::Up if row == 0 => index,
            Dir::Down if row + 1 >= self.rows() => index,
            Dir::Up => self.nearest(row - 1, col).unwrap_or(index),
            Dir::Down => self.nearest(row + 1, col).unwrap_or(index),
        }
    }

    /// 目标行里离 `col` 最近的列；这一行没有条目返回 None。
    /// 一行的条目总是从第 0 列开始连续的，所以最近列就是把列号夹进这一行的范围（同距取靠左）。
    fn nearest(&self, row: usize, col: usize) -> Option<usize> {
        let len = self.row_len(row);
        if len == 0 {
            return None;
        }
        self.index(row, col.min(len - 1))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn player() -> Grid {
        Grid::new(PLAYER_COLS, 8)
    }

    fn pet() -> Grid {
        Grid::new(PET_COLS, 4)
    }

    #[test]
    fn player_grid_is_four_by_two() {
        let g = player();
        assert_eq!((g.rows(), g.row_len(0), g.row_len(1)), (2, 4, 4));
        assert_eq!(g.cell(0), Some((0, 0)));
        assert_eq!(g.cell(3), Some((0, 3)), "防御在第一行最后一列");
        assert_eq!(g.cell(4), Some((1, 0)), "捕捉在第二行第一个");
        assert_eq!(g.cell(7), Some((1, 3)), "自动挂机在第二行最后一列");
        assert_eq!(g.cell(8), None);
    }

    #[test]
    fn pet_grid_is_two_by_two() {
        let g = pet();
        assert_eq!((g.rows(), g.row_len(0), g.row_len(1)), (2, 2, 2));
        assert_eq!(g.step(0, Dir::Down), 2, "攻击下面防御");
        assert_eq!(g.step(1, Dir::Down), 3, "技能下面自动挂机");
        assert_eq!(g.step(2, Dir::Up), 0);
        assert_eq!(g.step(3, Dir::Up), 1);
    }

    #[test]
    fn up_down_cross_rows_and_keep_the_column() {
        let g = player();
        assert_eq!(g.step(0, Dir::Down), 4, "攻击 ↓ 捕捉");
        assert_eq!(g.step(1, Dir::Down), 5, "精灵 ↓ 逃跑");
        assert_eq!(g.step(2, Dir::Down), 6, "道具 ↓ 换宠");
        assert_eq!(g.step(3, Dir::Down), 7, "防御 ↓ 自动挂机");
        assert_eq!(g.step(7, Dir::Up), 3, "反向 ↑ 返回");
        assert_eq!(g.step(4, Dir::Up), 0);
    }

    #[test]
    fn left_right_stay_in_the_row_and_stop_at_edges() {
        let g = player();
        assert_eq!(g.step(0, Dir::Right), 1);
        assert_eq!(g.step(3, Dir::Right), 3, "行末横移不跨到下一行");
        assert_eq!(g.step(4, Dir::Left), 4, "行首不再往左");
        assert_eq!(g.step(6, Dir::Right), 7);
    }

    #[test]
    fn up_down_stop_at_the_edges() {
        let g = player();
        assert_eq!(g.step(0, Dir::Up), 0);
        assert_eq!(g.step(4, Dir::Down), 4);
    }

    #[test]
    fn missing_column_falls_back_to_the_nearest_left_one() {
        // 最后一行只有 2 个条目（7 个动作 + 管理项之外的通用情形）。
        let ragged = Grid::new(4, 6);
        assert_eq!(ragged.row_len(1), 2);
        assert_eq!(ragged.step(2, Dir::Down), 5, "同列不存在时取最近列");
        assert_eq!(ragged.step(3, Dir::Down), 5, "超出这一行的列夹到最后一列");
        let tie = Grid::new(4, 5);
        assert_eq!(tie.row_len(1), 1);
        assert_eq!(tie.step(3, Dir::Down), 4);
        // 目标行没有条目就停住。
        let empty = Grid::new(4, 4);
        assert_eq!(empty.step(0, Dir::Down), 0);
    }
}
