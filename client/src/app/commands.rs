//! 命令表（STAGE-26，`docs/ui.md`「命令」）：每条命令在一处登记名字、别名、用法、一句话说明、
//! 是否“操作”类。分发（`world.rs` 的 `run_command`）、Tab 补全、`help`、观察模式拦截都读这张表，
//! 新增命令只需在 `COMMANDS` 里加一行。

use super::App;
use crate::proto::{Place, WarpMap};

/// 观察模式（agent 操控）下命令要不要在客户端拦下。
#[derive(Clone, Copy)]
pub enum Operation {
    /// 任何时候都能用。
    No,
    /// 整条命令都是“操作”类。
    Yes,
    /// 只有列出的子命令是操作类（`pet` 的管理子命令，`pet status` 不是）。
    Subs(&'static [&'static str]),
}

/// 第一个参数怎么补全。
#[derive(Clone, Copy)]
pub enum Arg {
    None,
    /// 从这些固定候选里选（子命令、能力名等）。
    Choice(&'static [&'static str]),
    /// 地点名（`warp`，候选来自地点表）。
    Place,
    /// 令牌编号（`revoke`，编号是「系统」页令牌列表的序号）。
    TokenNo,
    /// 命令名（`help`）。
    Command,
}

/// 命令表里的一条登记。
pub struct Command {
    pub name: &'static str,
    pub aliases: &'static [&'static str],
    /// 用法（不含命令名和“用法：”前缀），如“编号|地点名”。
    pub usage: &'static str,
    /// 一句话说明，`help` 列表用。
    pub summary: &'static str,
    pub operation: Operation,
    pub arg: Arg,
}

/// Tab 补全时消息栏里最多列出的候选数。
pub const CANDIDATE_LIST_LIMIT: usize = 20;

