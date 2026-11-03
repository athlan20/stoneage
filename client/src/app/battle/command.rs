//! 战斗指令：动作、出招的人，以及文字命令的解析（`docs/battle-ui.md`「命令表」，`docs/protocol.md` 10.5）。

use serde_json::{Value, json};

/// 谁出招：人物或宠物（协议的 `actor`）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Actor {
    Player,
    Pet,
}

impl Actor {
    pub fn wire(self) -> &'static str {
        match self {
            Self::Player => "player",
            Self::Pet => "pet",
        }
    }

    pub fn title(self) -> &'static str {
        match self {
            Self::Player => "人物指令",
            Self::Pet => "宠物指令",
        }
    }
}

/// 动作（协议的 `action`）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Act {
    Attack,
    Magic,
    Item,
    Guard,
    Capture,
    Escape,
    SwapPet,
    Skill,
}

/// 指令条上的一个格子（STAGE-153）：普通出招动作，或者在战斗里开关自动挂机的管理项。
/// 管理项不是战斗动作，绝不转成协议的 `Act`，也不消耗回合。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MenuSlot {
    Act(Act),
    /// 开关自动挂机：关闭时「开始自动挂机」，开启时「停止自动挂机」。
    AutoToggle,
}

impl MenuSlot {
    /// 出招动作；管理项返回 None。
    pub fn act(self) -> Option<Act> {
        match self {
            Self::Act(act) => Some(act),
            Self::AutoToggle => None,
        }
    }

    /// 指令条上显示的名字；管理项跟着挂机状态变化（`auto_on` 是本机托管是否开着）。
    pub fn label(self, auto_on: bool) -> &'static str {
        match self {
            Self::Act(act) => act.label(),
            Self::AutoToggle if auto_on => "停止自动挂机",
            Self::AutoToggle => "开始自动挂机",
        }
    }
}

/// 人物指令条的顺序：数字键 1–7 对应原来的七个动作，第 8 项是挂机管理项（STAGE-153）。
/// 网格是 4 列 2 行：第一行攻击、精灵、道具、防御，第二行捕捉、逃跑、换宠、挂机。
pub const PLAYER_MENU: [MenuSlot; 8] = [
    MenuSlot::Act(Act::Attack),
    MenuSlot::Act(Act::Magic),
    MenuSlot::Act(Act::Item),
    MenuSlot::Act(Act::Guard),
    MenuSlot::Act(Act::Capture),
    MenuSlot::Act(Act::Escape),
    MenuSlot::Act(Act::SwapPet),
    MenuSlot::AutoToggle,
];

/// 宠物指令条的顺序：1–3 是原来的三个动作，第 4 项是挂机管理项。网格是 2 列 2 行：
/// 第一行攻击、技能，第二行防御、挂机。
pub const PET_MENU: [MenuSlot; 4] = [
    MenuSlot::Act(Act::Attack),
    MenuSlot::Act(Act::Skill),
    MenuSlot::Act(Act::Guard),
    MenuSlot::AutoToggle,
];

impl Act {
    pub fn wire(self) -> &'static str {
        match self {
            Self::Attack => "attack",
            Self::Magic => "magic",
            Self::Item => "item",
            Self::Guard => "guard",
            Self::Capture => "capture",
            Self::Escape => "escape",
            Self::SwapPet => "swap_pet",
            Self::Skill => "skill",
        }
    }

    pub fn label(self) -> &'static str {
        match self {
            Self::Attack => "攻击",
            Self::Magic => "精灵",
            Self::Item => "道具",
            Self::Guard => "防御",
            Self::Capture => "捕捉",
            Self::Escape => "逃跑",
            Self::SwapPet => "换宠",
            Self::Skill => "技能",
        }
    }

    /// 这个动作能不能由这个角色下（协议 10.5：不属于该 `actor` 的 `action` 回复 BAD_REQUEST）。
    pub fn allowed_for(self, actor: Actor) -> bool {
        match actor {
            Actor::Player => !matches!(self, Self::Skill),
            Actor::Pet => matches!(self, Self::Attack | Self::Skill | Self::Guard),
        }
    }
}

