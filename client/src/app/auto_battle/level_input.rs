//! 捕捉等级上下限的手输（STAGE-264 第 3 条）。
//!
//! 行表上按 `Enter` 打开输入框，输入 0–200 的整数，0 表示不限；`←→` 按 5 步进的老行为保留。
//! 非数字（含负号、小数、空）与超出范围的输入拒绝并说明原因。下限、上限都不是 0 时，
//! 下限不能大于上限：不接受、原值保持，输入框留着让玩家改（不静默交换）。

use crossterm::event::{KeyCode, KeyEvent};

use super::AutoProfile;
use super::edit::Editor;
use super::fields::Field;
use crate::app::TextInput;

/// 等级的上限。
pub const LEVEL_MAX: u32 = 200;

/// 正在手输的等级：哪一项（下限或上限）、输入框。
#[derive(Debug)]
pub struct LevelEdit {
    pub field: Field,
    pub input: TextInput,
}

/// 打开手输框（行表上按 Enter）。输入框从空开始，当前值显示在行表与提示里。
pub fn open(ed: &mut Editor, field: Field) {
    ed.level = Some(LevelEdit {
        field,
        input: TextInput::new(3),
    });
    ed.hint = Some(format!(
        "{}：输入 0–{LEVEL_MAX} 的整数后 Enter（0＝不限）· Esc 取消",
        field.label()
    ));
}

/// 输入文字转成等级。空、负数、小数、文字都不行；超出 0–200 也不行。
pub fn parse_level(text: &str) -> Result<u32, String> {
    let t = text.trim();
    if t.is_empty() {
        return Err(format!("请输入 0–{LEVEL_MAX} 的整数（0＝不限）"));
    }
    if !t.chars().all(|c| c.is_ascii_digit()) {
        return Err(format!("「{t}」不行：只能输入 0–{LEVEL_MAX} 的整数，负数和小数也不行"));
    }
    match t.parse::<u32>() {
        Ok(v) if v <= LEVEL_MAX => Ok(v),
        _ => Err(format!("{t} 超出范围：等级只能是 0–{LEVEL_MAX}")),
    }
}

/// 把新值写进草稿。下限、上限都不是 0 且下限大于上限时拒绝，草稿不动。
pub fn apply(draft: &mut AutoProfile, field: Field, value: u32) -> Result<(), String> {
    let (min, max) = match field {
        Field::CaptureLvMin => (value, draft.capture.level_max),
        Field::CaptureLvMax => (draft.capture.level_min, value),
        _ => return Err("这一项不是等级".into()),
    };
    if min != 0 && max != 0 && min > max {
        return Err(format!("下限 {min} 大于上限 {max}，没有改动；请重新输入（Esc 取消）"));
    }
    match field {
        Field::CaptureLvMin => draft.capture.level_min = value,
        _ => draft.capture.level_max = value,
    }
    Ok(())
}

/// 手输框里的按键。Enter 确认，Esc 取消，其余交给输入框（含 ←→ 移动光标）。
pub fn level_key(ed: &mut Editor, key: KeyEvent) {
    let Some(mut le) = ed.level.take() else {
        return;
    };
    match key.code {
        KeyCode::Esc => ed.hint = Some(format!("已取消「{}」的手输", le.field.label())),
        KeyCode::Enter => {
            match parse_level(le.input.text()).and_then(|v| apply(&mut ed.draft, le.field, v).map(|()| v)) {
                Ok(v) => {
                    let shown = if v == 0 {
                        "不限".to_string()
                    } else {
                        format!("{v} 级")
                    };
                    ed.dirty = true;
                    ed.hint = Some(format!("{}已改为 {shown}，记得「保存并应用」", le.field.label()));
                }
                Err(msg) => {
                    ed.hint = Some(msg);
                    ed.level = Some(le);
                }
            }
        }
        _ => {
            le.input.handle_key(&key);
            ed.level = Some(le);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn legal_levels_parse_including_one_to_four() {
        assert_eq!(parse_level("1"), Ok(1));
        assert_eq!(parse_level("4"), Ok(4));
        assert_eq!(parse_level(" 0 "), Ok(0), "0 仍是不限");
        assert_eq!(parse_level("200"), Ok(200));
    }

    #[test]
    fn illegal_levels_are_refused_with_a_reason() {
        for bad in ["", "-3", "2.5", "abc", "１２", "201", "999"] {
            let err = parse_level(bad).expect_err(bad);
            assert!(err.contains("0–200"), "{bad}: {err}");
        }
        assert!(parse_level("201").unwrap_err().contains("超出范围"));
        assert!(parse_level("-3").unwrap_err().contains("负数"));
    }

    #[test]
    fn min_above_max_is_not_accepted_and_draft_stays() {
        let mut p = AutoProfile::default();
        p.capture.level_min = 1;
        p.capture.level_max = 3;
        assert!(apply(&mut p, Field::CaptureLvMin, 5).is_err());
        assert_eq!(p.capture.level_min, 1, "原值保持");
        assert!(apply(&mut p, Field::CaptureLvMin, 3).is_ok(), "相等可以");
        assert!(apply(&mut p, Field::CaptureLvMax, 2).is_err(), "上限不能小于已有下限");
        assert_eq!(p.capture.level_max, 3);
    }

    #[test]
    fn zero_means_unlimited_so_no_pair_check() {
        let mut p = AutoProfile::default();
        p.capture.level_min = 9;
        p.capture.level_max = 0;
        assert!(apply(&mut p, Field::CaptureLvMax, 2).is_err(), "上限 2 < 下限 9");
        assert!(apply(&mut p, Field::CaptureLvMax, 0).is_ok(), "上限 0 就是不限");
        assert!(apply(&mut p, Field::CaptureLvMin, 0).is_ok());
        assert_eq!((p.capture.level_min, p.capture.level_max), (0, 0));
    }
}