/// `page` 的参数候选：编号和页名（与 `world.rs` 的 `MENU_TITLES` 一致）。
const PAGE_ARGS: &[&str] =
    &["1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "地图", "角色", "宠物", "背包", "任务", "社交", "消息", "自动战斗", "系统", "百科"];

/// 全部命令。顺序就是 `help` 列表和补全候选的顺序。
pub const COMMANDS: &[Command] = &[
    Command { name: "help", aliases: &[], usage: "[命令名]", summary: "列出全部命令，help 命令名 看详细用法", operation: Operation::No, arg: Arg::Command },
    Command {
        name: "page",
        aliases: &["页"],
        usage: "1–9|页名",
        summary: "切换菜单页，弹窗开着时也能用，不关弹窗（STAGE-103）",
        operation: Operation::No,
        arg: Arg::Choice(PAGE_ARGS),
    },
    Command { name: "status", aliases: &[], usage: "", summary: "查看角色状态", operation: Operation::No, arg: Arg::None },
    Command { name: "leave", aliases: &["logout"], usage: "", summary: "回到角色选择页", operation: Operation::No, arg: Arg::None },
    Command { name: "quit", aliases: &["exit"], usage: "", summary: "退出游戏", operation: Operation::No, arg: Arg::None },
    Command { name: "turn", aliases: &[], usage: "方向(0–7，0 为北，顺时针)", summary: "原地转向", operation: Operation::Yes, arg: Arg::None },
    Command { name: "warp", aliases: &["传送"], usage: "地点编号|地点名|地图编号", summary: "免费直接传送到地点或任意地图", operation: Operation::Yes, arg: Arg::Place },
    Command { name: "talk", aliases: &["对话"], usage: "编号", summary: "和 NPC 对话（编号见「地图」页的本地图 NPC 表）", operation: Operation::Yes, arg: Arg::None },
    Command { name: "npc", aliases: &[], usage: "名字", summary: "按名字和 NPC 对话，不在本图时先问是否传送过去（STAGE-84）", operation: Operation::Yes, arg: Arg::None },
    Command { name: "choose", aliases: &[], usage: "选项编号", summary: "回应对话窗口里的选项", operation: Operation::Yes, arg: Arg::None },
    Command { name: "reply", aliases: &["回复"], usage: "文本", summary: "对对话窗口输入文字（比如买商品时报数量）", operation: Operation::Yes, arg: Arg::None },
    Command { name: "inlay", aliases: &["精工"], usage: "<目标格> <材料格…> [yes]", summary: "精工：把材料镶到武器或防具上（同 item inlay，STAGE-299）", operation: Operation::Yes, arg: Arg::None },
    Command { name: "addpoint", aliases: &[], usage: "体力|腕力|耐久|速度 [点数]", summary: "给能力加点（省略点数时加 1 点）", operation: Operation::Yes, arg: Arg::Choice(&["体力", "腕力", "耐久", "耐久力", "速度"]) },
    Command {
        name: "item",
        aliases: &[],
        usage: "use <槽位> [目标] | equip <编号> | unequip <编号> | move <从> <到> | drop <编号> [数量] yes | inlay <目标格> <材料格…> [yes]",
        summary: "使用消耗品（use，STAGE-30）、穿脱整理与精工（inlay，STAGE-299），item 单独输入看全部子命令",
        operation: Operation::Yes,
        arg: Arg::Choice(&["use", "equip", "unequip", "move", "drop", "inlay"]),
    },
    Command {
        name: "gold",
        aliases: &[],
        usage: "drop <数量> [yes]",
        summary: "丢弃石币（STAGE-328）：丢掉一部分石币，用来把持有量压到上限以下；丢弃不能恢复，要带 yes 或确认",
        operation: Operation::Yes,
        arg: Arg::Choice(&["drop"]),
    },
    Command {
        name: "pet",
        aliases: &[],
        usage: "status|battle|standby|rest <编号>，ride <编号>|off，rename <编号> [名字]，drop <编号>，skill <编号> <栏位> [背包格号…]，talk <编号>",
        summary: "查看或管理宠物（骑乘用 ride，STAGE-123），pet 单独输入看全部子命令",
        operation: Operation::Subs(&["battle", "standby", "rest", "ride", "rename", "drop", "skill", "talk"]),
        arg: Arg::Choice(&["status", "battle", "standby", "rest", "ride", "rename", "drop", "skill", "talk"]),
    },
    Command {
        name: "magic",
        aliases: &[],
        usage: "精灵栏位 [宠物编号] | use 精灵栏位 [self|pet 宠物编号]",
        summary: "战斗外使用精灵：变幻的精灵变身成自己的宠物，不带宠物编号变回自己（STAGE-203）；use 是通用写法，滋润的精灵对象写 self 或 pet（STAGE-231）",
        operation: Operation::Yes,
        arg: Arg::None,
    },
    Command {
        name: "players",
        aliases: &[],
        usage: "",
        summary: "把焦点切到场景页的附近玩家列并刷新（STAGE-273）",
        operation: Operation::No,
        arg: Arg::None,
    },
    Command {
        name: "wiki",
        aliases: &["百科"],
        usage: "[分类] | find <词> | open <编号> | next | prev | back",
        summary: "石器百科：按分类浏览、搜索、看词条详情（STAGE-83）",
        operation: Operation::No,
        arg: Arg::Choice(&["find", "open", "next", "prev", "back"]),
    },
    Command {
        name: "quest",
        aliases: &["任务"],
        usage: "[doing|done|all] | 编号 [wiki] | back",
        summary: "查看任务进度：进行中、已完成或全部；quest 编号 看任务的百科攻略（STAGE-86、STAGE-89）",
        operation: Operation::No,
        arg: Arg::Choice(&["doing", "done", "all", "back"]),
    },
    Command {
        name: "party",
        aliases: &["队伍"],
        usage: "[clone|disband|invite|join|accept|decline|leave|kick|leader|say]",
        summary: "队伍：不带参数打开社交页；clone 组成分身队，disband 解散（先确认，STAGE-162）；say 队伍聊天（STAGE-277）",
        operation: Operation::Subs(&["clone", "disband", "invite", "join", "accept", "decline", "leave", "kick", "leader"]),
        arg: Arg::Choice(&["clone", "disband", "invite", "join", "accept", "decline", "leave", "kick", "leader", "say"]),
    },
    Command {
        name: "trade",
        aliases: &["交易"],
        usage: "名字 | accept [名字] | decline [名字] | cancel",
        summary: "交易：trade 名字 邀请附近的玩家；accept、decline 答复收到的邀请；cancel 撤回邀请或取消交易（STAGE-295）",
        operation: Operation::Yes,
        arg: Arg::Choice(&["accept", "decline", "cancel"]),
    },
    Command {
        name: "duel",
        aliases: &["决斗"],
        usage: "名字 [yes] | on | off",
        summary: "决斗：duel 名字 向同图的人发起决斗（先确认，末尾 yes 不问）；duel on|off 开关接受决斗（STAGE-307）",
        operation: Operation::Yes,
        arg: Arg::Choice(&["on", "off"]),
    },
    Command { name: "battle", aliases: &[], usage: "", summary: "查看当前战斗", operation: Operation::No, arg: Arg::None },
    Command { name: "encounter", aliases: &["遇敌"], usage: "on|off", summary: "遇敌开关（不带参数查询，STAGE-40）", operation: Operation::Yes, arg: Arg::Choice(&["on", "off"]) },
    Command {
        name: "auto-battle",
        aliases: &["auto", "托管"],
        usage: "on|off|status",
        summary: "自动挂机开关（不带参数查询；off 同时关闭后续遇敌，STAGE-42、STAGE-153）",
        operation: Operation::Subs(&["on"]),
        arg: Arg::Choice(&["on", "off", "status"]),
    },
    Command {
        name: "battle-log",
        aliases: &["战斗日志"],
        usage: "auto|brief|full",
        summary: "战斗日志档位：brief 每回合一行汇总，full 逐条；auto 在 agent 操控时用简略（不带参数查询）",
        operation: Operation::No,
        arg: Arg::Choice(&["auto", "brief", "full"]),
    },
    Command { name: "take", aliases: &[], usage: "", summary: "收回控制权（agent 操控时）", operation: Operation::No, arg: Arg::None },
    Command { name: "release", aliases: &[], usage: "", summary: "把控制权交还给 agent", operation: Operation::No, arg: Arg::None },
    Command { name: "kick-agent", aliases: &[], usage: "", summary: "断开 agent 的连接", operation: Operation::No, arg: Arg::None },
    Command { name: "say", aliases: &["s", "说"], usage: "[文本]", summary: "本图说话（不带文字打开聊天框）", operation: Operation::No, arg: Arg::None },
    Command { name: "world", aliases: &["w", "喊"], usage: "[文本]", summary: "世界喊话（不带文字打开聊天框）", operation: Operation::No, arg: Arg::None },
    Command { name: "party-say", aliases: &["队聊"], usage: "[文本]", summary: "队伍聊天，同 party say（STAGE-277）", operation: Operation::No, arg: Arg::None },
    Command { name: "ag", aliases: &[], usage: "文本", summary: "给 agent 发私聊", operation: Operation::No, arg: Arg::None },
    Command { name: "token", aliases: &["tokens"], usage: "new [备注] [天数] | list", summary: "管理 agent 令牌", operation: Operation::No, arg: Arg::Choice(&["new", "list"]) },
    Command { name: "agent", aliases: &[], usage: "[备注] [天数]", summary: "生成令牌并写出接入说明，整段复制给你的 agent（STAGE-170）", operation: Operation::No, arg: Arg::None },
    Command { name: "revoke", aliases: &[], usage: "编号", summary: "吊销令牌（编号见 token list）", operation: Operation::No, arg: Arg::TokenNo },
];

/// 战斗中的文字命令（`docs/battle-ui.md`「命令表」）。解析在 `app/battle/command.rs`，
/// 这里登记名字和简写，供 `help` 和战斗中的 Tab 补全使用。
pub struct BattleCommand {
    pub name: &'static str,
    pub short: &'static str,
    pub usage: &'static str,
    pub summary: &'static str,
}

pub const BATTLE_COMMANDS: &[BattleCommand] = &[
    BattleCommand { name: "attack", short: "a", usage: "<目标>", summary: "攻击" },
    BattleCommand { name: "magic", short: "m", usage: "<精灵编号> [目标]", summary: "用精灵" },
    BattleCommand { name: "item", short: "i", usage: "<槽位> [目标]", summary: "用道具" },
    BattleCommand { name: "defend", short: "g", usage: "", summary: "防御" },
    BattleCommand { name: "capture", short: "c", usage: "<目标>", summary: "捕捉" },
    BattleCommand { name: "escape", short: "e", usage: "", summary: "逃跑" },
    BattleCommand { name: "swap", short: "p", usage: "<宠物栏位>|-", summary: "换宠" },
    BattleCommand { name: "skill", short: "s", usage: "<技能编号> [目标]", summary: "宠物技能" },
    BattleCommand { name: "clone", short: "f", usage: "<1-4|p1-p4> <目标>", summary: "指定分身目标" },
];

impl Command {
    /// 这个命令（或它的子命令）是不是操作类、观察模式下要拦下。
    pub fn blocks(&self, sub: &str) -> bool {
        match self.operation {
            Operation::No => false,
            Operation::Yes => true,
            Operation::Subs(subs) => subs.contains(&sub),
        }
    }

    /// “用法：warp 地点编号|地点名|地图编号”，参数不对时统一用它提示。
    pub fn usage_line(&self) -> String {
        if self.usage.is_empty() { format!("用法：{}", self.name) } else { format!("用法：{} {}", self.name, self.usage) }
    }

    /// 输入行右侧的灰字提示（不带“用法：”前缀）。
    pub fn hint(&self) -> String {
        if self.usage.is_empty() { self.name.to_string() } else { format!("{} {}", self.name, self.usage) }
    }
}

impl BattleCommand {
    /// “用法：attack <目标>（简写 a）”。
    pub fn usage_line(&self) -> String {
        let usage = if self.usage.is_empty() { String::new() } else { format!(" {}", self.usage) };
        format!("用法：{}{}（简写 {}）", self.name, usage, self.short)
    }
}

/// 按名字或别名查命令（ASCII 大小写不敏感）。
pub fn find(word: &str) -> Option<&'static Command> {
    let lower = word.to_lowercase();
    COMMANDS.iter().find(|c| c.name == lower || c.aliases.contains(&lower.as_str()))
}

