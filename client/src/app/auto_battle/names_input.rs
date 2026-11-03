//! 名单编辑框（STAGE-173 M07/M10/M13，STAGE-264 第 2 条）：输入框里打名字，Enter 加入名单。
//!
//! 一次可以输入多个：用半角 `|` 或全角 `｜` 分隔。切分规则：按分隔符切开 → 去首尾空白 →
//! 丢空项 → 组内去重；再与已有名单去重后追加。单条仍按包含匹配（`Capture::name_ok`）。
//! ↑↓ 选、Del 删、Esc 返回的老按键不变。

use crossterm::event::{KeyCode, KeyEvent};

use super::edit::Editor;
use super::fields::Field;
use crate::app::TextInput;

/// 输入框字数上限：一次输几个名字都要放得下。
pub const INPUT_MAX_CHARS: usize = 120;

/// 正在录入的名单：哪一项、输入框、名单里选中的一项（删除用）。
#[derive(Debug)]
pub struct NameEdit {
    pub field: Field,
    pub input: TextInput,
    pub sel: usize,
}

/// 把输入切成名字：按 `|` / `｜` 切开，去首尾空白，丢空项，组内去重。
pub fn split_names(text: &str) -> Vec<String> {
    let mut out: Vec<String> = Vec::new();
    for part in text.split(|c: char| c == '|' || c == '｜') {
        let name = part.trim();
        if !name.is_empty() && !out.iter().any(|n| n == name) {
            out.push(name.to_string());
        }
    }
    out
}

/// 一次追加的结果：真正加入的名字，以及因为名单里已有而跳过的个数。
#[derive(Debug, Default, PartialEq, Eq)]
pub struct Added {
    pub names: Vec<String>,
    pub skipped: usize,
}

/// 把输入追加进名单，和已有名单去重。
pub fn add_names(list: &mut Vec<String>, text: &str) -> Added {
    let mut added = Added::default();
    for name in split_names(text) {
        if list.contains(&name) {
            added.skipped += 1;
        } else {
            list.push(name.clone());
            added.names.push(name);
        }
    }
    added
}

/// 打开名单编辑框（行表上按 Enter）。
pub fn open(ed: &mut Editor, field: Field) {
    let mut input = TextInput::new(INPUT_MAX_CHARS);
    input.clear();
    ed.names = Some(NameEdit { field, input, sel: 0 });
    ed.hint = Some(format!(
        "{}：打名字后 Enter 加入 · ↑↓ 选 · Del 删除 · Esc 返回",
        field.label()
    ));
}

/// 名单编辑框里的按键。Esc 关掉编辑框（不改名单），其余键留在框里。
pub fn names_key(ed: &mut Editor, key: KeyEvent) {
    let Some(mut ne) = ed.names.take() else {
        return;
    };
    match key.code {
        KeyCode::Esc => ed.hint = Some(format!("已退出「{}」编辑", ne.field.label())),
        KeyCode::Enter => {
            enter(ed, &mut ne);
            ed.names = Some(ne);
        }
        KeyCode::Delete => {
            delete(ed, &mut ne);
            ed.names = Some(ne);
        }
        KeyCode::Up => {
            ne.sel = ne.sel.saturating_sub(1);
            ed.names = Some(ne);
        }
        KeyCode::Down => {
            ne.sel = ne.sel.saturating_add(1);
            ed.names = Some(ne);
        }
        _ => {
            ne.input.handle_key(&key);
            ed.names = Some(ne);
        }
    }
}

/// Enter：把输入切开、去重后追加；一个都加不进去时保留输入，方便改。
fn enter(ed: &mut Editor, ne: &mut NameEdit) {
    let text = ne.input.text().to_string();
    let names = split_names(&text);
    if names.is_empty() {
        ed.hint = Some("名字不能为空".into());
        return;
    }
    let Some(list) = Field::name_list(&mut ed.draft, ne.field) else {
        ed.hint = Some("这一项没有名单".into());
        return;
    };
    let added = add_names(list, &text);
    if added.names.is_empty() {
        ed.hint = Some(format!("这些名字都已经在名单里：{}", names.join("、")));
        return;
    }
    ne.sel = list.len() - 1;
    ne.input.clear();
    ed.dirty = true;
    ed.hint = Some(added_text(&added));
}

/// Del：删掉选中的一项。
fn delete(ed: &mut Editor, ne: &mut NameEdit) {
    let Some(list) = Field::name_list(&mut ed.draft, ne.field) else {
        ed.hint = Some("这一项没有名单".into());
        return;
    };
    if list.is_empty() {
        ed.hint = Some("名单是空的".into());
        return;
    }
    let i = ne.sel.min(list.len() - 1);
    let removed = list.remove(i);
    ne.sel = i.min(list.len().saturating_sub(1));
    ed.dirty = true;
    ed.hint = Some(format!("已删除「{removed}」，记得「保存并应用」"));
}

/// 加入结果的提示：最多列三个，多了省略；跳过的重复数也说一声。
fn added_text(a: &Added) -> String {
    let shown = a.names.iter().take(3).cloned().collect::<Vec<_>>().join("、");
    let more = if a.names.len() > 3 { "…" } else { "" };
    let skipped = if a.skipped > 0 {
        format!("，跳过已有的 {} 个", a.skipped)
    } else {
        String::new()
    };
    format!(
        "已加入 {} 个：{shown}{more}{skipped}，记得「保存并应用」",
        a.names.len()
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    fn list(items: &[&str]) -> Vec<String> {
        items.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn both_bars_split_and_blanks_are_dropped() {
        assert_eq!(split_names("乌力|凯比｜克克尔"), list(&["乌力", "凯比", "克克尔"]));
        assert_eq!(split_names("  乌力 | | 凯比 ｜ "), list(&["乌力", "凯比"]));
        assert!(split_names(" | ｜ ").is_empty(), "全是空项就没有名字");
        assert!(split_names("   ").is_empty());
    }

    #[test]
    fn duplicates_inside_one_batch_are_merged() {
        assert_eq!(split_names("乌力|乌力｜ 乌力 "), list(&["乌力"]));
    }

    #[test]
    fn batch_skips_names_already_in_the_list() {
        let mut names = list(&["乌力"]);
        let added = add_names(&mut names, "乌力|凯比|凯比");
        assert_eq!(names, list(&["乌力", "凯比"]));
        assert_eq!(
            added,
            Added {
                names: list(&["凯比"]),
                skipped: 1
            }
        );
    }

    #[test]
    fn single_name_still_works_like_before() {
        let mut names = Vec::new();
        assert_eq!(add_names(&mut names, "野猪").names, list(&["野猪"]));
        assert_eq!(add_names(&mut names, "野猪").names.len(), 0);
    }

    #[test]
    fn added_text_lists_at_most_three() {
        let a = Added {
            names: list(&["a", "b", "c", "d"]),
            skipped: 2,
        };
        assert_eq!(
            added_text(&a),
            "已加入 4 个：a、b、c…，跳过已有的 2 个，记得「保存并应用」"
        );
    }
}
