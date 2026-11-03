use crossterm::event::{KeyCode,KeyEvent};
use super::{App,Layer};
use super::{commands,msgview,scroll};
use super::world::{BAG_MENU,MESSAGES_MENU};

impl App {

    pub(super) fn world_key(&mut self, key: KeyEvent) {
        self.world_key_inner(key);
        // 观察模式盖在战斗上的菜单退回场景，就是回到战斗画面（STAGE-182）。
        // 战斗已摘掉而盖子还在时，玩家停在菜单里不踢回场景，只在退回场景根时收起（STAGE-284）。
        if self
            .world
            .as_ref()
            .is_some_and(|w| w.battle_veil && w.nav.depth() <= 1)
        {
            self.leave_battle_veil();
        }
    }

    fn world_key_inner(&mut self, key: KeyEvent) {
        // Backspace 备用返回键（STAGE-172）：能返回的层里等同 Esc，文字输入焦点里照旧删字。
        let key = self.back_alias(key);
        if self
            .world
            .as_ref()
            .is_some_and(|w| w.nav.is(&Layer::Command))
        {
            self.command_layer_key(key);
            return;
        }
        // 聊天输入框（STAGE-278）独占按键：战斗中也能说话，Esc 只收起输入框。
        if self.world.as_ref().is_some_and(|w| w.nav.is(&Layer::Chat)) {
            self.chat_layer_key(key);
            return;
        }
        // 自动战斗中的快捷键（STAGE-173）：8 停止自动战斗、7 只停自动遇敌。
        // 不分阶段（下指令 / 结算 / 等待都认），设置页、Tab 面板开着也认；只有正在打字时才是普通字符。
        if matches!(key.code, KeyCode::Char('7') | KeyCode::Char('8')) && self.auto_hotkey(key.code) {
            return;
        }
        if self.system_battle_key(key) { return; }
        // 确认层优先级最高（docs/interaction.md「按键分层」）：输入行为空时 Esc 取消确认。
        if self.confirm_layer_key(key) {
            return;
        }
        // 自动战斗设置（STAGE-173）：设置页开着就占住按键（含 Tab 换栏），战斗在后台照常跑。
        // 必须排在战斗按键之前——否则托管锁定的那层会把方向键/Enter 全吃掉，设置页没法操作。
        if self.world.as_ref().is_some_and(|w| w.auto_edit.is_some()) {
            self.auto_settings_key(key);
            return;
        }
        // 战斗中先交给战斗的按键处理：数字键选指令和目标，不切换菜单（STAGE-38）。
        if self.world.as_ref().is_some_and(|w| w.battle_front()) && self.battle_key(key) {
            return;
        }
        // 对话窗口打开时数字键和 Esc 用来选选项、关窗（STAGE-69，docs/ui.md「服务端驱动的界面」）。
        if self.dialog_key(key) {
            return;
        }
        // 组队请求弹窗（STAGE-274）：默认拒绝，Esc 稍后处理，其余按键照常。
        // 交易窗口（STAGE-295）：开着时占住全部按键。
        if self.trade_key(key) {
            return;
        }
        if self.party_popup_key(key) {
            return;
        }
        // 导航栈顶的层先接手（STAGE-131）：场景、游戏菜单、命令栏；功能页落到下面的页面选择和输入行。
        if self.nav_key(key) {
            return;
        }
        // 没有模态层时，输入行为空的方向键切页、移动页面的选中项，Enter 执行选中项的命令（STAGE-113）。
        if self.page_select_key(key) {
            return;
        }
        let Some(world) = self.world.as_mut() else {
            return;
        };
        // 接受文字的对话窗口（暗号）和等待确认时，数字是文字，不切菜单（确认见 confirm.rs）。
        let text_window = world.npc_window.as_ref().is_some_and(|w|w.window_id.is_empty()) || (world.confirm.is_some()&&!world.confirm_native);
        // 功能页里输入行为空时 `/` 打开命令栏（STAGE-131）：斜杠不写进输入行。
        if !text_window && key.code == KeyCode::Char('/') {
            world.nav.push(Layer::Command);
            return;
        }
        match key.code {
            KeyCode::Char(c @ '1'..='9')
                if world.input.is_empty() && !text_window && world.confirm.is_none() =>
            {
                self.switch_menu(usize::from(c as u8 - b'1'));
            }
            // 输入行为空时 0 不起作用（docs/interaction.md 按键表）：吞掉，不写进输入行。
            KeyCode::Char('0')
                if world.input.is_empty() && !text_window && world.confirm.is_none() => {}
            KeyCode::Esc => {
                // 输入行有字时 Esc 只清空；空行且没有模态层时退一步：
                // 页内详情先关（任务攻略、百科），没有就往导航栈上一层退（STAGE-131）。
                // 战斗中 / 有弹窗或确认时不动导航栈，那些层的 Esc 归自己。
                let idle = !text_window || world.input.is_empty();
                let modal =
                    world.battle_front() || world.npc_window.is_some() || world.confirm.is_some();
                if text_window {world.input.clear();world.history.reset();}
                if idle && !modal && !self.page_back() {
                    self.leave_page();
                }
            }
            KeyCode::Enter if text_window => {
                self.submit_input();
            }
            // Tab 补全命令和参数（STAGE-26）；输入行为空时在页面的栏之间切换（page_keys.rs，STAGE-113）。
            KeyCode::Tab if text_window && !world.input.is_empty() => self.complete_input(),
            // ↑↓ 翻看历史命令：输入行有字时按前缀翻；空行且页面没有可选的列表时也翻（有列表的页在 page_keys.rs 先接手）。
            KeyCode::Up if text_window => {
                let current = world.input.text().to_string();
                if let Some(line) = world.history.prev(&current) {
                    world.input.set(&line);
                }
            }
            KeyCode::Down if text_window => {
                if let Some(line) = world.history.next() {
                    world.input.set(&line);
                }
            }
            // PgUp/PgDn：「消息」页翻消息（`docs/ui.md` 消息页），「背包」页滚动物品表（背包页 A）。
            KeyCode::PageUp if world.menu == MESSAGES_MENU => {
                msgview::page(&self.log, &mut world.msg_view, false);
            }
            KeyCode::PageDown if world.menu == MESSAGES_MENU => {
                msgview::page(&self.log, &mut world.msg_view, true);
            }
            KeyCode::PageUp if world.menu == BAG_MENU => scroll::page_bag(world, false),
            KeyCode::PageDown if world.menu == BAG_MENU => scroll::page_bag(world, true),
            // Home/End 在输入行为空、当前页可以滚动时才作滚动，有内容时仍是移动光标（STAGE-26、STAGE-103）。
            KeyCode::Home | KeyCode::End
                if world.input.is_empty() && scroll::scrolls(world.menu) =>
            {
                self.scroll_edge(key.code == KeyCode::End);
            }
            _ if text_window => {
                world.input.handle_key(&key);
            }
            _=>{}
        }
    }

    /// 提交输入行（功能页的 Enter 和命令栏的 Enter 共用）：行首空白不算命令的一部分，
    /// 行尾空白留给 `reply`（暗号要逐字相同）；非空就记历史并执行。
    /// 返回这条命令是不是已知命令 —— 命令栏据此决定执行后收不收起（STAGE-131：错误留在栏内）。
    pub(super) fn submit_input(&mut self) -> bool {
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        let raw = world.input.text().trim_start().to_string();
        let line = raw.trim_end().to_string();
        if let Some(world) = self.world.as_mut() {
            world.input.clear();
        }
        if line.is_empty() {
            return false;
        }
        if let Some(world) = self.world.as_mut() {
            world.history.push(&line);
        }
        let cmd = line
            .split_once(char::is_whitespace)
            .map_or(line.as_str(), |(name, _)| name);
        let known = commands::find(cmd).is_some() || commands::battle_find(cmd).is_some();
        let before = self.log.all().iter().filter(|m| m.kind == super::Kind::Error).count();
        self.run_command(&line, &raw, true);
        known && self.log.all().iter().filter(|m| m.kind == super::Kind::Error).count() <= before
    }

}
