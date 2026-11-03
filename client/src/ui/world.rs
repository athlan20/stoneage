//! 进入游戏后的主界面外壳：上菜单、中内容、下消息与命令输入（`docs/ui.md`）。
//! 第 1 页「快捷传送」见 `travelpage.rs`，第 5 页「任务」见 `questpage.rs`，第 7 页「消息」见 `messagepage.rs`。

use ratatui::Frame;
use ratatui::layout::{Alignment, Constraint, Layout, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span};
use ratatui::widgets::{Block, List, Paragraph};
use unicode_width::UnicodeWidthStr;

use super::autosettings;
use super::battle;
use super::charpage;
use super::gamemenu;
use super::inventpage;
use super::messagepage;
use super::petpage;
use super::questpage;
use super::scene;
use super::socialpage;
use super::travelpage;
use super::system;
use super::widgets::message_items;
use super::wikipage;
use crate::app::commands;
use crate::app::{
    App, AUTO_MENU, BAG_MENU, Layer, MAP_MENU, MENU_TITLES, MESSAGES_MENU, PETS_MENU, QUEST_MENU, SOCIAL_MENU,
    SYSTEM_MENU, WIKI_MENU, WorldState, key_hint,
};

/// 战斗画面在前台时底部的行数（STAGE-248）：边框 2 行 + 分隔线 1 行 + 输入行 1 行，不再有消息区，
/// 战斗文字只在战斗日志区显示。战场的高度按这个数算（`ui/battle/mod.rs`）。
pub(crate) const BATTLE_BOTTOM_ROWS: u16 = 4;