/// 按名字或简写查战斗命令。
pub fn battle_find(word: &str) -> Option<&'static BattleCommand> {
    let lower = word.to_lowercase();
    BATTLE_COMMANDS.iter().find(|b| b.name == lower || b.short == lower)
}

/// “用法：{name} {usage}”；不是登记过的命令就只回命令名。
pub fn usage_of(word: &str) -> String {
    find(word).map_or(format!("用法：{word}"), Command::usage_line)
}

// ---- Tab 补全 ----

/// 补全时能看到的上下文。
pub struct Ctx<'a> {
    pub places: &'a [Place],
    pub maps: &'a [WarpMap],
    pub token_count: usize,
    pub in_battle: bool,
}

/// 一次补全的结果：把输入行从 `from`（字符下标）起的当前词（长 `word_len`）换成候选；
/// 候选只有一个时直接换成它，多个时调用方补到公共前缀并把候选列出来。
pub struct Completion {
    pub from: usize,
    pub word_len: usize,
    pub candidates: Vec<String>,
}

/// Tab 补全：补输入行末尾的当前词。命令名和第一个参数有候选，更深的参数没有；没有候选返回 None。
pub fn complete(line: &str, ctx: &Ctx) -> Option<Completion> {
    let words: Vec<&str> = line.split_whitespace().collect();
    if words.is_empty() {
        return None;
    }
    // 当前词：行尾是空白说明刚开了一个空词（如“warp ”之后按 Tab）。
    let (from, word): (usize, &str) = if line.ends_with(char::is_whitespace) {
        (line.chars().count(), "")
    } else {
        let word = line.rsplit(char::is_whitespace).next().unwrap_or(line);
        (line.chars().count() - word.chars().count(), word)
    };
    let candidates = if from == 0 {
        command_candidates(word, ctx)
    } else {
        // 只补第一个参数（命令后面的那个）。
        let words_before = if word.is_empty() { words.len() } else { words.len() - 1 };
        if words_before != 1 {
            return None;
        }
        let spec = find(words[0])?;
        arg_candidates(spec.arg, word, ctx)
    };
    if candidates.is_empty() {
        return None;
    }
    Some(Completion { from, word_len: word.chars().count(), candidates })
}

