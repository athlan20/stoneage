use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use unicode_width::UnicodeWidthChar;

/// 单行输入框。光标按字符计，显示宽度按终端列宽计（中文占 2 列）。
#[derive(Clone, Default)]
pub struct TextInput {
    text: String,
    cursor: usize,
    max_chars: usize,
}

/// 密码框的内容不能出现在调试输出里。
impl std::fmt::Debug for TextInput {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "TextInput({} 个字符)", self.text.chars().count())
    }
}

impl TextInput {
    pub fn new(max_chars: usize) -> Self {
        Self { text: String::new(), cursor: 0, max_chars }
    }

    pub fn with_text(max_chars: usize, text: &str) -> Self {
        let mut t = Self::new(max_chars);
        t.set(text);
        t
    }

    pub fn text(&self) -> &str {
        &self.text
    }

    pub fn is_empty(&self) -> bool {
        self.text.is_empty()
    }

    pub fn set(&mut self, text: &str) {
        self.text = text.chars().take(self.max_chars).collect();
        self.cursor = self.text.chars().count();
    }

    pub fn clear(&mut self) {
        self.text.clear();
        self.cursor = 0;
    }

    /// 把 `from`（字符下标）之后的内容换成 `text`，光标移到末尾（Tab 补全用）。
    pub fn replace_tail(&mut self, from: usize, text: &str) {
        let mut out: String = self.text.chars().take(from).collect();
        for c in text.chars() {
            if out.chars().count() >= self.max_chars {
                break;
            }
            out.push(c);
        }
        self.text = out;
        self.cursor = self.text.chars().count();
    }

    /// 超过 `width` 列时的横向滚动窗口：返回 (片段, 光标在片段内的列偏移)，光标始终在窗口里。
    /// 窗口先向光标右边填，再向左边补：光标在末尾输入时能看到前面的全部，光标在开头时能看到后面的内容。
    pub fn view(&self, width: usize) -> (String, u16) {
        let chars: Vec<char> = self.text.chars().collect();
        let widths: Vec<usize> = chars.iter().map(|c| c.width().unwrap_or(1)).collect();
        let cursor = self.cursor.min(chars.len());
        if width == 0 || widths.iter().sum::<usize>() <= width {
            let col: usize = widths[..cursor].iter().sum();
            return (self.text.clone(), col as u16);
        }
        let mut end = cursor;
        let mut used = 0usize;
        while end < chars.len() && used + widths[end] <= width {
            used += widths[end];
            end += 1;
        }
        let mut start = cursor;
        while start > 0 && used + widths[start - 1] <= width {
            start -= 1;
            used += widths[start];
        }
        let text: String = chars[start..end].iter().collect();
        let col: usize = widths[start..cursor].iter().sum();
        (text, col as u16)
    }

    /// 要画到屏幕上的字符串；`mask` 不为空时每个字符都显示成它。
    pub fn display(&self, mask: Option<char>) -> String {
        match mask {
            Some(m) => self.text.chars().map(|_| m).collect(),
            None => self.text.clone(),
        }
    }

    /// 光标相对输入框起点的列偏移。
    pub fn cursor_col(&self, mask: Option<char>) -> u16 {
        let w: usize = self
            .text
            .chars()
            .take(self.cursor)
            .map(|c| mask.unwrap_or(c).width().unwrap_or(1))
            .sum();
        w as u16
    }

    fn byte_index(&self, char_idx: usize) -> usize {
        self.text.char_indices().nth(char_idx).map_or(self.text.len(), |(i, _)| i)
    }

    /// 处理编辑按键，返回是否已被输入框消耗。
    pub fn handle_key(&mut self, key: &KeyEvent) -> bool {
        let plain = !key.modifiers.intersects(KeyModifiers::CONTROL | KeyModifiers::ALT);
        match key.code {
            KeyCode::Char(c) if plain && !c.is_control() => {
                if self.text.chars().count() < self.max_chars {
                    let at = self.byte_index(self.cursor);
                    self.text.insert(at, c);
                    self.cursor += 1;
                }
                true
            }
            KeyCode::Backspace => {
                if self.cursor > 0 {
                    let at = self.byte_index(self.cursor - 1);
                    self.text.remove(at);
                    self.cursor -= 1;
                }
                true
            }
            KeyCode::Delete => {
                if self.cursor < self.text.chars().count() {
                    let at = self.byte_index(self.cursor);
                    self.text.remove(at);
                }
                true
            }
            KeyCode::Left => {
                self.cursor = self.cursor.saturating_sub(1);
                true
            }
            KeyCode::Right => {
                self.cursor = (self.cursor + 1).min(self.text.chars().count());
                true
            }
            KeyCode::Home => {
                self.cursor = 0;
                true
            }
            KeyCode::End => {
                self.cursor = self.text.chars().count();
                true
            }
            _ => false,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crossterm::event::KeyEvent;

    fn key(c: KeyCode) -> KeyEvent {
        KeyEvent::new(c, KeyModifiers::NONE)
    }

    #[test]
    fn edit_chinese_and_width() {
        let mut t = TextInput::new(8);
        for c in "勇者ab".chars() {
            t.handle_key(&key(KeyCode::Char(c)));
        }
        assert_eq!(t.text(), "勇者ab");
        assert_eq!(t.cursor_col(None), 6);
        t.handle_key(&key(KeyCode::Left));
        t.handle_key(&key(KeyCode::Backspace));
        assert_eq!(t.text(), "勇者b");
        assert_eq!(t.cursor_col(Some('•')), 2);
    }

    #[test]
    fn respects_max_chars() {
        let mut t = TextInput::new(2);
        for c in "abc".chars() {
            t.handle_key(&key(KeyCode::Char(c)));
        }
        assert_eq!(t.text(), "ab");
    }

    #[test]
    fn debug_hides_content() {
        let t = TextInput::with_text(10, "secret1");
        assert!(!format!("{t:?}").contains("secret1"));
    }

    #[test]
    fn view_shows_everything_when_it_fits() {
        let mut t = TextInput::new(220);
        for c in "勇者ab".chars() {
            t.handle_key(&key(KeyCode::Char(c)));
        }
        assert_eq!(t.view(10), ("勇者ab".to_string(), 6));
    }

    #[test]
    fn view_scrolls_and_keeps_cursor_visible() {
        let mut long = TextInput::new(220);
        for c in "0123456789".chars() {
            long.handle_key(&key(KeyCode::Char(c)));
        }
        // 光标在末尾：显示末尾四个字符，光标贴右边。
        assert_eq!(long.view(4), ("6789".to_string(), 4));
        // 光标移到开头：从开头显示，能看到后面的内容。
        long.handle_key(&key(KeyCode::Home));
        assert_eq!(long.view(4), ("0123".to_string(), 0));
        // 中文按显示宽度算。
        let mut t = TextInput::new(220);
        for c in "勇者".chars() {
            t.handle_key(&key(KeyCode::Char(c)));
        }
        assert_eq!(t.view(3), ("者".to_string(), 2));
    }

    #[test]
    fn replace_tail_swaps_the_last_word() {
        let mut t = TextInput::new(20);
        t.set("warp 萨");
        t.replace_tail(5, "萨姆吉尔村 ");
        assert_eq!(t.text(), "warp 萨姆吉尔村 ");
        assert_eq!(t.cursor_col(None), 16, "光标列按显示宽度算");
        // 超长截断到 max_chars。
        let mut small = TextInput::new(6);
        small.set("ab");
        small.replace_tail(2, "cdefg");
        assert_eq!(small.text(), "abcdef");
    }
}
