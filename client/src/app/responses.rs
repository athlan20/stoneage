//! 认证、世界查询及操作响应；传输生命周期见 session.rs。
use super::session::{ConnState, Pending, format_wait};
use super::magic_use::magic_use_error_hint;
use super::scene_state::SceneColumn;
use super::{App, Screen, WorldState};
use crate::proto::{CharacterList, HelloInfo, ProtoError, WorldEntry};
use serde_json::{Value, json};
impl App {
    pub(super) fn on_ok(&mut self, pending: Pending, data: Value) {
        let refresh_bit = pending.refresh_bit();
        match pending {
            Pending::Hello => {
                let info: HelloInfo = serde_json::from_value(data).unwrap_or_default();
                self.on_latest_client(info.latest_client.as_deref());
                self.session.hello = Some(info);
                self.session.state = ConnState::Ready;
                if self.screen == Screen::Register {
                    if self.register_enabled() {
                        self.log.system("已连接服务端");
                    } else {
                        self.log.error("服务端未开放注册");
                    }
                }
                if let Some(action) = self.session.queued.take() {
                    self.dispatch(action);
                }
            }
            Pending::Login { account } => {
                self.login.account.set(&account);
                self.login.password.clear();
                self.log.system(format!("登录成功：{account}"));
                self.send("characters", json!({}), Pending::Characters);
            }
            Pending::Register { account } => {
                self.login.account.set(&account);
                self.login.password.clear();
                self.register.password.clear();
                self.register.confirm.clear();
                self.screen = Screen::Login;
                self.login.focus = super::LoginFocus::Password;
                self.log.system(format!("注册成功：{account}，请登录"));
            }
            Pending::Characters => {
                let list: CharacterList = serde_json::from_value(data).unwrap_or_default();
                self.on_characters(list);
            }
            Pending::Create => {
                self.log.system("角色已创建");
                self.send("characters", json!({}), Pending::Characters);
            }
            Pending::Delete => {
                self.log.system("角色已删除");
                self.send("characters", json!({}), Pending::Characters);
            }
            Pending::Enter => {
                let mut entry: WorldEntry = serde_json::from_value(data).unwrap_or_default();
                self.log
                    .system(format!("进入游戏：{}", entry.character.name));
                // 进入时正在战斗（断线重连之类）：战场在进入之后补上（STAGE-37）。
                let battle = entry.battle.take();
                let staging = self.stage_reconnect(&entry.character.name, battle.is_some());
                self.world = Some(WorldState::new(entry));
                self.world.as_mut().unwrap().online = !staging;
                // 自动战斗方案按「服务器 + 角色」打开（STAGE-173 M15）：切角色不串、重启还在。
                let server = self.session.addr.clone();
                let character = self
                    .world
                    .as_ref()
                    .map(|w| w.entry.character.name.clone())
                    .unwrap_or_default();
                self.load_auto_store(&server, &character);
                self.screen = if staging {
                    Screen::Characters
                } else {
                    Screen::World
                };
                self.send("places", json!({}), Pending::Places);
                self.send("maps", json!({}), Pending::Maps);
                // 背包进世界后取一次，之后靠 inventory_changed 推送（STAGE-28）。
                self.send("inventory", json!({}), Pending::Inventory);
                // 宠物同样进世界后取一次，之后靠 pets_changed 推送（STAGE-31）。
                self.send("pets", json!({}), Pending::Pets);
                // 任务进度同样进世界后取一次，之后靠 quests_changed 推送（STAGE-86）。
                self.send("quests", json!({}), Pending::Quests);
                // 本地图 NPC 进世界后取一次，之后换地图时重取（STAGE-68，docs/protocol.md 5.32）。
                self.refresh_scene();
                if let Some(snap) = battle {
                    self.resume_battle(snap);
                    if staging {
                        self.send("battle", json!({}), Pending::BattleQuery);
                    }
                }
            }
            Pending::Leave => {
                self.reconnect = None;
                self.world = None;
                self.send("characters", json!({}), Pending::Characters);
            }
            Pending::Status => self.on_status(data),
            Pending::Places => self.on_places(data),
            Pending::Maps => self.on_maps(data),
            Pending::Inventory => self.on_inventory(data),
            Pending::Pets => self.on_pets(data),
            Pending::Quests => self.on_quests(data),
            Pending::Npcs(c) => {if !self.scene_list_ok(c,SceneColumn::Npc,data){return}},
            Pending::Exits(c) => {if !self.scene_list_ok(c,SceneColumn::Exit,data){return}},
            Pending::EncounterAreas(c) => {if !self.scene_list_ok(c,SceneColumn::Encounter,data){return}},
            Pending::Players(c) => {if !self.scene_list_ok(c,SceneColumn::Players,data){return}},
            Pending::EncounterStart => {if let Some(w)=self.world.as_mut(){w.scene.starting_encounter=false;}},
            // 停止遇敌（场景页、系统页、停止挂机联动）走同一个状态机（STAGE-153）：回包里带着权威状态。
            Pending::EncounterStop => self.on_encounter_ok(Some(false), data),
            Pending::UseExit => {if let Some(w)=self.world.as_mut(){w.scene.using_exit=false;}},
            Pending::ItemOp(op) => self.on_item_op(op, &data),
            Pending::GoldDrop { amount } => self.on_gold_drop(amount, &data),
            Pending::ItemInlay(op) => self.on_inlay_ok(op, &data),
            Pending::AutoPost(op) => self.on_post_ok(op, &data),
            Pending::QuestNpcs { no, state, query } => self.on_quest_npcs(no, &state, query, data),
            Pending::Talk => {
                if let Some(w) = self.world.as_mut() {
                    w.browse.quest_visit = None;
                }
            }
            Pending::NpcChoose | Pending::NpcReply | Pending::NpcClose => {}
            Pending::Warp(name) => {
                self.log.system(format!("已传送到 {name}"));
                // 快捷传送页发起的传送成功后回场景页（STAGE-161）：玩家在那里看到服务端确认的位置。
                self.leave_travel_page();
            }
            Pending::PetCatalog => self.on_pet_catalog(data),
            Pending::PetStart { map, mode } => self.on_pet_start(&map, &mode),
            Pending::LevelingCatalog { segment } => self.on_leveling_catalog(data, segment),
            Pending::LevelingStart { map, mode } => self.on_leveling_start(&map, &mode),
            Pending::NpcFind(name) => self.on_npc_find(&name, data),
            Pending::NpcList { query, offset } => self.on_npc_list(query, offset, data),
            Pending::NpcWarp { npc, map_name } => {
                self.log.system(format!("已传送到 {map_name}"));
                self.open_npc(npc);
            }
            Pending::AddPoint { stat, points } => self.on_add_point(&stat, points),
            Pending::PetOp(op) => self.on_pet_op(op, &data),
            Pending::MagicUse => {} // 变身的提示由服务端推送（notice），响应没有内容（STAGE-203）
            Pending::DuelChallenge => {} // 开战由 battle_start 推送接管（STAGE-307）
            Pending::PartyOp(op) => self.on_party_op_ok(op, data),
            Pending::PartyPlayers(op) => self.on_players_op_ok(op, data),
            Pending::Trade(op) => self.on_trade_ok(op, data),
            Pending::SocialSettings { field, value } => self.on_social_ok(field, value, data),
            Pending::KickAgent => self.log.system("已让 agent 下线"),
            Pending::TokenCreate => self.on_token_created(data),
            Pending::TokenList => self.on_token_list(data),
            Pending::TokenRevoke => {
                self.log.system("令牌已吊销");
                self.request_tokens();
            }
            // 结果通过推送告诉玩家（control_changed、position_changed、agent_message），响应本身没有内容。
            Pending::AgentSay(text) => self.log.agent(format!("你: {text}")),
            Pending::Chat { text, .. } => self.on_chat_ok(&text),
            Pending::BattleCommand { turn } => {
                if self
                    .world
                    .as_ref()
                    .and_then(|w| w.battle.as_ref())
                    .is_some_and(|b| b.turn == turn)
                {
                    self.on_battle_command_ok(data);
                }
            }
            Pending::BattleClone { turn, who, target } => {
                if let Some(b) = self.world.as_mut().and_then(|w| w.battle.as_mut()).filter(|b| b.turn == turn) {
                    b.on_clone_accepted(who, target);
                }
                self.flush_battle_log();
            }
            Pending::BattleQuery => self.on_battle_query(data),
            Pending::Encounter { set } => self.on_encounter_ok(set, data),
            Pending::BattleAck => {}
            Pending::Turn | Pending::Take | Pending::Release | Pending::Ping => {}
        }
        self.refreshed(refresh_bit);
    }

