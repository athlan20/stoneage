//! 输入行为空时的提示（STAGE-104，`docs/interaction.md`「提示行」）：列出最上面那一层能用的键。
//! 提示和按键处理读同一份状态（确认、对话窗口、战斗的阶段和选择），这里只把状态翻译成文字，不另外判断。
//! 层的顺序和按键分层一致：确认 > 弹窗 > 战斗 > 当前页。

use super::battle::{Phase, Pick};
use super::page_keys;
use super::pet_merge::is_merge_skill;
use super::scroll;
use super::select;
use super::world::{MAP_MENU, MESSAGES_MENU, WorldState};
use super::{BagStep, PetStep, SceneStep};

/// 输入行为空时的提示，每一段是一组按键（如 `←→ 栏目`），放不下时由显示端从右往左截。
pub fn key_hint(world: &WorldState) -> Vec<String> {
    if (world.menu==super::SYSTEM_MENU && world.nav.is(&super::Layer::Page(super::SYSTEM_MENU)) && world.npc_window.is_none() && world.confirm.is_none()) || world.system.battle_open {
        return vec![match &world.system.step {
            Some(super::system::SystemStep::Chat(_))=>"输入私聊 · Enter 发送 · Esc 返回",
            Some(super::system::SystemStep::Create{..})=>"Tab 字段 · ←→ 按钮 · Enter 继续 · Esc 取消",
            Some(super::system::SystemStep::Fresh)=>"Enter / Esc/Backspace 清除原文并返回",
            Some(super::system::SystemStep::Revoke{..})=>"←→ 选择 · Enter 确认 · Esc 取消",
            _ if world.system.battle_open=>"↑↓ 选择 · Enter 执行 · Esc 关闭面板 · / 命令",
            _=>"↑↓ 选择 · Enter 打开 · Esc/Backspace 返回 · / 命令",
        }.into()];
    }
    if world.browse.step.is_some() && !world.battle_front() && world.npc_window.is_none() && world.confirm.is_none() {
        return vec![if world.nav.is(&super::Layer::Detail) { "↑↓ 阅读 · PgUp/PgDn 翻页 · Esc/Backspace 返回" }
            else if world.nav.is(&super::Layer::Form) { "输入关键词 · Enter 搜索 · Esc 取消" }
            else { "↑↓ 选择 · Enter 打开 · Esc/Backspace 返回" }.into()];
    }
    // 确认层（y 才执行，其他取消）：是操作类，观察模式下拦。
    if world.confirm.is_some() && world.confirm_native {return vec!["←→ 选择 · Enter 确认 · Esc 取消".into()];}
    if world.confirm.is_some() {
        return layer(world, true, vec!["y 确认".into(), "其他取消".into()]);
    }
    if let Some(w) = world.npc_window.as_ref() {
        if !w.window_id.is_empty() {
            return layer(world, true, vec![
                if w.trade.is_some() {
                    "←→ 选择 · Enter 确认 · Esc 取消"
                } else if w.input.is_some() {
                    "输入内容 · Enter 提交 · Esc 关闭"
                } else if w.ranking.is_some() {
                    "↑↓ 选择 · Enter 确认 · Esc 关闭"
                } else {
                    "↑↓ 选择 · Enter 确认 · PgUp/PgDn 翻页 · Esc 关闭"
                }
                .into(),
            ]);
        }
        if w.accepts_text {
            return layer(world, true, vec!["reply 文本".into(), "Esc 关闭".into()]);
        }
        if !w.options.is_empty() {
            return layer(
                world,
                true,
                vec![format!("1-{} 选择", w.options.len()), "Esc 关闭".into()],
            );
        }
        // 没有选项的窗口：Enter 和数字键都只是关窗，不是操作。
        return layer(world, false, vec!["Enter 关闭".into()]);
    }
    // 自动战斗设置页（STAGE-173）：顶层页、系统页、Tab 面板三个入口共用一套按键说明。
    // 设置页开着就占住按键（战斗在后台继续跑），所以提示行也只写设置页的按键。
    if world.auto_edit.is_some() {
        return vec!["Tab 换栏 · ↑↓ 选择 · ←→ 调整 · Enter 执行 · Esc 返回".into()];
    }
    if let Some(b) = world.battle.as_ref().filter(|_| !world.battle_veil) {
        // 战斗控制面板（STAGE-153）：任何阶段、任何玩家都能按 Tab 打开，提示行里一直写着入口。
        let panel = "Tab 控制";
        // 自动战斗时（STAGE-173）：不管在下指令、结算还是等待，底部都只留这两个入口。
        if world.auto_battle {
            return vec!["8 停止自动挂机".into(), "7 停止自动遇敌".into(), "Tab 自动战斗设置".into()];
        }
        if b.result.is_some() || b.phase == Phase::Ended {
            return layer(world, false, vec!["Enter 继续".into(), panel.into()]);
        }
        // 观察模式（agent 操控）下 Esc 打开游戏菜单看背包、宠物（STAGE-182），提示行里写出入口。
        let panel = if world.observing() { "Esc 菜单 · Tab 控制" } else { panel };
        return match b.phase {
            // 回放速度与“跳过”都只在系统页设（STAGE-173），回放阶段没有按键。
            Phase::Replay => vec![] as Vec<String>,
            // 指令条停在第一项时 Esc 不起作用，只有进了道具、技能等子列表或选目标才写“Esc 返回”。
            Phase::Input(actor) => match b.pick {
                Pick::Action => {
                    let mut keys = vec![format!("1-{} 指令", b.menu(actor).len())];
                    if b.has_optional() {
                        keys.push("f 分身".into());
                    }
                    // 宠物指令停在第一项、能退回人物指令时写 Esc 改人物指令（STAGE-109）。
                    if b.can_revert() {
                        keys.push("Esc 改人物指令".into());
                    }
                    keys.push(panel.into());
                    layer(world, true, keys)
                }
                Pick::Target { .. } => layer(
                    world,
                    true,
                    vec![
                        "←↑↓→ 选目标".into(),
                        "Enter 确认".into(),
                        "数字直选".into(),
                        "Esc 返回".into(),
                        panel.into(),
                    ],
                ),
                Pick::Clones => layer(
                    world,
                    true,
                    vec!["数字 1–8 直选".into(), "↑↓ Enter 选目标".into(), "Esc 返回".into(), panel.into()],
                ),
                Pick::CloneAim { .. } => layer(
                    world,
                    true,
                    vec!["←↑↓→ 选目标".into(), "Enter 确认".into(), "数字直选".into(), "Esc 返回".into(), panel.into()],
                ),
                Pick::Item | Pick::Pet | Pick::Magic | Pick::Skill => {
                    layer(world, true, vec!["数字选编号".into(), "Esc 返回".into(), panel.into()])
                }
            },
            Phase::Waiting | Phase::Ended => vec![panel.into()],
        };
    }
    // 社交页的动作菜单与成员详情（STAGE-162）：看详情不是操作，解散在确认层里才拦观察模式。
    if let Some(step) = world.party_step.as_ref() {
        return match step {
            super::PartyStep::Actions { .. } => vec!["↑↓ 选择 · Enter 执行 · Esc/Backspace 返回".into()],
            super::PartyStep::Detail { .. } => vec!["↑↓ 阅读 · PgUp/PgDn 翻页 · Esc/Backspace 返回".into()],
        };
    }
    // 社交页设置栏（STAGE-276）：Enter 或空格切换开关。
    if world.menu == super::SOCIAL_MENU
        && world.nav.is(&super::Layer::Page(super::SOCIAL_MENU))
        && super::social_settings::on_settings_column(world)
    {
        return layer(world, true, vec!["Enter 或空格 切换".into(), "←→ 换栏".into()]);
    }
    // 社交页没有队伍时 Enter 直接组成分身队（STAGE-162）；组队是操作类，观察模式下拦。
    if world.menu == super::SOCIAL_MENU
        && world.nav.is(&super::Layer::Page(super::SOCIAL_MENU))
        && world.party().is_none()
        && !super::party_players::on_request_column(world)
    {
        return layer(world, true, vec!["Enter 组成分身队".into(), "/ 命令".into()]);
    }
    if matches!(world.nav.top(), super::Layer::Page(_) | super::Layer::Detail) {
        if world.menu == super::QUEST_MENU {
            return vec![if world.quest_guide.is_some() { "↑↓ 阅读 · PgUp/PgDn 翻页 · Esc/Backspace 返回" }
                else { "←→ 分类 · ↑↓ 选择 · Enter 动作 · Esc/Backspace 返回" }.into()];
        }
        if world.menu == super::WIKI_MENU {
            return vec![if world.wiki.open.is_some() { "↑↓ 阅读 · PgUp/PgDn 翻页 · Esc/Backspace 返回" }
                else { "↑↓ 选择 · Enter 打开 · Tab 搜索 · Esc/Backspace 返回" }.into()];
        }
    }
    // 背包对象动作流程（STAGE-132）：动作、目标、目的格、数量各自一组键；会改数据，观察模式下拦。
    if let Some(step) = world.bag_step.as_ref() {
        let keys = match step {
            BagStep::Actions { .. } => {
                vec!["↑↓ 选择".into(), "Enter 执行".into(), "Esc/Backspace 返回".into()]
            }
            BagStep::Target { .. } => {
                vec!["↑↓ 选择".into(), "Enter 使用".into(), "Esc/Backspace 返回".into()]
            }
            BagStep::MoveTo { .. } => {
                vec!["↑↓ 选择".into(), "Enter 移动".into(), "Esc/Backspace 返回".into()]
            }
            BagStep::InlayPick { .. } => {
                vec!["↑↓ 选择".into(), "Space 勾选".into(), "Enter 确认".into(), "Esc/Backspace 返回".into()]
            }
            BagStep::DropCount { .. } => {
                vec!["数字 数量".into(), "Enter 继续".into(), "Esc 取消".into()]
            }
        };
        return layer(world, true, keys);
    }
    // 宠物动作流程与角色加点表单（STAGE-133）：都是会改数据的操作，观察模式下拦。
    if let Some(step) = world.pet_step.as_ref() {
        let keys = match step {
            PetStep::Actions { .. } => {
                vec!["↑↓ 选择".into(), "Enter 执行".into(), "Esc/Backspace 返回".into()]
            }
            PetStep::Rename { .. } => {
                vec!["输入 名字".into(), "Enter 确定".into(), "Esc 取消".into()]
            }
            PetStep::Skills { slot, index } => {
                // 光标在加工、料理上才有 Enter（STAGE-208）。
                let merge = world.pet_at(*slot).and_then(|p| p.skills.get(*index).map(|s| is_merge_skill(s.id)));
                if merge == Some(true) {
                    vec!["↑↓ 查看".into(), "Enter 选材料".into(), "Esc/Backspace 返回".into()]
                } else {
                    vec!["↑↓ 查看".into(), "Esc/Backspace 返回".into()]
                }
            }
            PetStep::Merge { .. } => vec![
                "↑↓ 选择".into(),
                "空格 勾选".into(),
                "Enter 合成".into(),
                "Esc/Backspace 返回".into(),
            ],
            PetStep::MergeResult { .. } => vec!["Enter 再合成".into(), "Esc/Backspace 返回".into()],
        };
        return layer(world, true, keys);
    }
    if world.char_step.is_some() {
        return layer(
            world,
            true,
            vec!["数字 点数".into(), "Enter 确认".into(), "Esc 取消".into()],
        );
    }
    // 场景页的查找表单（STAGE-134）。传送到其他地点在 STAGE-161 起走快捷传送页，不再是这里的步骤。
    if let Some(step) = world.scene_step.as_ref() {
        let keys = match step {
            SceneStep::Catalog { .. } => vec![
                "输入 名字".into(),
                "Tab 分类".into(),
                "Enter 对话 / 传送".into(),
                "Esc 返回".into(),
            ],
        };
        return layer(world, true, keys);
    }
    // 导航栈顶那一层的提示（STAGE-131）：场景和游戏菜单各有自己的键。
    // 场景里 Enter 是对 NPC 对话或进传送页，属操作类，观察模式下换成收回控制权。
    if world.at_scene() && world.scene.details {
        return vec!["↑↓ 阅读 · PgUp/PgDn 翻页 · Tab / Esc/Backspace 返回".into()];
    }
    if world.at_scene() {
        return layer(
            world,
            true,
            vec![
                "←→ 切列".into(),
                "↑↓/翻页 选择".into(),
                "Enter 打开".into(),
                                "Esc 菜单".into(),
                "/ 命令".into(),
            ],
        );
    }
    if world.at_menu() {
        return layer(
            world,
            false,
            vec![
                "↑↓ 选择".into(),
                "Enter 进入".into(),
                "Esc/Backspace 场景".into(),
                "/ 命令".into(),
            ],
        );
    }
    // 快捷传送页（STAGE-161）：一层一层选，Enter 下钻或传送，Esc 逐级返回。
    if world.menu == MAP_MENU && world.nav.is(&super::Layer::Page(MAP_MENU)) {
        if !world.travel.ready() {
            return layer(world, false, vec!["Enter 重新获取".into(), "Esc/Backspace 返回".into()]);
        }
        let mut keys = vec!["↑↓/翻页 选择".into(), "Enter 进入 / 传送".into()];
        if !world.travel.path().is_empty() {
            keys.push("Esc/Backspace 上一层".into());
        }
        keys.push("/ 命令".into());
        return layer(world, true, keys);
    }
    // 世界，没有弹窗：←→ 栏目（STAGE-113）；页面有能选的项时写选择的键，没有就写历史和 help；
    // 能滚动又没有选择栏的页面加上翻页键（有选择栏时 PgUp/PgDn 移动选中项，不再写翻页）。
    // 「消息」页没有选择栏，`↑↓` 是逐行滚动，写滚动和回到最新（STAGE-116）。
    let columns = select::page_columns(world);
    let mut keys = vec!["←→ 栏目".to_string()];
    if select::selectable(&columns) {
        keys.extend(select::hint(&columns, &world.select[world.menu]));
        if page_keys::can_back(world) {
            keys.push("Esc/Backspace 返回".to_string());
        }
    } else if world.menu == MESSAGES_MENU {
        keys.extend(["↑↓ 滚动".to_string(), "End 最新".to_string()]);
    } else {
        keys.extend(["↑↓ 历史".to_string(), "help 命令".to_string()]);
        if scroll::scrolls(world.menu) {
            keys.push("PgUp/PgDn 翻页".to_string());
        }
    }
    keys
}

/// 当前层的提示。`operation` 为真表示这一层的数字、Enter 等是操作类快捷键：观察模式下这些键被拦，
/// 提示换成“agent 操控中 · take 收回”，让玩家知道为什么按了没反应。
fn layer(world: &WorldState, operation: bool, keys: Vec<String>) -> Vec<String> {
    if operation && world.observing() {
        let mut hint=vec!["agent 操控中".into(), "take 收回".into()];if world.battle.is_some()&&!world.battle_veil{hint.push("Esc 菜单".into());hint.push("Tab 系统".into());}return hint;
    }
    keys
}
