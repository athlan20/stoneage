//! STAGE-148/152 scene navigation; modal key priority stays in world_nav.
use super::{
    App, Layer,
    scene_state::{EncounterChoice, EncounterPrompt, LoadState, SceneColumn},
};
use crossterm::event::{KeyCode, KeyEvent};

/// scene_enter 的意图：先把借用收干净，再执行需要 &mut self 的动作。
enum Enter {
    None,
    Talk(String),
    Exit,
    Travel,
    Catalog,
    Home,
    OpenPrompt,
    OpenPlayers,
    OpenChat,
    StopEncounter,
    Notice(&'static str, String),
}

impl App {
    pub(super) fn scene_key(&mut self, key: KeyEvent) -> bool {
        // STAGE-273：附近玩家的动作面板同样模态。
        if self.players_prompt_key(key) {
            return true;
        }
        // 动作面板是模态的：先吃掉所有按键，Esc 只是关面板。
        let (handled, submit) = self.scene_prompt_key(key);
        if handled {
            if let Some((id, mode)) = submit {
                self.start_encounter(id, mode);
            }
            return true;
        }
        let Some(w) = self.world.as_mut() else {
            return false;
        };
        if w.scene.details {
            match key.code {
                KeyCode::Esc | KeyCode::Tab => w.scene.details = false,
                KeyCode::Up => w.scene.detail_scroll = w.scene.detail_scroll.saturating_sub(1),
                KeyCode::Down => w.scene.detail_scroll = w.scene.detail_scroll.saturating_add(1),
                KeyCode::PageUp => {
                    w.scene.detail_scroll = w
                        .scene
                        .detail_scroll
                        .saturating_sub(w.scene.rows.max(1) as u16)
                }
                KeyCode::PageDown => {
                    w.scene.detail_scroll = w
                        .scene
                        .detail_scroll
                        .saturating_add(w.scene.rows.max(1) as u16)
                }
                KeyCode::Home => w.scene.detail_scroll = 0,
                _ => {}
            }
            return true;
        }
        if w.scene.column == SceneColumn::Exit
            && matches!(
                key.code,
                KeyCode::Up
                    | KeyCode::Down
                    | KeyCode::Home
                    | KeyCode::End
                    | KeyCode::PageUp
                    | KeyCode::PageDown
                    | KeyCode::Enter
            )
        {
            w.scene.left_touched = true;
        }
        let column = w.scene.column;
        let total = w.scene.len(w.npcs.len());
        let index = match column {
            SceneColumn::Exit => &mut w.scene_index,
            SceneColumn::Npc => &mut w.scene.npc_index,
            SceneColumn::Players => &mut w.scene.players.index,
            SceneColumn::Encounter => &mut w.scene.area_index,
        };
        let last = total.saturating_sub(1);
        let step = w.scene.rows.max(1);
        let mut load_areas = false;
        match key.code {
            KeyCode::Esc => {
                w.nav.push(Layer::Menu);
                w.menu_index = 0;
            }
            KeyCode::Char('/') => w.nav.push(Layer::Command),
            KeyCode::Left => {
                w.scene.column = column.left();
                // 遇敌列进场景时已预加载（STAGE-160）；这里只兜底 area_query 仍为 0 的旧路径。
                load_areas = w.scene.column.is_encounter() && w.scene.area_query == 0;
            }
            KeyCode::Right => {
                w.scene.column = column.right();
                load_areas = w.scene.column.is_encounter() && w.scene.area_query == 0;
            }
            KeyCode::Up => *index = index.saturating_sub(1),
            KeyCode::Down => *index = (*index + 1).min(last),
            KeyCode::PageUp => *index = index.saturating_sub(step),
            KeyCode::PageDown => *index = (*index + step).min(last),
            KeyCode::Home => *index = 0,
            KeyCode::End => *index = last,
            KeyCode::Tab => {
                w.scene.details = true;
                w.scene.detail_scroll = 0;
            }
            KeyCode::F(5) => {
                self.request_scene_list(column);
            }
            KeyCode::Enter => self.scene_enter(),
            _ => {}
        }
        if load_areas {
            self.request_scene_list(SceneColumn::Encounter);
        }
        true
    }