pub fn render(frame: &mut Frame, app: &mut App, area: Rect) {
    // 战斗在前台（`battle_front`，观察模式菜单盖在战斗上时不算，STAGE-182）：底部是战斗的 4 行，没有消息区。
    let battle_front = app.world.as_ref().is_some_and(|w| w.battle_front());
    // 「消息」页（非战斗）底部只留输入行，消息栏隐藏（STAGE-111）：底部 3 行，省下的行给中间的消息页。
    let compact = app
        .world
        .as_ref()
        .is_some_and(|w| !w.battle_front() && w.menu == MESSAGES_MENU);
    let bottom_height: u16 = if compact { 3 } else if battle_front { BATTLE_BOTTOM_ROWS } else { 9 };
    let message_rows=bottom_height.saturating_sub(4);
    // STAGE-250：战斗顶栏在骑着宠物且内容区够高时是 2 行（骑宠信息），否则 1 行，见 battle::top_height。
    let top_height = battle::top_height(app.world.as_ref().and_then(|w| w.battle.as_ref()), area.height);
    let [top, mid, bottom] = Layout::vertical([
        Constraint::Length(top_height),
        Constraint::Min(0),
        Constraint::Length(bottom_height),
    ])
    .areas(area);
    // 中间页面区域的宽度和高度记给页面选择用：详情并排还是替换列表、哪些栏画得出来，按它判断（STAGE-115、STAGE-116）。
    if let Some(world) = app.world.as_mut() {
        world.page_width = mid.width;
        world.page_height = mid.height;
    }
    let Some(world) = app.world.as_ref() else {
        return;
    };
    let e = &world.entry;
    let s = &e.status;
    let status = format!(
        "Lv{} HP {}/{} MP {}/{}",
        s.level, s.hp, s.max_hp, s.mp, s.max_mp
    );
    let battle_on = world.battle.is_some();
    // 观察模式下菜单盖在战斗上（STAGE-182）：顶部仍是战斗信息条，中间和底部按非战斗画。
    let front = world.battle_front();
    let mut right = vec![];
    if !world.online{right.push(Span::styled("离线 · 资料可能过期 ",Style::default().fg(Color::Yellow).add_modifier(Modifier::BOLD)));}
    if battle_on {
        // 战斗中：边框变红，右侧写“战斗中”；标签行换成战斗信息条（STAGE-106，`docs/battle-ui.md`）。1–7 不切换页面。
        right.push(Span::styled(
            "战斗中 ",
            Style::default().fg(Color::Red).add_modifier(Modifier::BOLD),
        ));
    } else {
        // 观察模式：agent 在操控，菜单栏右侧显示黄色的“agent 操控中”（docs/agent.md）。
        if world.observing() {
            right.push(Span::styled(
                "agent 操控中 ",
                Style::default()
                    .fg(Color::Yellow)
                    .add_modifier(Modifier::BOLD),
            ));
        }
        // 托管（自动战斗，STAGE-42）：平时也显示，玩家随时知道托管还开着；战斗中写在信息条里。
        if world.auto_battle {
            right.push(Span::styled("自动挂机中 ", Style::default().fg(Color::Yellow)));
        }
    }
    let mut block = Block::bordered()
        .title(format!("石器时代 · {}", e.character.name))
        .title(Line::from(right).right_aligned());
    if battle_on {
        block = block.border_style(Style::default().fg(Color::Red));
    }
    let inner = block.inner(top);
    frame.render_widget(block, top);
    match world.battle.as_ref() {
        Some(b) => {
            let width = usize::from(inner.width);
            let mut lines = vec![battle::top_line(b, world.observing(), world.auto_battle, width)];
            // STAGE-250：第二行是骑宠，只在顶栏有 2 行内容时画（顶栏高度见上面的 top_height）。
            if inner.height >= 2 {
                lines.extend(battle::mount_line(b, width));
            }
            frame.render_widget(Paragraph::new(lines), inner);
        }
        None => {
            // STAGE-131 的新外壳：左边是导航路径（场景 › 菜单 › 背包），右边是角色状态。
            // 原来常驻的九个菜单标签改成按 Esc 打开的「游戏菜单」，标签行不再占位置。
            let path = nav_path(world);
            frame.render_widget(
                Paragraph::new(Line::from(Span::styled(
                    path,
                    Style::default().fg(Color::Green),
                ))),
                inner,
            );
            // 有队伍时右侧加“队伍 5/5”：在场景和任何页面都看得到队伍还在（STAGE-162）。
            let mut right = vec![Span::styled(status.clone(), Style::default().fg(Color::DarkGray))];
            if let Some(p) = world.party() {
                right.push(Span::styled(format!("  队伍 {}/{}", p.size(), crate::proto::MAX_PARTY_SIZE), Style::default().fg(Color::Green)));
            }
            frame.render_widget(Paragraph::new(Line::from(right)).alignment(Alignment::Right), inner);
        }
    }

    let menu = world.menu;
    let is_messages = menu == MESSAGES_MENU;
    let is_bag = menu == BAG_MENU;
    let is_pets = menu == PETS_MENU;
    let is_wiki = menu == WIKI_MENU;
    let is_quest = menu == QUEST_MENU;
    let is_social = menu == SOCIAL_MENU;
    let is_auto = menu == AUTO_MENU;
    // 自动战斗设置开着就占满中间区域（STAGE-173）：战斗在后台继续，设置管设置。
    let editing = world.auto_edit.is_some();
    // 导航栈顶（STAGE-131）：场景和游戏菜单各自占满中间区域，其余画当前功能页。
    let at_scene = world.nav.is(&Layer::Scene) || world.scene_step.is_some();
    let at_menu = world.nav.is(&Layer::Menu);
    if !front
        && !at_scene
        && !at_menu
        && !is_messages
        && !is_bag
        && !is_pets
        && !is_wiki
        && !is_quest
        && !is_social
        && !is_auto
        && menu != MAP_MENU
    {
        let title = match menu {
            SYSTEM_MENU => "系统 · agent 令牌".to_string(),
            n => MENU_TITLES[n]
                .trim_start_matches(|c: char| c.is_ascii_digit())
                .to_string(),
        };
        let block = Block::bordered().title(title);
        let inner = block.inner(mid);
        frame.render_widget(block, mid);
        if menu == 1 {
            charpage::render(frame, app, inner);
        } else if menu == SYSTEM_MENU {
            system::render(frame, app, inner);
        } else {
            frame.render_widget(
                Paragraph::new("  这一页尚未实现。").style(Style::default().fg(Color::DarkGray)),
                inner,
            );
        }
    }

    // NPC 对话窗口（STAGE-69）：盖在菜单页上面（战斗时服务端已经把窗口关掉）。

    // 下：边框 2 行 + 消息 5 行 + 分隔线 1 行 + 输入 1 行 = 9 行（原来只留了 8 行，消息栏只能显示 4 条）。
    // 消息页只剩边框和输入行（compact）。
    let input_block = Block::bordered().title("消息");
    let input_inner = input_block.inner(bottom);
    let (msgs, sep, input) = if compact {
        (Rect::default(), Rect::default(), input_inner)
    } else {
        let [msgs, sep, input] = Layout::vertical([
            Constraint::Length(message_rows),
            Constraint::Length(1),
            Constraint::Length(1),
        ])
        .areas(input_inner);
        (msgs, sep, input)
    };
    // 输入行的内容和提示先算好（只读借用），消息页和背包页的渲染需要可变借用。
    let width = usize::from(input.width);
    let (visible, cursor_col) = if !world.nav.is(&Layer::Command) && !world.npc_window.as_ref().is_some_and(|n|n.window_id.is_empty()) && !(world.confirm.is_some()&&!world.confirm_native){(String::new(),0)}else{world.input.view(width.saturating_sub(2))};
    let hint = input_hint(world);

    // 消息页和背包页要更新滚动状态，需要可变借用，放在 `world` 的只读借用结束之后。
    // 战斗时中间区域整个换成战场，菜单页不画（菜单栏的选中项保留，战斗结束后回到原来的页面）。
    if editing {
        // 自动战斗设置（STAGE-173）盖在战场上面：战斗在后台继续跑，设置管设置。
        let block = Block::bordered().title("自动战斗设置");
        let inner = block.inner(mid);
        frame.render_widget(block, mid);
        autosettings::render(frame, app, inner);
    } else if front {
        battle::render(frame, app, mid);
        if app.world.as_ref().is_some_and(|w|w.system.battle_open){frame.render_widget(ratatui::widgets::Clear,mid);system::render(frame,app,mid);}
    } else if at_scene {
        scene::render(frame, app, mid);
    } else if at_menu {
        gamemenu::render(frame, app, mid);
    } else if is_messages {
        messagepage::render(frame, app, mid);
    } else if is_bag {
        inventpage::render(frame, app, mid);
    } else if is_pets {
        petpage::render(frame, app, mid);
    } else if is_wiki {
        wikipage::render(frame, app, mid);
    } else if is_quest {
        questpage::render(frame, app, mid);
    } else if is_social {
        socialpage::render(frame, app, mid);
    } else if menu == MAP_MENU {
        // 快捷传送（STAGE-161）：整块区域由 travelpage 自己画边框和标题。
        travelpage::render(frame, app, mid);
    }

    if app.world.as_ref().is_some_and(|w|w.confirm.is_some()&&w.confirm_native && !w.nav.is(&Layer::Command)) {
        let w=app.world.as_ref().unwrap();let area=super::widgets::centered(mid,70,7);frame.render_widget(ratatui::widgets::Clear,area);
        let text=format!("{}\n\n{}  {}\n←→ 选择 · Enter 执行 · Esc 取消",w.confirm_text,if !w.confirm_choice{"[取消]"}else{" 取消 "},if w.confirm_choice{"[确认]"}else{" 确认 "});
        frame.render_widget(Paragraph::new(text).wrap(ratatui::widgets::Wrap{trim:false}).block(Block::bordered().title("确认操作")),area);
    }
    frame.render_widget(input_block, bottom);
    if !compact {
        // 战斗在前台时没有消息区（msgs 高度为 0），只画分隔线。
        if !battle_front {
            frame.render_widget(List::new(message_items(app, usize::from(message_rows))), msgs);
        }
        frame.render_widget(Paragraph::new("─".repeat(usize::from(sep.width))), sep);
    }
    // 输入行：左边是命令（超过宽度时横向滚动，STAGE-26），右边是灰字提示。
    frame.render_widget(Paragraph::new(format!("> {visible}")), input);
    if app
        .world
        .as_ref()
        .is_some_and(|w| w.nav.is(&Layer::Command) || w.npc_window.as_ref().is_some_and(|n|n.window_id.is_empty() && n.accepts_text))
    {
        frame.set_cursor_position((input.x + 2 + cursor_col, input.y));
    }
    // 灰字放在输入文字之后，剩下的宽度不够时从右往左截（docs/interaction.md「提示行」）。
    let used = 2 + UnicodeWidthStr::width(visible.as_str());
    if let Some(hint) = fit_hint(hint, width.saturating_sub(used + 1)) {
        let hint = Paragraph::new(hint)
            .style(Style::default().fg(Color::DarkGray))
            .alignment(Alignment::Right);
        frame.render_widget(hint, input);
    }
    if !front {
        super::browse::render(frame, app, mid);
        super::dialog::render(frame, app, mid);
    }
}

