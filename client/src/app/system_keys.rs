use super::system::{SystemStep};
use super::{App, Layer, SYSTEM_MENU};
use crossterm::event::{KeyCode, KeyEvent};
use std::time::Instant;

impl App {
    /// 战斗中的控制面板（STAGE-153）：任何玩家都能按 Tab 打开（原来只有观察方和离线快照能开），
    /// 打开期间本地自动出招与自动关结果暂停，关闭后仍开着挂机就按现有延迟重新安排出招。
    pub(super) fn system_battle_key(&mut self, key: KeyEvent) -> bool {
        let Some(w) = self.world.as_mut() else {
            return false;
        };
        if w.battle.is_none() {
            return false;
        }
        if key.code == KeyCode::Tab && !w.system.battle_open {
            w.close_battle_veil(); // 观察模式的菜单盖在战斗上时，先回到战斗画面再开面板（STAGE-182）
            w.system.battle_open = true;
            // 打开时光标落在「自动战斗设置」上：自动战斗中按 Tab 十之八九是去改设置（STAGE-173）；
            // 没开自动战斗时仍落在「自动挂机」上（按 Tab 十之八九是为了开始托管）。
            if w.system.step.is_none() {
                w.system.index = if w.auto_battle { 10 } else { 6 };
            }
            return true;
        }
        if !w.system.battle_open {
            return false;
        }
        if key.code == KeyCode::Esc && w.system.step.is_none() {
            w.system.battle_open = false;
            self.resume_auto_after_panel();
            return true;
        }
        self.system_key_inner(key);
        true
    }

    /// 关掉控制面板：仍开着挂机就按现有延迟重新安排这一次合法出招（不补发、不积压）；
    /// 阶段已经过去（等待、回放、结束）就把旧安排丢掉，等下一个下指令阶段再说。
    fn resume_auto_after_panel(&mut self) {
        let now = Instant::now();
        let delay = self.auto_delay();
        let Some(w) = self.world.as_mut() else {
            return;
        };
        if !w.auto_battle {
            return;
        }
        if let Some(b) = w.battle.as_mut() {
            if matches!(b.phase, super::battle::Phase::Input(_)) && !b.submitting {
                b.schedule_auto(now, delay);
            } else {
                b.auto_due = None;
            }
        }
    }
    pub(super) fn system_key(&mut self, key: KeyEvent) -> bool {
        if self
            .world
            .as_ref()
            .is_none_or(|w| w.menu != SYSTEM_MENU || !w.nav.is(&Layer::Page(SYSTEM_MENU)))
        {
            return false;
        }
        self.system_key_inner(key);
        true
    }
    fn system_key_inner(&mut self, key: KeyEvent) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        let rows=super::system::visible_rows(w);
        let pos=rows.iter().position(|i|*i==w.system.index).unwrap_or(0);
        // 战斗中的控制面板不是页面：数字键不切菜单，按键也不往战斗网格里穿透（STAGE-153）。
        let panel = w.system.battle_open;
        let step = w.system.step.take();
        let Some(mut step) = step else {
            match key.code {
                KeyCode::Char(c @ '1'..='9') if !panel => {
                    self.switch_menu(usize::from(c as u8 - b'1'));
                }
                KeyCode::Up => w.system.index = rows[pos.saturating_sub(1)],
                KeyCode::Down => w.system.index = rows[(pos+1).min(rows.len()-1)],
                KeyCode::Home => w.system.index = 0,
                KeyCode::End => w.system.index = *rows.last().unwrap(),
                KeyCode::Enter => {
                    self.system_enter();
                }
                KeyCode::Esc => {
                    self.leave_page();
                }
                KeyCode::Char('/') => w.nav.push(Layer::Command),
                _ => {}
            }
            return;
        };
        if !matches!(step, SystemStep::Chat(_) | SystemStep::Create { .. } | SystemStep::AutoBattle) {
            if key.code == KeyCode::Char('/') {
                w.system.step = Some(step);
                w.nav.push(Layer::Command);
                return;
            }
            if let KeyCode::Char(c @ '1'..='9') = key.code
                && !panel
            {
                self.switch_menu(usize::from(c as u8 - b'1'));
                return;
            }
        }
        // 设置页自己管 Esc（有草稿改动时先问保存），不能落到下面的“整页退出”。
        if matches!(step, SystemStep::AutoBattle) {
            if let Some(w) = self.world.as_mut() {
                w.system.step = Some(step);
            }
            self.auto_settings_key(key);
            // 编辑器退出（Esc 无改动、或确认框选了保存/放弃）就退回系统行表，别停在空页面。
            if self.world.as_ref().is_some_and(|w| w.auto_edit.is_none())
                && let Some(w) = self.world.as_mut()
            {
                w.system.step = None;
            }
            return;
        }
        if key.code == KeyCode::Esc {
            w.agent.fresh = None;
            if matches!(step, SystemStep::Revoke { .. }) {
                w.system.step = Some(SystemStep::Tokens { index: 0 });
            }
            return;
        }
        match &mut step {
            SystemStep::Chat(input) => {
                if key.code == KeyCode::Enter {
                    let text = input.text().to_string();
                    if !self.session.system_busy() && self.say_to_agent(&text) {
                        return;
                    }
                } else {
                    input.handle_key(&key);
                }
            }
            SystemStep::Create {
                label,
                days,
                focus,
                confirm,
            } => match key.code {
                KeyCode::Tab => *focus = (*focus + 1) % 3,
                KeyCode::BackTab => *focus = (*focus + 2) % 3,
                KeyCode::Left | KeyCode::Right if *focus == 2 => *confirm = !*confirm,
                KeyCode::Enter if *focus < 2 => *focus += 1,
                KeyCode::Enter => {
                    if !*confirm {
                        return;
                    }
                    let label = label.text().to_string();
                    match days.text().parse::<u32>() {
                        Ok(d) => {
                            if self.create_token(&label, Some(d)) {
                                return;
                            }
                        }
                        Err(_) => self.log.error("有效期请输入1–365的整数"),
                    }
                }
                _ => {
                    if *focus == 0 {
                        label.handle_key(&key);
                    } else if *focus == 1 {
                        days.handle_key(&key);
                    }
                }
            },
            SystemStep::Tokens { index } => match key.code {
                KeyCode::Up => *index = index.saturating_sub(1),
                KeyCode::Down => *index = (*index + 1).min(w.agent.tokens.len().saturating_sub(1)),
                KeyCode::Enter => {
                    if let Some(t) = w.agent.tokens.get(*index) {
                        w.system.step = Some(SystemStep::Revoke {
                            id: t.token_id.clone(),
                            label: t.label.clone(),
                            confirm: false,
                        });
                        return;
                    }
                }
                _ => {}
            },
            SystemStep::Revoke { id, confirm, .. } => match key.code {
                KeyCode::Left | KeyCode::Right | KeyCode::Tab | KeyCode::BackTab => {
                    *confirm = !*confirm
                }
                KeyCode::Enter => {
                    if !*confirm {
                        self.system_step(SystemStep::Tokens { index: 0 });
                        return;
                    }
                    if self.revoke_token_id(id) {
                        self.system_step(SystemStep::Tokens { index: 0 });
                        return;
                    }
                }
                _ => {}
            },
            SystemStep::Fresh => {
                if key.code == KeyCode::Enter {
                    w.agent.fresh = None;
                    return;
                }
            }
            _ => {}
        }
        if let Some(w) = self.world.as_mut() {
            w.system.step = Some(step);
        }
    }
}
