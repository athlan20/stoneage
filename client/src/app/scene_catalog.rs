//! NPC 目录的查询、过滤与选择。
use crossterm::event::{KeyCode,KeyEvent};
use serde_json::json;
use super::App;
use crate::proto::NpcCatalog;
use super::scene_action::SceneStep;
use super::session::Pending;
use super::confirm::Confirm;
const PAGE_STEP:usize=10;
const CATALOG_LIMIT:usize=200;

impl App {
    pub(super) fn scene_catalog_key(&mut self, key: KeyEvent) -> bool {
        match key.code {
            KeyCode::Esc => self.scene_step_back(),
            KeyCode::Tab => {
                self.catalog_switch_scope();
                true
            }
            KeyCode::Up | KeyCode::Down => {
                self.catalog_move(key.code == KeyCode::Down, 1);
                true
            }
            KeyCode::PageUp | KeyCode::PageDown => {
                self.catalog_move(key.code == KeyCode::PageDown, PAGE_STEP);
                true
            }
            KeyCode::Enter => {
                // 搜索词或分类改过就先重新查，否则选中的是候选。
                if self.catalog_dirty() {
                    self.catalog_fetch();
                } else {
                    self.catalog_pick();
                }
                true
            }
            _ => {
                self.catalog_type(&key);
                true
            }
        }
    }

    fn catalog_dirty(&self) -> bool {
        matches!(
            self.world.as_ref().and_then(|w| w.scene_step.as_ref()),
            Some(SceneStep::Catalog { dirty: true, .. })
        )
    }

    /// 目录页的搜索框是显式输入，普通字符都进它（包括 `/`）；改过之后标记成要重查。
    fn catalog_type(&mut self, key: &KeyEvent) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        let Some(SceneStep::Catalog { text, dirty, .. }) = world.scene_step.as_mut() else {
            return;
        };
        let before = text.text().to_string();
        text.handle_key(key);
        if text.text() != before {
            *dirty = true;
        }
    }

    /// 切分类：只改分类并标记成要重查，**不在这里发请求**（等玩家按 Enter 再查），
    /// 这样切换本身不产生流量，也不会把已经查好的候选清掉。
    fn catalog_switch_scope(&mut self) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        let Some(SceneStep::Catalog { scope, dirty, .. }) = world.scene_step.as_mut() else {
            return;
        };
        *scope = scope.next();
        *dirty = true;
    }

    fn catalog_move(&mut self, down: bool, step: usize) {
        let rows = match self.world.as_ref().and_then(|w| w.scene_step.as_ref()) {
            Some(SceneStep::Catalog { items, .. }) => items.len(),
            _ => 0,
        };
        if rows == 0 {
            return;
        }
        let Some(world) = self.world.as_mut() else {
            return;
        };
        if let Some(SceneStep::Catalog { index, .. }) = world.scene_step.as_mut() {
            let current = (*index).min(rows - 1);
            *index = if down {
                (current + step).min(rows - 1)
            } else {
                current.saturating_sub(step)
            };
        }
    }

    /// 发一次目录查询（分类 + 搜索词 + 分页）。
    pub(super) fn catalog_fetch(&mut self) {
        if let Some(w)=self.world.as_mut(){w.catalog_query+=1;}
        let query=self.world.as_ref().map_or(0,|w|w.catalog_query);
        let (scope, name) = match self.world.as_ref().and_then(|w| w.scene_step.as_ref()) {
            Some(SceneStep::Catalog { text, scope, .. }) => {
                (*scope, text.text().trim().to_string())
            }
            _ => return,
        };
        if let Some(world) = self.world.as_mut() {
            if let Some(SceneStep::Catalog { loading, dirty, .. }) = world.scene_step.as_mut() {
                *loading = true;
                *dirty = false;
            }
        }
        let mut data = json!({"scope": scope.wire(), "offset": 0, "limit": CATALOG_LIMIT});
        if !name.is_empty() {
            data["name"] = json!(name);
        }
        self.send("npc_list", data, Pending::NpcList{query,offset:0});
    }

    /// `npc_list` 响应：填进目录页。玩家已经按 Esc 离开时丢掉这份结果。
    pub(super) fn on_npc_list(&mut self,query:u64,offset:usize,data:serde_json::Value){
        let list:NpcCatalog=serde_json::from_value(data).unwrap_or_default();
        let Some(w)=self.world.as_mut() else{return;};if w.catalog_query!=query{return;}
        let Some(SceneStep::Catalog{items,total,index,loading,scope,text,dirty,..})=w.scene_step.as_mut() else{return;};
        if *dirty { *loading=false;return; }
        if offset==0{items.clear();*index=0;}else if items.len()!=offset{return;}
        let received=list.npcs.len();items.extend(list.npcs);*total=list.total;
        if received==0 && items.len()<*total{*loading=false;self.log.error("NPC目录未取完整，请重新查询");return;}
        if items.len()<*total{
            let next=items.len();let data=json!({"scope":scope.wire(),"name":text.text().trim(),"offset":next,"limit":CATALOG_LIMIT});
            self.send("npc_list",data,Pending::NpcList{query,offset:next});
        }else{*loading=false;}
    }

    /// 选中候选：就在本图直接对话；在别的图上问一句再过去（沿用 `npc_visit` 的确认）。
    fn catalog_pick(&mut self) {
        let Some(world) = self.world.as_ref() else {
            return;
        };
        let Some(SceneStep::Catalog { items, index, .. }) = world.scene_step.as_ref() else {
            return;
        };
        let Some(npc) = items.get(*index).cloned() else {
            return;
        };
        if npc.floor == world.entry.position.floor {
            self.open_npc(npc.id);
            self.close_scene_step();
            return;
        }
        let text = format!(
            "{} 在 {}，不在这张地图上。要传送过去吗？",
            npc.name, npc.map_name
        );
        self.ask_confirm(Confirm::NpcVisit(npc), &text);
        self.close_scene_step();
    }

    // ---- 通用 ----
}
