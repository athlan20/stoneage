//! 列表光标的移动（STAGE-113，`docs/interaction.md`「页面选择」）：战斗列表面板（STAGE-107）和各页的列表共用。
//! 只停在能选的行上（`usable`），到头不循环，列表变短时夹住。行的内容由调用方决定，这里只看下标。

/// PgUp、PgDn 一次移动的行数。
pub const PAGE_STEP: usize = 5;

/// 把 `focus` 夹到 `0..len` 之内；`len` 为 0 时是 0。
pub fn clamp(len: usize, focus: usize) -> usize {
    focus.min(len.saturating_sub(1))
}

/// 离 `focus` 最近的能选的一行：先往下找，找不到再往上找；一行都能选的都没有时是 None。
pub fn nearest(len: usize, usable: impl Fn(usize) -> bool, focus: usize) -> Option<usize> {
    let focus = clamp(len, focus);
    (focus..len)
        .find(|&i| usable(i))
        .or_else(|| (0..focus).rev().find(|&i| usable(i)))
}

/// 从 `focus` 往下（`down`）或往上移动 `n` 行，只停在能选的行上。
/// `n` 为 1 时是方向键；`n` 为 `PAGE_STEP` 时是 PgUp、PgDn。找不到能选的就停在原地。
pub fn step(len: usize, usable: impl Fn(usize) -> bool, focus: usize, n: usize, down: bool) -> usize {
    let found = if down {
        (focus + n..len)
            .find(|&i| usable(i))
            .or_else(|| (focus + 1..len).rev().find(|&i| usable(i)))
    } else {
        focus
            .checked_sub(n)
            .and_then(|far| (0..=far).rev().find(|&i| usable(i)))
            .or_else(|| (0..focus).find(|&i| usable(i)))
    };
    found.unwrap_or(focus)
}

#[cfg(test)]
mod tests {
    use super::*;

    /// 第 1 行是空的（不能选），其余都能选。
    fn usable(i: usize) -> bool {
        i != 1
    }

    #[test]
    fn clamp_keeps_focus_inside_the_list() {
        assert_eq!(clamp(3, 7), 2);
        assert_eq!(clamp(0, 4), 0, "空列表是 0");
    }

    #[test]
    fn nearest_prefers_below_then_above() {
        assert_eq!(nearest(4, usable, 1), Some(2), "原来的行不能选，先找下面");
        assert_eq!(nearest(4, usable, 3), Some(3));
        assert_eq!(nearest(2, |i| i == 0, 1), Some(0), "下面没有，就找上面");
        assert_eq!(nearest(2, |_| false, 0), None);
    }

    #[test]
    fn step_skips_unusable_rows_and_stops_at_the_ends() {
        assert_eq!(step(4, usable, 0, 1, true), 2, "跳过第 1 行");
        assert_eq!(step(4, usable, 3, 1, true), 3, "到头不循环");
        assert_eq!(step(4, usable, 2, 1, false), 0, "往上跳过第 1 行");
        assert_eq!(step(4, usable, 0, 1, false), 0);
    }

    #[test]
    fn paging_lands_on_the_last_or_first_usable_row() {
        assert_eq!(step(12, |_| true, 0, PAGE_STEP, true), 5);
        assert_eq!(step(12, |_| true, 10, PAGE_STEP, true), 11, "不足一页时停在最后一行");
        assert_eq!(step(12, |_| true, 11, PAGE_STEP, false), 6);
        assert_eq!(step(12, |_| true, 3, PAGE_STEP, false), 0, "不足一页时停在第一行");
    }
}
