//! STAGE-139：系统列表与独立表单。
use crate::app::{
    App,
    agent_mcp,
    system::{ROWS, SystemStep},
};
use ratatui::{
    Frame,
    layout::Rect,
    style::{Color, Style},
    widgets::{Block, List, ListItem, ListState, Paragraph, Wrap},
};
pub fn render(frame: &mut Frame, app: &App, area: Rect) {
    let Some(w) = app.world.as_ref() else {
        return;
    };
    let online = if w.entry.agent_online {
        "在线"
    } else {
        "离线"
    };
    let controller = if w.observing() {
        "agent 操控中"
    } else {
        "玩家操控"
    };
    // 战斗中的控制面板（STAGE-153）：标题写清是战斗控制，底部提示怎么关。
    let in_battle = w.battle.is_some();
    let title = if in_battle { format!("战斗控制 · agent {online} · {controller}") } else { format!("系统 · agent {online} · {controller}") };
    let hint = if in_battle {
        "↑↓ 选择 · Enter 执行 · Esc 关闭面板 · / 命令"
    } else {
        "↑↓ 选择 · Enter 确认 · Esc 返回 · / 命令"
    };
    let block = Block::bordered().title(title).title_bottom(hint);
    let inner = block.inner(area);
    frame.render_widget(block, area);
    let Some(step) = w.system.step.as_ref() else {
        // 停止遇敌的状态提示占最下面一行：关闭中、失败原因都写在这里。
        let status = if let Some(err) = w.halt.error.as_deref() {
            Some(format!("停止自动遇敌失败：{err}（Enter 重试，↑↓ 可先看别的项）"))
        } else if w.halt.busy() {
            Some("正在关闭自动遇敌…".to_string())
        } else {
            None
        };
        let list_area = match &status {
            Some(_) if inner.height > 1 => Rect { height: inner.height - 1, ..inner },
            _ => inner,
        };
        let rows=crate::app::system::visible_rows(w);
        let items: Vec<ListItem> = rows.iter().map(|&i| {
                // 挂机与遇敌是动态项（STAGE-153）：名字固定，状态写「开 / 关」，和「Unicode：开」一个写法；
                // Enter 仍是切换，动作名不再和状态拼在一起（「停止：开启」读起来像两件事）。
                let (s, suffix) = match i {
                    6 => ("自动挂机", format!("：{}", if w.auto_battle { "开" } else { "关" })),
                    7 => (
                        if in_battle { "自动遇敌" } else { "连续遇敌" },
                        format!(
                            "：{}",
                            if w.halt.busy() {
                                "正在关闭"
                            } else if w.entry.encounter {
                                "开"
                            } else {
                                "关"
                            }
                        ),
                    ),
                    8 => (ROWS[i], if app.unicode { "：开" } else { "：关" }.to_string()),
                    9 => (ROWS[i], format!("：{}", app.playback_label())),
                    _ => (ROWS[i], String::new()),
                };
                let reason = if i <= 3 && !w.entry.agent_online {
                    "（agent 不在线）"
                } else if matches!(i, 6 | 7) && w.observing() {
                    "（需收回控制权）"
                } else if i == 6 && w.halt.busy() {
                    "（正在关闭自动遇敌）"
                } else {
                    ""
                };
                ListItem::new(format!("{s}{suffix}{reason}"))
            })
            .collect();
        let mut state = ListState::default().with_selected(rows.iter().position(|i|*i==w.system.index));
        frame.render_stateful_widget(
            List::new(items)
                .highlight_symbol("› ")
                .highlight_style(Style::default().fg(Color::Yellow)),
            list_area,
            &mut state,
        );
        if let Some(status) = status {
            let at = Rect { y: inner.y + inner.height - 1, height: 1, ..inner };
            let style = if w.halt.error.is_some() { Style::default().fg(Color::Red) } else { Style::default().fg(Color::Yellow) };
            frame.render_widget(Paragraph::new(status).style(style), at);
        }
        return;
    };
    // 自动战斗设置（STAGE-173）：整页五栏编辑器，不是一行文字。
    if matches!(step, SystemStep::AutoBattle) {
        crate::ui::autosettings::render(frame, app, inner);
        return;
    }
    let text=match step {
        SystemStep::Chat(input)=>format!("私聊 agent（最多200字，Enter 发送）\n\n{}",input.text()),
        SystemStep::Create{label,days,focus,confirm}=>format!("生成 agent 令牌（最多5个，Tab 切换）\n\n{}备注（最多32字）：{}\n{}有效期（1–365天）：{}\n\n{}{}  {}",if *focus==0{"› "}else{"  "},label.text(),if *focus==1{"› "}else{"  "},days.text(),if *focus==2{"› "}else{"  "},if !confirm{"[取消]"}else{" 取消 "},if *confirm{"[生成]"}else{" 生成 "}),
        SystemStep::Tokens{index}=>{
            let mut lines=vec!["agent 令牌（Enter 吊销，Esc 返回）".to_string()];
            if w.agent.tokens.is_empty(){lines.push("还没有令牌".into());}
            for (i,t) in w.agent.tokens.iter().enumerate(){lines.push(format!("{}{} {} 到期 {} 最后使用 {}",if i==*index{"› "}else{"  "},t.token_id,t.label,t.expires_at,t.last_used_at.as_deref().unwrap_or("从未")));}lines.join("\n")
        }
        SystemStep::Revoke{label,confirm,..}=>format!("吊销令牌：{label}\n\n吊销后不能再用它登录。\n\n{}  {}",if !confirm{"[取消]"}else{" 取消 "},if *confirm{"[吊销]"}else{" 吊销 "}),
        SystemStep::Fresh=>w.agent.fresh.as_ref().map(|t|format!("新令牌（只显示这一次，请立即交给 agent）\n\nagent 令牌 {}  {}\n到期 {}  最后使用 从未\n\n{}\n\nClaude Code 接入（MCP，先装好 stoneage-mcp，再在终端执行这一行）：\n{}\n\nEnter / Esc 返回后清除原文",t.token_id,t.label,t.expires_at,t.token,agent_mcp::mcp_add_command(&app.session.addr,&t.token))).unwrap_or("令牌原文已清除".into()),
        SystemStep::Info=>format!("连接信息\n服务端 {}\n账号 {}\n角色 {}\nagent {online} · {controller}\n显示 {} · 默认回放 {}",app.session.addr,app.login.account.text(),w.entry.character.name,if app.unicode{"Unicode"}else{"ASCII"},app.playback_label()),
        SystemStep::Help=>"Esc 逐层返回；方向键选择对象与动作，Enter 确认。\n/ 打开命令栏；表单内斜杠和空格是文字。\n战斗：回放速度与「跳过」在系统页「默认回放」里设，战斗中没有快捷键；任何玩家都能按 Tab 打开战斗控制。\n自动挂机只负责出招，不会打开遇敌；主动停止挂机同时关闭自动遇敌（单向联动）。\n自动战斗设置（系统页与 Tab 面板同一份）：Tab 换栏、↑↓ 选、←→ 调、Enter 执行。\nCtrl+C 显示退出确认。".into(),
        SystemStep::AutoBattle=>String::new(),
    };
    frame.render_widget(Paragraph::new(text).wrap(Wrap { trim: false }), inner);
}
