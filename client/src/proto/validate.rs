//! 本地格式检查，规则与 `docs/protocol.md` 的 `register`、`create_character` 一致。
//! 客户端先检查再发送，最终以服务端结果为准。

use serde_json::{Value, json};

pub const ACCOUNT_MIN: usize = 3;
pub const ACCOUNT_MAX: usize = 15;
pub const PASSWORD_MIN: usize = 6;
pub const PASSWORD_MAX: usize = 64;
pub const NAME_MAX_CHARS: usize = 16;
pub const STAT_POINTS: u8 = 20;
pub const ELEMENT_POINTS: u8 = 10;

pub fn check_account(account: &str) -> Result<(), String> {
    let n = account.chars().count();
    if !(ACCOUNT_MIN..=ACCOUNT_MAX).contains(&n) {
        return Err(format!("账号需要 {ACCOUNT_MIN}–{ACCOUNT_MAX} 个字符"));
    }
    let lower = account.to_lowercase();
    if !lower.starts_with(|c: char| c.is_ascii_lowercase()) {
        return Err("账号必须以字母开头".into());
    }
    if !lower.chars().all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '_') {
        return Err("账号只能包含字母、数字和下划线".into());
    }
    Ok(())
}

pub fn check_password(password: &str) -> Result<(), String> {
    let n = password.chars().count();
    if !(PASSWORD_MIN..=PASSWORD_MAX).contains(&n) {
        return Err(format!("密码需要 {PASSWORD_MIN}–{PASSWORD_MAX} 个字符"));
    }
    if password.trim() != password {
        return Err("密码不能有首尾空格".into());
    }
    Ok(())
}

/// 角色名的本地检查：1–16 个字符（按 Unicode 字符计）、无空白和控制字符。敏感词和唯一性由服务端判断。
pub fn check_character_name(name: &str) -> Result<(), String> {
    let name = name.trim();
    if name.is_empty() {
        return Err("请输入角色名".into());
    }
    if name.chars().any(|c| c.is_whitespace() || c.is_control()) {
        return Err("角色名不能包含空白或控制字符".into());
    }
    if name.chars().count() > NAME_MAX_CHARS {
        return Err(format!("角色名太长（最多 {NAME_MAX_CHARS} 个字）"));
    }
    Ok(())
}

/// 能力点：体力、腕力、耐久力、速度。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Stats {
    pub vital: u8,
    pub strength: u8,
    pub toughness: u8,
    pub dexterity: u8,
}

impl Stats {
    pub fn total(&self) -> u8 {
        self.vital + self.strength + self.toughness + self.dexterity
    }

    pub fn remaining(&self) -> u8 {
        STAT_POINTS.saturating_sub(self.total())
    }

    pub fn get_mut(&mut self, i: usize) -> &mut u8 {
        match i {
            0 => &mut self.vital,
            1 => &mut self.strength,
            2 => &mut self.toughness,
            _ => &mut self.dexterity,
        }
    }

    pub fn to_json(&self) -> Value {
        json!({"vital": self.vital, "strength": self.strength, "toughness": self.toughness, "dexterity": self.dexterity})
    }
}

/// 属性点：地、水、火、风。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Elements {
    pub earth: u8,
    pub water: u8,
    pub fire: u8,
    pub wind: u8,
}

impl Elements {
    const EARTH: usize = 0;
    const WATER: usize = 1;
    const FIRE: usize = 2;
    const WIND: usize = 3;

    pub fn as_array(&self) -> [u8; 4] {
        [self.earth, self.water, self.fire, self.wind]
    }

    fn from_array(a: [u8; 4]) -> Self {
        Self { earth: a[0], water: a[1], fire: a[2], wind: a[3] }
    }

    pub fn total(&self) -> u8 {
        self.as_array().iter().sum()
    }

    pub fn remaining(&self) -> u8 {
        ELEMENT_POINTS.saturating_sub(self.total())
    }

    /// 与 `i` 互斥的属性序号：地↔火、水↔风。
    pub fn opposite(i: usize) -> usize {
        match i {
            Self::EARTH => Self::FIRE,
            Self::FIRE => Self::EARTH,
            Self::WATER => Self::WIND,
            _ => Self::WATER,
        }
    }

    /// 该项当前是否因互斥而不能加点。
    pub fn blocked(&self, i: usize) -> bool {
        self.as_array()[Self::opposite(i)] > 0
    }

    /// 加 1 点。没有剩余点数时，从第一个非零的其他属性挪 1 点过来（原版 `LOGIN.CPP` 的做法）。
    pub fn increase(&mut self, i: usize) -> bool {
        let mut a = self.as_array();
        if self.blocked(i) || a[i] >= ELEMENT_POINTS {
            return false;
        }
        if self.remaining() == 0 {
            let Some(from) = (0..4).find(|&k| k != i && a[k] > 0) else {
                return false;
            };
            a[from] -= 1;
        }
        a[i] += 1;
        *self = Self::from_array(a);
        true
    }

    pub fn decrease(&mut self, i: usize) -> bool {
        let mut a = self.as_array();
        if a[i] == 0 {
            return false;
        }
        a[i] -= 1;
        *self = Self::from_array(a);
        true
    }

    pub fn to_json(&self) -> Value {
        json!({"earth": self.earth, "water": self.water, "fire": self.fire, "wind": self.wind})
    }

    /// 协议要求：总和正好 10，最多 2 项非零，且互斥项不同时有点。
    pub fn is_valid(&self) -> bool {
        let a = self.as_array();
        self.total() == ELEMENT_POINTS
            && a.iter().filter(|&&p| p > 0).count() <= 2
            && !(self.earth > 0 && self.fire > 0)
            && !(self.water > 0 && self.wind > 0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn account_rules() {
        assert!(check_account("alice").is_ok());
        assert!(check_account("Alice_1").is_ok());
        assert!(check_account("ab").is_err());
        assert!(check_account("1abc").is_err());
        assert!(check_account("a-bc").is_err());
        assert!(check_account("abcdefghijklmnop").is_err());
    }

    #[test]
    fn password_rules() {
        assert!(check_password("secret1").is_ok());
        assert!(check_password("short").is_err());
        assert!(check_password(" secret1").is_err());
        assert!(check_password(&"x".repeat(65)).is_err());
    }

    #[test]
    fn name_rules() {
        assert!(check_character_name("勇者小明").is_ok());
        assert!(check_character_name("  ").is_err());
        assert!(check_character_name("a b").is_err());
        assert!(check_character_name(&"汉".repeat(16)).is_ok());
        assert!(check_character_name(&"汉".repeat(17)).is_err());
        assert!(check_character_name(&"a".repeat(16)).is_ok());
        assert!(check_character_name(&"a".repeat(17)).is_err());
        assert!(check_character_name("石器😀").is_ok());
    }

    #[test]
    fn elements_move_points_when_none_left() {
        let mut e = Elements { earth: 5, water: 5, fire: 0, wind: 0 };
        assert!(e.increase(0));
        assert_eq!(e.as_array(), [6, 4, 0, 0]);
        // 地有点，火被互斥挡住
        assert!(!e.increase(2));
        assert!(e.is_valid());
    }

    #[test]
    fn elements_exclusive() {
        let mut e = Elements { earth: 0, water: 0, fire: 10, wind: 0 };
        assert!(!e.increase(0));
        assert!(e.increase(1));
        assert_eq!(e.as_array(), [0, 1, 9, 0]);
        assert!(!e.increase(3));
    }
}
