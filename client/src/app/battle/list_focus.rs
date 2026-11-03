//! 列表面板的高亮（STAGE-107，`docs/battle-ui.md`「编号规则」「列表面板」）：打开列表时默认落在哪一行。
//! ↑↓、PgUp、PgDn 的移动和夹住规则与页面列表共用 `app/cursor.rs`；行的内容见 `list.rs`。

use super::Pick;
use super::list::Row;
use crate::app::cursor;

pub use crate::app::cursor::PAGE_STEP;

/// 默认高亮：道具优先本场上次用过的，其次第一个回复道具，最后是第一个能选的；都没有就是 0。
pub fn default_focus(rows: &[Row], pick: Pick, last_item: Option<u8>) -> usize {
    let usable = |r: &Row| r.blocked.is_none();
    let found = match pick {
        Pick::Item => last_item
            .and_then(|slot| rows.iter().position(|r| usable(r) && r.key == i64::from(slot)))
            .or_else(|| rows.iter().position(|r| usable(r) && r.heals)),
        _ => None,
    };
    found.or_else(|| rows.iter().position(usable)).unwrap_or(0)
}

/// 高亮的位置夹在列表范围内（道具被用掉后列表变短，高亮落到最后一行）。
pub fn clamp(rows: &[Row], focus: usize) -> usize {
    cursor::clamp(rows.len(), focus)
}

/// 从 `focus` 往下（`down`）或往上移动 `n` 行，只停在能选的行上（见 `cursor::step`）。
pub fn step(rows: &[Row], focus: usize, n: usize, down: bool) -> usize {
    cursor::step(rows.len(), |i| rows[i].blocked.is_none(), focus, n, down)
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 一行：`key` 是命令参数，`blocked` 表示不能选，`heals` 是回复类道具。
    fn row(key: i64, blocked: bool, heals: bool) -> Row {
        Row {
            number: None,
            key,
            name: String::new(),
            extra: String::new(),
            target: "",
            blocked: blocked.then(|| "不能选".to_string()),
            desc: String::new(),
            heals,
        }
    }

    #[test]
    fn default_focus_prefers_last_used_then_a_heal_item() {
        // 回复药（7 号，回复类）、小斧头（9 号，不能用）、毒消（12 号）。
        let rows = vec![row(7, false, true), row(9, true, false), row(12, false, false)];
        assert_eq!(default_focus(&rows, Pick::Item, Some(12)), 2, "上次用过的");
        assert_eq!(default_focus(&rows, Pick::Item, None), 0, "没有上次用过的，就是回复药");
        assert_eq!(
            default_focus(&rows, Pick::Item, Some(9)),
            0,
            "上次用的不能用，退回回复药"
        );
        assert_eq!(default_focus(&rows, Pick::Magic, Some(12)), 0, "只有道具看上次用过的");
    }

    #[test]
    fn step_skips_blocked_rows_and_stops_at_the_ends() {
        let rows = vec![row(7, false, true), row(9, true, false), row(12, false, false)];
        assert_eq!(step(&rows, 0, 1, true), 2, "中间的小斧头被跳过");
        assert_eq!(step(&rows, 2, 1, true), 2, "到头不循环");
        assert_eq!(step(&rows, 2, 1, false), 0);
        assert_eq!(step(&rows, 0, 1, false), 0);
    }

    #[test]
    fn paging_moves_by_n_and_lands_on_the_last_usable_row() {
        let rows: Vec<Row> = (0..12).map(|i| row(i, false, false)).collect();
        assert_eq!(step(&rows, 0, PAGE_STEP, true), 5);
        assert_eq!(step(&rows, 10, PAGE_STEP, true), 11, "不足一页时停在最后一行");
        assert_eq!(step(&rows, 11, PAGE_STEP, false), 6);
        assert_eq!(step(&rows, 3, PAGE_STEP, false), 0, "不足一页时停在第一行");
    }
}