    pub(super) fn on_error(&mut self, pending: Pending, err: ProtoError) {
        // STAGE-299：精工的失败文字（镶满时带已镶件数）单独处理，黄色。
        if matches!(pending, Pending::ItemInlay(_)) {
            return self.on_inlay_error(&err);
        }
        if let Pending::Chat { channel, .. } = &pending {
            self.on_chat_failed(*channel, &err);
            return;
        }
        match &pending {
            Pending::Npcs(c)|Pending::Exits(c)|Pending::EncounterAreas(c)|Pending::Players(c)=>{
                let column=match &pending {
                    Pending::Npcs(_)=>SceneColumn::Npc,
                    Pending::Exits(_)=>SceneColumn::Exit,
                    Pending::Players(_)=>SceneColumn::Players,
                    _=>SceneColumn::Encounter,
                };
                if !self.scene_query_current(*c,column){return}
                self.scene_list_error(*c,column,&err.message);
            }
            Pending::EncounterStart=>{
                // 失败不动坐标，重新拉一次区域列表（可用性可能已经变了）。
                if let Some(w)=self.world.as_mut(){w.scene.starting_encounter=false;}
                self.request_scene_list(SceneColumn::Encounter);
            }
            Pending::UseExit=>{
                if let Some(w)=self.world.as_mut(){w.scene.using_exit=false;}
                self.request_scene_list(SceneColumn::Exit);
            }
            _=>{}
        }
        if let Pending::BattleCommand { turn } | Pending::BattleClone { turn, .. } = &pending {
            if self
                .world
                .as_ref()
                .and_then(|w| w.battle.as_ref())
                .is_none_or(|b| b.turn != *turn)
            {
                return;
            }
        }
        if let Pending::PartyPlayers(op) = &pending {
            self.on_players_op_failed(op, &err.code);
        }
        if let Pending::Trade(op) = &pending {
            self.on_trade_failed(op, &err.code);
        }
        if let Pending::SocialSettings { .. } = &pending {
            self.on_social_failed();
        }
        let mut text = err.message.clone();
        // STAGE-295：交易的请求先用交易的提示，组队的文案（如 TARGET_REFUSES「关闭了组队邀请」）不套到交易上。
        // STAGE-346：道具使用的 INVALID_TARGET 是“目标栏位没有宠物”等具体原因，不能被组队的
        // 「对象不能是自己」顶掉——服务端有文字时直接用服务端的。
        let hint = match pending {
            Pending::ItemOp(_) => None,
            Pending::Trade(_) => super::trade_text::error_hint(&err.code)
                .or_else(|| super::party_players::party_error_hint(&err.code)),
            _ => super::party_players::party_error_hint(&err.code)
                .or_else(|| super::trade_text::error_hint(&err.code)),
        };
        if let Some(hint) = hint {
            text = hint.to_string();
        }
        if text.is_empty() {
            // STAGE-231：精灵的错误码没有中文原因时，用前端的提示（服务端的原因优先）。
            let hint = if matches!(pending, Pending::MagicUse) { magic_use_error_hint(&err.code) } else { None };
            text = hint.map_or_else(|| format!("请求失败（{}）", err.code), str::to_string);
        }
        if err.code == "ACCOUNT_LOCKED" {
            if let Some(s) = err.retry_after_seconds() {
                text = format!("{text}，剩余 {}", format_wait(s));
            }
        }
        if err.code == "RATE_LIMITED" {
            if let Some(s) = err.retry_after_seconds() {
                text = format!("{text}，请 {} 后再试", format_wait(s));
            }
        }
        self.log.error(&text);
        if pending.refresh_bit() != 0 && self.reconnect.as_ref().is_some_and(|r| r.refreshing()) {
            self.offline(&format!("资料刷新失败：{text}"), false);
            return;
        }
        if matches!(pending, Pending::TokenCreate) {
            if let Some(w) = self.world.as_mut() {
                w.agent.guide_wanted = false;
            }
        }
        match pending {
            Pending::Hello => self.drop_link(),
            Pending::Login { .. } => self.login.password.clear(),
            Pending::Create => self.on_create_error(&err),
            Pending::Characters | Pending::Delete | Pending::Enter | Pending::Leave => {}
            Pending::Register { .. } | Pending::Ping => {}
            Pending::Status
            | Pending::Turn
            | Pending::Places
            | Pending::Maps
            | Pending::Inventory
            | Pending::Pets
            | Pending::Quests
            | Pending::QuestNpcs { .. }
            | Pending::Npcs(_)
            | Pending::Players(_)
            | Pending::Exits(_)
            | Pending::EncounterAreas(_)
            | Pending::EncounterStart
            | Pending::UseExit
            | Pending::ItemOp(_)
            | Pending::GoldDrop { .. }
            | Pending::ItemInlay(_)
            | Pending::Warp(_)
            | Pending::DuelChallenge
            | Pending::NpcFind(_)
            | Pending::NpcList { .. }
            | Pending::NpcWarp { .. }
            | Pending::Talk
            | Pending::NpcChoose
            | Pending::NpcReply
            | Pending::NpcClose
            | Pending::AddPoint { .. }
            | Pending::MagicUse
            | Pending::PartyOp(_)
            | Pending::PartyPlayers(_)
            | Pending::Trade(_)
            | Pending::SocialSettings { .. }
            | Pending::Take
            | Pending::Release
            | Pending::KickAgent
            | Pending::AgentSay(_)
            | Pending::Chat { .. }
            | Pending::TokenCreate
            | Pending::TokenList
            | Pending::TokenRevoke => {}
            Pending::PetCatalog => self.on_pet_catalog_error(text.clone()),
            Pending::PetStart { .. } => {}
            Pending::LevelingCatalog { .. } => self.on_leveling_catalog_error(text.clone()),
            Pending::LevelingStart { .. } => {}
            Pending::PetOp(op) => self.on_pet_op_failed(&op),
            Pending::AutoPost(op) => self.on_post_failed(op, &err.code),
            Pending::BattleCommand { turn } => {
                if self
                    .world
                    .as_ref()
                    .and_then(|w| w.battle.as_ref())
                    .is_some_and(|b| b.turn == turn)
                {
                    self.on_battle_command_failed();
                }
            }
            Pending::BattleClone { turn, .. } => {
                if let Some(b) = self.world.as_mut().and_then(|w| w.battle.as_mut()).filter(|b| b.turn == turn) {
                    b.on_clone_rejected();
                }
            }
            // 停止遇敌被拒（STAGE-153）：挂机保持关闭、说明原因，等玩家从系统页重试。
            Pending::EncounterStop | Pending::Encounter { set: Some(false) } => self.halt_failed(&text),
            Pending::Encounter { .. } | Pending::BattleAck | Pending::BattleQuery => {}
        }
    }
}
