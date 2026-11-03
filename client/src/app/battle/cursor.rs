//! STAGE-167：上下沿同一列移动；左右在前后列切换，和敌左我右的站位一致。
use super::issue::check_target;
use super::slots::{self, Cell};
use super::{Act, BattleState, Pick};
use crate::proto::TargetType;
use crossterm::event::KeyCode;

pub(super) fn move_cursor(b: &mut BattleState, code: KeyCode) {
    let (act, kind) = match b.pick {
        Pick::Target { act, kind, .. } => (act, kind),
        Pick::CloneAim { .. } => (Act::Attack, TargetType::Enemy),
        _ => return,
    };
    let cur = b.cursor;
    let next = match code {
        KeyCode::Up | KeyCode::Down => {
            let step = if code == KeyCode::Up { -1 } else { 1 };
            let mut row = cur.col as isize;
            loop {
                row += step;
                if !(0..5).contains(&row) {
                    break None;
                }
                let cell = Cell {
                    col: row as usize,
                    ..cur
                };
                if selectable(b, kind, act, cell) {
                    break Some(cell);
                }
            }
        }
        KeyCode::Left | KeyCode::Right => {
            // 我方前列在左，敌方前列在右。到外边界就停住，不反向绕回。
            let front = (code == KeyCode::Left) == (cur.side == b.me_side());
            if cur.front == front {
                None
            } else {
                (0..5)
                    .map(|col| Cell { front, col, ..cur })
                    .filter(|&cell| selectable(b, kind, act, cell))
                    .min_by_key(|cell| cell.col.abs_diff(cur.col))
            }
        }
        _ => None,
    };
    if let Some(cell) = next {
        b.cursor = cell;
    }
}

fn selectable(b: &BattleState, kind: TargetType, act: Act, cell: Cell) -> bool {
    check_target(b, kind, act, slots::no_at(cell)).is_ok()
}