/// 顶部左侧的简短路径（STAGE-131）：把导航栈翻译成「场景 › 菜单 › 背包」这样的文字。
fn nav_path(world: &WorldState) -> String {
    world
        .nav
        .layers()
        .iter()
        .map(|layer| match layer {
            Layer::Page(menu) => MENU_TITLES[*menu]
                .trim_start_matches(|c: char| c.is_ascii_digit())
                .to_string(),
            other => other.label().to_string(),
        })
        .collect::<Vec<_>>()
        .join(" › ")
}

/// 输入行右侧的灰字（一段一组按键）：有内容时是当前命令的用法（STAGE-26），没有内容时是当前层的按键（STAGE-104）。
fn input_hint(world: &WorldState) -> Vec<String> {
    if !world.nav.is(&Layer::Command) && !world.npc_window.as_ref().is_some_and(|n|n.window_id.is_empty()) && !(world.confirm.is_some()&&!world.confirm_native){return key_hint(world);}
    let text = world.input.text();
    if text.is_empty() {
        return key_hint(world);
    }
    let first = text.split_whitespace().next().unwrap_or("");
    vec![match commands::find(first) {
        Some(c) => c.hint(),
        None => "输入 help 查看命令".into(),
    }]
}

/// 从右往左截到放得下（`width` 列以内）；最左边的一段最重要，留到最后。一段都放不下时返回 None。
fn fit_hint(mut keys: Vec<String>, width: usize) -> Option<String> {
    while !keys.is_empty() {
        let text = keys.join(" · ");
        if UnicodeWidthStr::width(text.as_str()) <= width {
            return Some(text);
        }
        keys.pop();
    }
    None
}

#[cfg(test)]
mod tests {
    use super::fit_hint;

    #[test]
    fn hint_drops_from_the_right_until_it_fits() {
        let all: Vec<String> = vec!["1-2 选择".into(), "Esc 关闭".into()];
        assert_eq!(
            fit_hint(all.clone(), 40).as_deref(),
            Some("1-2 选择 · Esc 关闭")
        );
        assert_eq!(
            fit_hint(all.clone(), 10).as_deref(),
            Some("1-2 选择"),
            "右边的先截，最左边的留到最后"
        );
        assert_eq!(fit_hint(all, 5), None, "最左边的一段也放不下就不显示");
    }
}