/// 候选是否以这个词开头（ASCII 大小写不敏感）。
fn starts(candidate: &str, word: &str) -> bool {
    candidate.to_lowercase().starts_with(&word.to_lowercase())
}

/// 命令名候选：命令表的名字和别名（战斗中再加上战斗命令的名字和简写）。
fn command_candidates(word: &str, ctx: &Ctx) -> Vec<String> {
    let mut out: Vec<String> = vec![];
    let mut push = |s: &str| {
        if starts(s, word) && !out.iter().any(|x| x == s) {
            out.push(s.to_string());
        }
    };
    for c in COMMANDS {
        push(c.name);
        for a in c.aliases {
            push(a);
        }
    }
    if ctx.in_battle {
        for b in BATTLE_COMMANDS {
            push(b.name);
            push(b.short);
        }
    }
    out
}

/// 第一个参数的候选。
fn arg_candidates(kind: Arg, word: &str, ctx: &Ctx) -> Vec<String> {
    match kind {
        Arg::None => vec![],
        Arg::Command => command_candidates(word, ctx),
        Arg::Choice(list) => list.iter().filter(|s| starts(s, word)).map(|s| s.to_string()).collect(),
        Arg::TokenNo => {
            (1..=ctx.token_count).map(|n| n.to_string()).filter(|s| starts(s, word)).collect()
        }
        Arg::Place => {
            // 地点名在前，地图名接在后面（同名的只列一次，如“迷宫”）。
            let mut names: Vec<String> = ctx.places.iter().map(|p| p.name.clone()).collect();
            for m in ctx.maps {
                if !names.contains(&m.name) {
                    names.push(m.name.clone());
                }
            }
            let hits: Vec<String> = names.iter().filter(|n| starts(n, word)).cloned().collect();
            // 前缀没有命中时退到包含（“渔村”能补出“玛丽娜丝渔村”）。
            if hits.is_empty() && !word.is_empty() {
                names.into_iter().filter(|n| n.contains(word)).collect()
            } else {
                hits
            }
        }
    }
}

