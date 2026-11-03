use crossterm::event::{KeyCode,KeyEvent};
use super::{App,Layer};
use super::world::GAME_MENU;

impl App {
    /// 导航栈顶那一层的按键（STAGE-131）：场景、游戏菜单、命令栏在这里处理；
    /// 功能页返回 false，交给页面选择与输入行。返回 true 表示已处理。
    /// 有模态层（战斗、对话窗口、确认）时不接管：它们的按键归自己，字母照旧进输入行。
    pub(super) fn nav_key(&mut self, key: KeyEvent) -> bool {
        let Some(world) = self.world.as_ref() else {
            return false;
        };
        if world.battle_front() || world.npc_window.is_some() || world.confirm.is_some() {
            return false;
        }
        // 命令栏压在最上面时先由它接手（背包动作层里按 `/` 打开命令栏也是这条路径）。
        if world.nav.is(&Layer::Command) {
            return self.command_layer_key(key);
        }
        if self.system_key(key) { return true; }
        // 背包对象动作流程（STAGE-132）在背包页之上接管按键。
        if self.browse_key(key) { return true; }
        let Some(world) = self.world.as_ref() else { return false; };
        if world.bag_step.is_some() {
            return self.bag_action_key(key);
        }
        // 宠物动作流程与角色加点表单（STAGE-133）。
        if world.pet_step.is_some() {
            return self.pet_action_key(key);
        }
        if world.char_step.is_some() {
            return self.char_action_key(key);
        }
        // 社交页的动作菜单与成员详情（STAGE-162）。
        if world.party_step.is_some() {
            return self.party_action_key(key);
        }
        // 场景页的传送列表与查找表单（STAGE-134）。
        if world.scene_step.is_some() {
            return self.scene_step_key(key);
        }
        match world.nav.top().clone() {
            Layer::Scene => self.scene_key(key),
            Layer::Menu => self.menu_layer_key(key),
            Layer::Command => self.command_layer_key(key),
            _ => false,
        }
    }

    /// 游戏菜单（STAGE-131）：`↑↓` 选项，Enter 进入功能页，Esc 回场景，`/` 打开命令栏。
    fn menu_layer_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => {
                if let Some(world) = self.world.as_mut() {
                    world.nav.pop();
                }
                true
            }
            KeyCode::Char('/') => {
                if let Some(world) = self.world.as_mut() {
                    world.nav.push(Layer::Command);
                }
                true
            }
            KeyCode::Up => {
                self.move_menu(false);
                true
            }
            KeyCode::Down => {
                self.move_menu(true);
                true
            }
            KeyCode::Home => {
                if let Some(world) = self.world.as_mut() {
                    world.menu_index = 0;
                }
                true
            }
            KeyCode::End => {
                if let Some(world) = self.world.as_mut() {
                    world.menu_index = GAME_MENU.len() - 1;
                }
                true
            }
            KeyCode::Enter => {
                let menu = self
                    .world
                    .as_ref()
                    .map(|w| GAME_MENU[w.menu_index.min(GAME_MENU.len() - 1)].1);
                if let Some(menu) = menu {
                    self.switch_menu(menu);
                }
                true
            }
            KeyCode::Char(_) => true,
            _ => false,
        }
    }

    /// 游戏菜单的选中行上下移动。
    fn move_menu(&mut self, down: bool) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        let last = GAME_MENU.len() - 1;
        let cur = world.menu_index.min(last);
        world.menu_index = if down {
            (cur + 1).min(last)
        } else {
            cur.saturating_sub(1)
        };
    }

    /// 命令栏（STAGE-131）：Enter 执行输入行，Esc 收起并保留草稿，其余键交给输入行。
    pub(super) fn command_layer_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => {
                if let Some(world) = self.world.as_mut() {
                    world.nav.pop();
                }
                true
            }
            KeyCode::Enter => {
                if self.world.as_ref().is_some_and(|w|w.battle_front()) && self.battle_bar_enter(){return true;}
                if self.submit_input() {
                    if let Some(world) = self.world.as_mut() {
                        world.nav.close_command();
                    }
                }
                true
            }
            KeyCode::Tab => { self.complete_input(); true }
            KeyCode::Up => {
                if let Some(w) = self.world.as_mut() { let text=w.input.text().to_string(); if let Some(line)=w.history.prev(&text){w.input.set(&line);} }
                true
            }
            KeyCode::Down => {
                if let Some(w) = self.world.as_mut() { if let Some(line)=w.history.next(){w.input.set(&line);} }
                true
            }
            _ => {
                if let Some(world) = self.world.as_mut() {
                    world.input.handle_key(&key);
                }
                true
            }
        }
    }

    /// 功能页 Esc 退一层（STAGE-131）：回到游戏菜单；栈里没有菜单层时（从场景入口直接进来的）落到场景。
    pub(super) fn leave_page(&mut self) {
        if let Some(world) = self.world.as_mut() {
            world.nav.pop();
        }
    }

}
