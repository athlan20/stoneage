//! 命令历史（STAGE-26，STAGE-113 起按前缀翻）：本次登录以来输入过的命令，↑↓ 翻看。
//! 输入框本身见 `input.rs`。

/// 开始翻的那一刻输入行的字是前缀：只翻以它开头的命令（英文不区分大小写），翻的过程中不变；
/// ↓ 越过最新一条时还原成这几个字。输入行为空时前缀是空串，就是翻全部。
#[derive(Debug, Default)]
pub struct History {
    lines: Vec<String>,
    /// 正在浏览的位置：等于 `lines.len()` 表示不在历史里。
    pos: usize,
    /// 开始翻时输入行的字：既是前缀，也是回到最新时还原的内容。
    stash: String,
}

/// `line` 是否以 `prefix` 开头（英文不区分大小写）。
fn starts_with_ci(line: &str, prefix: &str) -> bool {
    line.to_lowercase().starts_with(&prefix.to_lowercase())
}

impl History {
    /// 最多保留多少条。
    const MAX: usize = 100;

    /// 提交了一行命令；连续重复的只留一条。
    pub fn push(&mut self, line: &str) {
        self.stash.clear();
        if self.lines.last().map(String::as_str) != Some(line) {
            if self.lines.len() >= Self::MAX {
                self.lines.remove(0);
            }
            self.lines.push(line.to_string());
        }
        self.pos = self.lines.len();
    }

    /// ↑：往前找一条以输入行的字开头的命令（刚开始翻时 `current` 就是前缀）；找不到返回 None（输入行不动）。
    pub fn prev(&mut self, current: &str) -> Option<String> {
        if self.pos == self.lines.len() {
            self.stash = current.to_string();
        }
        let found = (0..self.pos)
            .rev()
            .find(|&i| starts_with_ci(&self.lines[i], &self.stash))?;
        self.pos = found;
        Some(self.lines[found].clone())
    }

    /// ↓：往后找一条以前缀开头的命令；越过最新一条时还原输入到一半的内容（可能为空串）。
    pub fn next(&mut self) -> Option<String> {
        if self.pos >= self.lines.len() {
            return None;
        }
        match (self.pos + 1..self.lines.len()).find(|&i| starts_with_ci(&self.lines[i], &self.stash)) {
            Some(found) => {
                self.pos = found;
                Some(self.lines[found].clone())
            }
            None => {
                self.pos = self.lines.len();
                Some(self.stash.clone())
            }
        }
    }

    /// 结束浏览（清空输入行、提交之后）。
    pub fn reset(&mut self) {
        self.pos = self.lines.len();
        self.stash.clear();
    }
}

#[cfg(test)]
mod tests {
    use super::History;

    #[test]
    fn history_browses_newest_first() {
        let mut h = History::default();
        assert!(h.prev("").is_none(), "没有历史时 ↑ 不动");
        h.push("help");
        h.push("warp 1");
        h.push("warp 1"); // 连续重复只留一条
        assert_eq!(h.prev("").as_deref(), Some("warp 1"));
        assert_eq!(h.prev("").as_deref(), Some("help"));
        assert!(h.prev("").is_none(), "已在最早一条");
        assert_eq!(h.next().as_deref(), Some("warp 1"));
        // 回到最新时还原输入到一半的内容；没有存过就是空串（输入行清空）。
        assert_eq!(h.next().as_deref(), Some(""));
        // 输入到一半去翻历史，回来时还在；只翻以打的字开头的（这里是 wa）。
        assert_eq!(h.prev("wa").as_deref(), Some("warp 1"));
        assert_eq!(h.next().as_deref(), Some("wa"));
        // 提交之后浏览位置回到最新。
        h.push("take");
        assert_eq!(h.prev("").as_deref(), Some("take"));
    }

    #[test]
    fn history_browses_by_the_prefix_typed_when_browsing_began() {
        let mut h = History::default();
        for line in ["help", "warp 1", "Warp 2", "item use 7", "warp 3"] {
            h.push(line);
        }
        assert_eq!(h.prev("WA").as_deref(), Some("warp 3"), "不区分大小写");
        assert_eq!(h.prev("WA").as_deref(), Some("Warp 2"));
        assert_eq!(h.prev("WA").as_deref(), Some("warp 1"));
        assert!(h.prev("WA").is_none(), "前面没有以 wa 开头的了，输入行不动");
        assert_eq!(h.next().as_deref(), Some("Warp 2"));
        // 越过最新一条时还原成开始翻时打的字。
        assert_eq!(h.next().as_deref(), Some("warp 3"));
        assert_eq!(h.next().as_deref(), Some("WA"));
        assert!(h.next().is_none(), "已经在最新，↓ 不动");
    }

    #[test]
    fn empty_prefix_browses_everything() {
        let mut h = History::default();
        h.push("help");
        h.push("item use 7");
        assert_eq!(h.prev("").as_deref(), Some("item use 7"));
        assert_eq!(h.prev("").as_deref(), Some("help"));
    }

    #[test]
    fn history_keeps_at_most_100_lines() {
        let mut h = History::default();
        for i in 0..105 {
            h.push(&format!("cmd{i}"));
        }
        assert_eq!(h.prev("").as_deref(), Some("cmd104"));
        assert_eq!(h.prev("").as_deref(), Some("cmd103"));
    }

    #[test]
    fn history_reset_stops_browsing() {
        let mut h = History::default();
        h.push("a");
        assert_eq!(h.prev("").as_deref(), Some("a"));
        h.reset();
        assert!(h.next().is_none(), "复位后 ↓ 不动");
        assert_eq!(h.prev("").as_deref(), Some("a"), "复位后 ↑ 从最新一条开始");
    }
}