/// 候选的公共前缀（按字符算，中文一个字算一个字符）。
pub fn common_prefix(list: &[String]) -> String {
    let mut out = String::new();
    for (i, ch) in list[0].chars().enumerate() {
        if list.iter().all(|s| s.chars().nth(i) == Some(ch)) {
            out.push(ch);
        } else {
            break;
        }
    }
    out
}

impl App {
    /// Tab：补全输入行的最后一个词。唯一候选补全并加空格；
    /// 多个候选先补到公共前缀，没有进展时把候选写进消息栏。
    pub(super) fn complete_input(&mut self) {
        let Some(world) = self.world.as_ref() else {
            return;
        };
        if world.input.is_empty() {
            return;
        }
        let ctx = Ctx {
            places: &world.places,
            maps: &world.maps,
            token_count: world.agent.tokens.len(),
            in_battle: world.battle.is_some(),
        };
        let line = world.input.text().to_string();
        let Some(c) = complete(&line, &ctx) else {
            return;
        };
        let world = self.world.as_mut().unwrap();
        if c.candidates.len() == 1 {
            world.input.replace_tail(c.from, &format!("{} ", c.candidates[0]));
        } else {
            let common = common_prefix(&c.candidates);
            if common.chars().count() > c.word_len {
                world.input.replace_tail(c.from, &common);
            } else {
                // 地图名很多（几百个），候选列表只显示前面一部分，并写明总数。
                let shown: Vec<&str> = c.candidates.iter().take(CANDIDATE_LIST_LIMIT).map(String::as_str).collect();
                let more = if c.candidates.len() > shown.len() { format!("  …共 {} 个", c.candidates.len()) } else { String::new() };
                self.log.system(format!("候选：{}{more}", shown.join("  ")));
            }
        }
    }