/// 文字命令解析出的一条指令：动作和参数，目标还是“我方/敌方 + 界面编号”。
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Typed {
    pub act: Act,
    /// `(是否我方, 界面编号 1–10)`。没写 `m`/`e` 时是 `None`，由这一项的目标类型决定是哪一方（运行时才知道）。
    pub target: Option<(Option<bool>, u8)>,
    /// 精灵编号（0–4）、道具槽位（5–19）、技能栏位（1–7）、换宠栏位（0–19，收回为 -1）。
    pub arg: Option<i64>,
}

/// 解析战斗里的文字命令（`attack e3`、`a e3`、`defend`、`g`…）。不是战斗命令返回 None；
/// 是战斗命令但参数不对时返回 Err，原因直接显示给玩家。
pub fn parse(line: &str) -> Option<Result<Typed, String>> {
    let (cmd, rest) = line.split_once(char::is_whitespace).map_or((line, ""), |(a, b)| (a, b.trim()));
    let act = match cmd {
        "attack" | "a" => Act::Attack,
        "defend" | "g" => Act::Guard,
        "escape" | "e" => Act::Escape,
        "capture" | "c" => Act::Capture,
        "magic" | "m" => Act::Magic,
        "item" | "i" => Act::Item,
        "swap" | "p" => Act::SwapPet,
        "skill" | "s" => Act::Skill,
        _ => return None,
    };
    Some(parse_args(act, rest))
}

fn parse_args(act: Act, rest: &str) -> Result<Typed, String> {
    let words: Vec<&str> = rest.split_whitespace().collect();
    let first = words.first().copied();
    let second = words.get(1).copied();
    match act {
        Act::Attack | Act::Capture => {
            let t = first.ok_or_else(|| format!("{} 要写目标，例如 {} e3", act.label(), short(act)))?;
            Ok(Typed { act, target: Some(parse_target(t)?), arg: None })
        }
        Act::Guard | Act::Escape => Ok(Typed { act, target: None, arg: None }),
        Act::Magic => {
            let arg = number(first, 0..=4, "精灵编号要在 0–4 之间")?;
            let target = second.map(parse_target).transpose()?;
            Ok(Typed { act, target, arg: Some(arg) })
        }
        Act::Item => {
            let arg = number(first, 5..=19, "道具槽位要在 5–19 之间（背包页的编号）")?;
            let target = second.map(parse_target).transpose()?;
            Ok(Typed { act, target, arg: Some(arg) })
        }
        Act::Skill => {
            let arg = number(first, 1..=7, "技能编号要在 1–7 之间")?;
            let target = second.map(parse_target).transpose()?;
            Ok(Typed { act, target, arg: Some(arg) })
        }
        Act::SwapPet => match first {
            Some("-") => Ok(Typed { act, target: None, arg: Some(-1) }),
            _ => Ok(Typed {
                act, target: None, arg: Some(number(first, 0..=i64::from(crate::app::pets::MAX_PET_SLOT), "宠物栏位要在 0–19 之间，- 表示收回")?)
            }),
        },
    }
}

/// 解析一个参数里的数字；缺少或超出范围时返回 `msg`。
fn number(word: Option<&str>, range: std::ops::RangeInclusive<i64>, msg: &str) -> Result<i64, String> {
    word.and_then(|w| w.parse::<i64>().ok()).filter(|n| range.contains(n)).ok_or_else(|| msg.to_string())
}

/// 目标写法：`e3` 是敌方 3 号，`m2` 是我方 2 号；只写数字时哪一方由目标类型决定（返回的 side 为 `None`）。
pub(super) fn parse_target(text: &str) -> Result<(Option<bool>, u8), String> {
    let lower = text.to_ascii_lowercase();
    let (mine, digits) = match lower.chars().next() {
        Some('m') => (Some(true), &lower[1..]),
        Some('e') => (Some(false), &lower[1..]),
        _ => (None, lower.as_str()),
    };
    match digits.parse::<u8>() {
        Ok(n) if (1..=10).contains(&n) => Ok((mine, n)),
        _ => Err(format!("目标写法是 e3（敌方 3 号）或 m2（我方 2 号），编号 1–10：{text}")),
    }
}

