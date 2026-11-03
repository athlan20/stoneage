//! 抓宠丢弃记录页的状态与按键（STAGE-264 第 5 条）：从设置页“捕捉”栏末尾进入，Esc 返回。
//!
//! 列表第一行是「清空记录」，之后是记录（最新在上）。清空要确认，默认选「取消」。
//! 界面在 `ui/capture_log_page.rs`；这里只管数据和按键。

use std::path::PathBuf;

use crossterm::event::{KeyCode, KeyEvent};

use super::capture_log::{self, CaptureRecord};
use super::store::Store;

#[derive(Debug, Clone)]
pub struct LogView {
    /// 记录文件与角色；没有方案目录时为 `None`，只能看到空列表。
    pub path: Option<PathBuf>,
    pub character: String,
    /// 按时间从旧到新，界面倒着显示。
    pub records: Vec<CaptureRecord>,
    /// 光标：0 是「清空记录」，1.. 是记录（1 为最新）。
    pub sel: usize,
    /// 清空确认框是否打开，以及框里的光标：0 是「取消」（默认），1 是「清空」。
    pub ask: bool,
    pub ask_choice: usize,
    /// 上一步的结果或说明，显示在底部。
    pub hint: Option<String>,
}

/// 按键之后页面是否还开着。
pub type StillOpen = bool;

impl LogView {
    pub fn open(store: Option<&Store>) -> Self {
        let (path, character) = match store {
            Some(s) => (Some(capture_log::path(s)), s.character.clone()),
            None => (None, String::new()),
        };
        let records = path
            .as_deref()
            .map(|p| capture_log::load(p, &character))
            .unwrap_or_default();
        Self {
            path,
            character,
            records,
            sel: 0,
            ask: false,
            ask_choice: 0,
            hint: None,
        }
    }

    /// 行数：清空行加记录条数。
    pub fn rows(&self) -> usize {
        1 + self.records.len()
    }

    /// 第 `row` 行（0 起，最新在上）对应的记录；清空行或越界时为 `None`。
    pub fn record_at(&self, row: usize) -> Option<&CaptureRecord> {
        let n = self.records.len();
        (row >= 1 && row <= n).then(|| &self.records[n - row])
    }

    fn clear(&mut self) {
        let Some(path) = self.path.clone() else {
            self.hint = Some("没有方案目录，记录无处可清".into());
            return;
        };
        match capture_log::clear(&path, &self.character) {
            Ok(()) => {
                let n = self.records.len();
                self.records.clear();
                self.sel = 0;
                self.hint = Some(format!("已清空 {n} 条记录"));
            }
            Err(e) => self.hint = Some(format!("清空失败：{e}")),
        }
    }
}

/// 记录页的按键。返回 `false` 表示该退回设置页。
pub fn key(log: &mut LogView, key: KeyEvent) -> StillOpen {
    if log.ask {
        match key.code {
            KeyCode::Left | KeyCode::Right | KeyCode::Tab | KeyCode::BackTab => {
                log.ask_choice = (log.ask_choice + 1) % 2;
            }
            KeyCode::Enter => {
                let clear = log.ask_choice == 1;
                log.ask = false;
                log.ask_choice = 0;
                if clear {
                    log.clear();
                } else {
                    log.hint = Some("已取消，记录没有动".into());
                }
            }
            KeyCode::Esc => {
                log.ask = false;
                log.ask_choice = 0;
                log.hint = Some("已取消，记录没有动".into());
            }
            _ => {}
        }
        return true;
    }
    match key.code {
        KeyCode::Esc => return false,
        KeyCode::Up => log.sel = log.sel.saturating_sub(1),
        KeyCode::Down => log.sel = (log.sel + 1).min(log.rows() - 1),
        KeyCode::Home => log.sel = 0,
        KeyCode::End => log.sel = log.rows() - 1,
        KeyCode::Enter => {
            if log.sel == 0 {
                if log.records.is_empty() {
                    log.hint = Some("没有记录可清空".into());
                } else {
                    log.ask = true;
                    log.ask_choice = 0;
                    log.hint = None;
                }
            } else {
                log.hint = Some("记录只看不改；要清空请选第一行「清空记录」".into());
            }
        }
        _ => {}
    }
    true
}

#[cfg(test)]
mod tests {
    use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};

    use super::*;

    fn press(log: &mut LogView, code: KeyCode) -> bool {
        key(log, KeyEvent::new(code, KeyModifiers::NONE))
    }

    fn view_with(n: usize) -> LogView {
        let records = (0..n)
            .map(|i| CaptureRecord {
                at: "2026-10-21 00:00:00".into(),
                name: format!("宠{i}"),
                level: 1,
                quality: vec![],
                reason: String::new(),
            })
            .collect();
        LogView {
            path: None,
            character: "勇者".into(),
            records,
            sel: 0,
            ask: false,
            ask_choice: 0,
            hint: None,
        }
    }

    #[test]
    fn newest_record_is_the_first_after_the_clear_row() {
        let v = view_with(3);
        assert_eq!(v.rows(), 4);
        assert_eq!(v.record_at(0).map(|r| r.name.as_str()), None);
        assert_eq!(v.record_at(1).map(|r| r.name.as_str()), Some("宠2"));
        assert_eq!(v.record_at(3).map(|r| r.name.as_str()), Some("宠0"));
    }

    #[test]
    fn clear_asks_and_the_default_is_cancel() {
        let mut v = view_with(2);
        assert!(press(&mut v, KeyCode::Enter), "Enter 后页面还开着");
        assert!(v.ask);
        assert_eq!(v.ask_choice, 0, "默认停在「取消」");
        press(&mut v, KeyCode::Enter);
        assert!(!v.ask);
        assert_eq!(v.records.len(), 2, "默认取消，记录不动");
    }

    #[test]
    fn esc_cancels_the_ask_before_leaving_the_page() {
        let mut v = view_with(2);
        press(&mut v, KeyCode::Enter);
        assert!(press(&mut v, KeyCode::Esc), "第一个 Esc 只关确认框");
        assert!(!v.ask);
        assert!(!press(&mut v, KeyCode::Esc), "第二个 Esc 返回设置页");
    }

    #[test]
    fn confirming_clear_empties_the_list_and_the_file() {
        let dir = std::env::temp_dir().join(format!("stage264-view-{}", std::process::id()));
        let path = dir.join("capture-log.json");
        let _ = std::fs::remove_dir_all(&dir);
        let mut v = view_with(2);
        v.path = Some(path.clone());
        press(&mut v, KeyCode::Enter);
        press(&mut v, KeyCode::Right);
        press(&mut v, KeyCode::Enter);
        assert!(v.records.is_empty());
        assert_eq!(v.hint.as_deref(), Some("已清空 2 条记录"));
        assert!(capture_log::load(&path, "勇者").is_empty());
    }

    #[test]
    fn without_a_store_there_is_nothing_to_clear() {
        let mut v = view_with(2);
        press(&mut v, KeyCode::Enter);
        press(&mut v, KeyCode::Right);
        press(&mut v, KeyCode::Enter);
        assert_eq!(v.records.len(), 2);
        assert_eq!(v.hint.as_deref(), Some("没有方案目录，记录无处可清"));
    }

    #[test]
    fn cursor_stays_inside_the_list() {
        let mut v = view_with(2);
        press(&mut v, KeyCode::Up);
        assert_eq!(v.sel, 0);
        press(&mut v, KeyCode::End);
        assert_eq!(v.sel, 2);
        press(&mut v, KeyCode::Down);
        assert_eq!(v.sel, 2);
    }
}