    /// `help`：列出全部命令；`help 命令名` 看它的用法、别名、说明和观察模式提示。
    pub(super) fn help_command(&mut self, rest: &str) {
        let name = rest.trim();
        if name.is_empty() {
            self.log.system("输入 help 命令名 看详细用法。全部命令：");
            for c in COMMANDS {
                let aliases = if c.aliases.is_empty() { String::new() } else { format!("（{}）", c.aliases.join("、")) };
                let usage = if c.usage.is_empty() { String::new() } else { format!(" {}", c.usage) };
                self.log.system(format!("  {}{aliases}{usage} — {}", c.name, c.summary));
            }
            let battle: Vec<String> = BATTLE_COMMANDS
                .iter()
                .map(|b| {
                    let usage = if b.usage.is_empty() { String::new() } else { format!(" {}", b.usage) };
                    format!("{}/{}{}", b.name, b.short, usage)
                })
                .collect();
            self.log.system(format!("战斗中：{}（目标写 e3 敌方 3 号、m2 我方 2 号）", battle.join("、")));
            return;
        }
        if let Some(c) = find(name) {
            self.log.system(c.usage_line());
            let mut note = c.summary.to_string();
            if !c.aliases.is_empty() {
                note += &format!("；别名：{}", c.aliases.join("、"));
            }
            self.log.system(note);
            match c.operation {
                Operation::Yes => self.log.system("这是操作类命令：agent 操控时会被拦下"),
                Operation::Subs(subs) => self.log.system(format!("其中 {} 是操作类：agent 操控时会被拦下", subs.join("/"))),
                Operation::No => {}
            }
        } else if let Some(b) = battle_find(name) {
            self.log.system(b.usage_line());
            self.log.system("战斗中的命令，目标与参数的写法见 docs/battle-ui.md");
        } else {
            self.log.error(format!("没有叫“{name}”的命令"));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn ctx<'a>(places: &'a [Place], tokens: usize) -> Ctx<'a> {
        Ctx { places, maps: &[], token_count: tokens, in_battle: false }
    }

    fn place(id: u32, name: &str) -> Place {
        Place { id, name: name.into(), floor: 100, x: 0, y: 0 }
    }

    #[test]
    fn finds_by_name_and_alias() {
        assert_eq!(find("warp").unwrap().name, "warp");
        assert_eq!(find("传送").unwrap().name, "warp");
        assert_eq!(find("tokens").unwrap().name, "token");
        assert_eq!(find("Help").unwrap().name, "help", "大小写不敏感");
        assert!(find("没有").is_none());
        assert_eq!(battle_find("a").unwrap().name, "attack");
        assert!(find("a").is_none(), "战斗命令不进主命令表，避免和普通命令抢名字");
    }

    #[test]
    fn operation_commands_block_in_observer_mode() {
        let blocks = |name: &str, sub: &str| find(name).unwrap().blocks(sub);
        assert!(blocks("turn", ""));
        assert!(blocks("warp", ""));
        assert!(blocks("addpoint", ""));
        assert!(blocks("item", "equip"));
        assert!(!blocks("status", ""));
        assert!(!blocks("pet", "status"), "pet status 只是查看");
        assert!(blocks("pet", "drop"));
        assert!(blocks("pet", "skill"), "pet skill 会让宠物用技能，观察方不能发");
        assert!(blocks("pet", "ride"), "pet ride 会改宠物状态，观察方不能发（STAGE-123）");
        assert!(blocks("pet", "talk"), "pet talk 会改宠物上限、给道具，观察方不能发（STAGE-239）");
    }

    /// STAGE-162：`party` 命令登记在表里（别名“队伍”），clone、disband 是操作类、观察模式下拦，单独 `party` 只是打开页面。
    #[test]
    fn party_command_blocks_only_its_operations_and_completes_subcommands() {
        assert_eq!(find("队伍").unwrap().name, "party");
        let party = find("party").unwrap();
        assert!(party.blocks("clone") && party.blocks("disband"));
        assert!(!party.blocks(""), "打开社交页不是操作");
        assert_eq!(complete("party c", &ctx(&[], 0)).unwrap().candidates, vec!["clone".to_string()]);
        assert_eq!(complete("party d", &ctx(&[], 0)).unwrap().candidates, vec!["disband".to_string(), "decline".to_string()]);
        assert_eq!(usage_of("party"), "用法：party [clone|disband|invite|join|accept|decline|leave|kick|leader|say]");
    }

    #[test]
    fn wiki_command_is_registered_and_completes_subcommands() {
        assert_eq!(find("百科").unwrap().name, "wiki");
        assert!(!find("wiki").unwrap().blocks("find"), "百科是纯查询，agent 操控时也能用");
        let got = complete("wiki fi", &ctx(&[], 0)).unwrap();
        assert_eq!(got.candidates, vec!["find".to_string()]);
    }