/// 命令的示例写法，用在错误提示里。
fn short(act: Act) -> &'static str {
    match act {
        Act::Capture => "c e3",
        _ => "a e3",
    }
}

/// 发给服务端的 `battle_command` 数据（10.5）。`target` 和 `arg` 只在对应的动作里带上。
pub fn command_data(turn: u32, actor: Actor, act: Act, target: Option<u8>, arg: Option<i64>) -> Value {
    let mut v = json!({"turn": turn, "actor": actor.wire(), "action": act.wire()});
    let field = match act {
        Act::Magic => Some("magic"),
        Act::Item => Some("slot"),
        Act::Skill => Some("skill"),
        Act::SwapPet => Some("pet_slot"),
        _ => None,
    };
    if let (Some(name), Some(n)) = (field, arg) {
        v[name] = json!(n);
    }
    if let Some(t) = target {
        v["target"] = json!(t);
    }
    v
}

#[cfg(test)]
mod tests {
    use super::*;

    fn ok(line: &str) -> Typed {
        parse(line).expect("应当是战斗命令").expect("参数应当正确")
    }

    #[test]
    fn attack_takes_enemy_target_by_letter() {
        assert_eq!(ok("attack e3"), Typed { act: Act::Attack, target: Some((Some(false), 3)), arg: None });
        assert_eq!(ok("a E10").target, Some((Some(false), 10)));
        // 只写数字时哪一方由目标类型决定（攻击是敌方），解析阶段先不定。
        assert_eq!(ok("a 2").target, Some((None, 2)));
    }

    #[test]
    fn short_and_long_names_agree() {
        assert_eq!(ok("g"), ok("defend"));
        assert_eq!(ok("e"), ok("escape"));
        assert_eq!(ok("c m1"), Typed { act: Act::Capture, target: Some((Some(true), 1)), arg: None });
    }

    #[test]
    fn item_and_magic_take_optional_target() {
        assert_eq!(ok("item 5"), Typed { act: Act::Item, target: None, arg: Some(5) });
        assert_eq!(ok("i 7 m1"), Typed { act: Act::Item, target: Some((Some(true), 1)), arg: Some(7) });
        assert_eq!(ok("m 0 e2"), Typed { act: Act::Magic, target: Some((Some(false), 2)), arg: Some(0) });
        assert_eq!(ok("m 0 2").target, Some((None, 2)), "没写 m/e 时哪一方由目标类型决定");
    }

    #[test]
    fn swap_accepts_dash_for_recall() {
        assert_eq!(ok("swap -").arg, Some(-1));
        assert_eq!(ok("p 3").arg, Some(3));
    }

    #[test]
    fn bad_arguments_are_reported() {
        assert!(parse("a").unwrap().is_err());
        assert!(parse("a e11").unwrap().is_err());
        assert!(parse("item 4").unwrap().is_err());
        assert!(parse("skill 8").unwrap().is_err());
        assert!(parse("swap 20").unwrap().is_err());
    }

    #[test]
    fn other_commands_are_not_battle_commands() {
        assert!(parse("status").is_none());
        assert!(parse("warp 1").is_none());
    }

    #[test]
    fn command_json_carries_only_the_needed_fields() {
        let v = command_data(2, Actor::Player, Act::Attack, Some(15), None);
        assert_eq!(v, json!({"turn": 2, "actor": "player", "action": "attack", "target": 15}));
        let v = command_data(1, Actor::Pet, Act::Guard, None, None);
        assert_eq!(v, json!({"turn": 1, "actor": "pet", "action": "guard"}));
        let v = command_data(1, Actor::Player, Act::SwapPet, None, Some(1));
        assert_eq!(v, json!({"turn": 1, "actor": "player", "action": "swap_pet", "pet_slot": 1}));
        let v = command_data(1, Actor::Player, Act::Item, Some(0), Some(5));
        assert_eq!(v, json!({"turn": 1, "actor": "player", "action": "item", "slot": 5, "target": 0}));
    }

    #[test]
    fn actions_match_the_actor() {
        assert!(!Act::Skill.allowed_for(Actor::Player));
        assert!(!Act::Capture.allowed_for(Actor::Pet));
        assert!(Act::Guard.allowed_for(Actor::Pet));
    }
}