    /// 动作面板的按键。返回（是否已处理, 要提交的（区域 ID, 模式））。
    fn scene_prompt_key(&mut self, key: KeyEvent) -> (bool, Option<(String, &'static str)>) {
        let Some(w) = self.world.as_mut() else {
            return (false, None);
        };
        let Some(prompt) = w.scene.prompt else {
            return (false, None);
        };
        match key.code {
            KeyCode::Esc => {
                w.scene.prompt = None;
                (true, None)
            }
            KeyCode::Up => {
                w.scene.prompt = Some(EncounterPrompt {
                    choice: prompt.choice.up(),
                });
                (true, None)
            }
            KeyCode::Down => {
                w.scene.prompt = Some(EncounterPrompt {
                    choice: prompt.choice.down(),
                });
                (true, None)
            }
            KeyCode::Enter => {
                let id = w.scene.selected_area().map(|a| a.id.clone());
                w.scene.prompt = None;
                (true, id.zip(prompt.choice.mode()))
            }
            // 面板打开时其它键不落到页面导航上。
            _ => (true, None),
        }
    }

    fn scene_enter(&mut self) {
        let act = {
            let Some(w) = self.world.as_ref() else {
                return;
            };
            if w.scene.using_exit || w.scene.starting_encounter || self.session.npc_busy() {
                return;
            }
            if self.session.scene_operation_busy() {
                return;
            }
            match w.scene.column {
                SceneColumn::Npc => {
                    if w.scene.npc_load != LoadState::Ready {
                        Enter::Notice("npc", "NPC 资料尚未就绪，按 F5 重试".into())
                    } else if let Some(npc) = w.npcs.get(w.scene.npc_index) {
                        Enter::Talk(npc.id.to_string())
                    } else {
                        Enter::None
                    }
                }
                // STAGE-273：资料没就绪时不开面板，按 F5 重试（同 NPC、遇敌列）。
                SceneColumn::Players => {
                    // STAGE-278：列底部固定的「聊天…」行（编号等于玩家数），不论资料是否就绪都能打开。
                    if w.scene.players.index >= w.scene.players.len() {
                        Enter::OpenChat
                    } else if w.scene.players.load != LoadState::Ready {
                        Enter::Notice("players", "玩家资料尚未就绪，按 F5 重试".into())
                    } else if w.scene.players.selected().is_some() {
                        Enter::OpenPlayers
                    } else {
                        Enter::None
                    }
                }
                SceneColumn::Encounter => {
                    if w.scene.stop_selected() {
                        Enter::StopEncounter
                    } else if w.scene.area_load != LoadState::Ready {
                        Enter::Notice("area", "遇敌资料尚未就绪，按 F5 重试".into())
                    } else if let Some(a) = w.scene.areas.get(w.scene.area_index) {
                        if a.available {
                            Enter::OpenPrompt
                        } else {
                            Enter::Notice(
                                "blocked",
                                a.unavailable_reason
                                    .clone()
                                    .unwrap_or_else(|| "这个区域当前不能遇敌".into()),
                            )
                        }
                    } else {
                        Enter::None
                    }
                }
                SceneColumn::Exit => {
                    let count = w.scene.exits.len();
                    match w.scene_index.checked_sub(count) {
                        None => Enter::Exit,
                        Some(0) => Enter::Travel,
                        Some(1) => Enter::Catalog,
                        Some(_) => Enter::Home,
                    }
                }
            }
        };
        // 队员（真人队跟随中）的换图、遇敌类入口灰显并说明原因（STAGE-274，docs/ui.md）。
        let blocked = self
            .world
            .as_ref()
            .and_then(super::party_rules::follower_reason)
            .filter(|_| matches!(act, Enter::Exit | Enter::Travel | Enter::Home | Enter::OpenPrompt | Enter::StopEncounter));
        let act = match blocked {
            Some(why) => Enter::Notice("party", why.to_string()),
            None => act,
        };
        match act {
            Enter::Talk(id) => {
                self.talk_command(&id);
            }
            Enter::Exit => {
                self.use_scene_exit();
            }
            Enter::Travel => {
                // STAGE-161：和游戏菜单里的「快捷传送」是同一个目录。
                self.open_travel_page();
            }
            Enter::Catalog => {
                self.scene_open_catalog();
            }
            Enter::Home => {
                self.scene_go_home();
            }
            Enter::StopEncounter => {
                self.stop_encounter();
            }
            Enter::OpenPrompt => {
                if let Some(w) = self.world.as_mut() {
                    w.scene.prompt = Some(EncounterPrompt {
                        choice: EncounterChoice::Once, // 默认「遇敌 1 次」
                    });
                }
            }
            Enter::OpenChat => self.open_chat(Some(crate::proto::ChatChannel::Map)),
            Enter::OpenPlayers => {
                if let Some(w) = self.world.as_mut() {
                    w.scene.players.prompt = Some(0); // 默认第一项「查看资料」
                }
            }
            Enter::Notice(kind, text) => {
                let _ = kind;
                self.log.error(text);
            }
            Enter::None => {}
        }
    }
}