    #[test]
    fn completes_command_names() {
        let c = ctx(&[], 0);
        let got = complete("wa", &c).unwrap();
        assert_eq!((got.from, got.word_len), (0, 2));
        assert_eq!(got.candidates, vec!["warp"]);
        assert_eq!(complete("a", &c).unwrap().candidates, vec!["addpoint", "auto-battle", "auto", "ag", "agent"]);
        assert!(complete("没有的命令名", &c).is_none());
        assert!(complete("", &c).is_none());
    }

    #[test]
    fn completes_arguments() {
        let places = vec![place(1, "萨姆吉尔村"), place(2, "玛丽娜丝渔村"), place(3, "萨伊那斯")];
        let c = ctx(&places, 12);
        // 前缀唯一的地点直接补全。
        let got = complete("warp 萨姆", &c).unwrap();
        assert_eq!(got.candidates, vec!["萨姆吉尔村"]);
        assert_eq!(got.from, "warp ".chars().count());
        // 不止一个时全部列出。
        assert_eq!(complete("warp 萨", &c).unwrap().candidates, vec!["萨姆吉尔村", "萨伊那斯"]);
        // 前缀没有命中就退到包含。
        assert_eq!(complete("warp 渔村", &c).unwrap().candidates, vec!["玛丽娜丝渔村"]);
        // 尾随空格＝开始一个新词，列出全部候选。
        assert_eq!(complete("warp ", &c).unwrap().candidates.len(), 3);
        // 能力名、子命令、令牌编号。
        assert_eq!(complete("addpoint 耐", &c).unwrap().candidates, vec!["耐久", "耐久力"]);
        assert_eq!(complete("item eq", &c).unwrap().candidates, vec!["equip"]);
        assert_eq!(complete("token l", &c).unwrap().candidates, vec!["list"]);
        assert_eq!(complete("revoke 1", &c).unwrap().candidates, vec!["1", "10", "11", "12"]);
        // help 补命令名；第二个参数不补。
        assert_eq!(complete("help wa", &c).unwrap().candidates, vec!["warp"]);
        assert!(complete("item move 3 ", &c).is_none());
    }

    #[test]
    fn battle_commands_complete_only_in_battle() {
        let c = ctx(&[], 0);
        assert!(complete("at", &c).is_none(), "平时没有以 at 开头的命令");
        let mut battle = ctx(&[], 0);
        battle.in_battle = true;
        assert_eq!(complete("at", &battle).unwrap().candidates, vec!["attack"]);
        assert!(complete("a", &battle).unwrap().candidates.contains(&"a".to_string()));
    }

    #[test]
    fn common_prefix_counts_chars() {
        assert_eq!(common_prefix(&["萨姆吉尔村".into(), "萨伊那斯".into()]), "萨");
        assert_eq!(common_prefix(&["abc".into(), "abcd".into()]), "abc");
        assert_eq!(common_prefix(&["ab".into(), "cd".into()]), "");
    }

    #[test]
    fn usage_lines_come_from_the_table() {
        assert_eq!(usage_of("warp"), "用法：warp 地点编号|地点名|地图编号");
        assert_eq!(usage_of("quit"), "用法：quit");
        assert_eq!(usage_of("传送"), "用法：warp 地点编号|地点名|地图编号");
        assert_eq!(find("attack").map(Command::usage_line), None, "战斗命令的用法在 battle_find 里");
        assert_eq!(battle_find("attack").unwrap().usage_line(), "用法：attack <目标>（简写 a）");
    }

    #[test]
    fn warp_completes_map_names_after_places() {
        let places = vec![place(1, "萨姆吉尔村")];
        let map = |floor, name: &str| WarpMap { floor, name: name.into() };
        let maps = vec![map(1006, "萨姆吉尔的村长家"), map(117, "迷宫"), map(118, "迷宫")];
        let c = Ctx { places: &places, maps: &maps, token_count: 0, in_battle: false };
        assert_eq!(complete("warp 村长", &c).unwrap().candidates, vec!["萨姆吉尔的村长家"], "没有前缀命中时退到包含");
        assert_eq!(complete("warp 迷", &c).unwrap().candidates, vec!["迷宫"], "同名地图只列一次");
        assert_eq!(complete("warp 萨", &c).unwrap().candidates, vec!["萨姆吉尔村", "萨姆吉尔的村长家"], "地点在前，地图接后");
    }
}
