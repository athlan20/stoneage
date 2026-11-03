//! STAGE-273：场景页「附近玩家」列的状态（同图其他在线玩家，`docs/protocol.md` 5.66）。
//! 以 `players` 整表为准；`players_changed` 推送按名字增量应用。

use super::scene_state::LoadState;
use crate::proto::{NearbyPlayer, PlayerChange};
use std::cmp::Ordering;

#[derive(Debug, Default)]
pub struct PlayersState {
    pub list: Vec<NearbyPlayer>,
    /// 同图超过 50 人时服务端只给了前 50 个。
    pub truncated: bool,
    pub load: LoadState,
    /// 请求编号：每发一次 `players` 加一，换图也加一，旧响应靠它被丢弃（同 NPC 列）。
    pub query: u64,
    pub index: usize,
    pub top: usize,
    /// 动作面板打开时选中的动作（下标，见 `players_keys::PLAYER_ACTIONS`）。
    pub prompt: Option<usize>,
}

impl PlayersState {
    /// 换地图或重新进入世界：旧列表作废，在途的响应也因 query 变化而被丢弃。
    pub fn invalidate(&mut self) {
        self.query += 1;
        self.list.clear();
        self.truncated = false;
        self.load = LoadState::Loading;
        self.index = 0;
        self.top = 0;
        self.prompt = None;
    }

    pub fn len(&self) -> usize {
        self.list.len()
    }

    pub fn selected(&self) -> Option<&NearbyPlayer> {
        self.list.get(self.index)
    }

    /// `players` 整表替换：按名字保住当前选中的玩家。
    pub fn replace(&mut self, list: Vec<NearbyPlayer>, truncated: bool) {
        let name = self.selected().map(|p| p.name.clone());
        self.list = list;
        self.truncated = truncated;
        self.sort();
        self.keep_selection(name);
        self.load = LoadState::Ready;
    }

    /// `players_changed` 增量：enter/update 整条替换（没有就新增），leave 按名字删除，不认识的 op 忽略。
    pub fn apply(&mut self, changes: Vec<PlayerChange>) {
        let name = self.selected().map(|p| p.name.clone());
        for c in changes {
            match c.op.as_str() {
                "enter" | "update" => {
                    if let Some(p) = c.player {
                        match self.list.iter_mut().find(|x| x.name == p.name) {
                            Some(slot) => *slot = p,
                            None => self.list.push(p),
                        }
                    }
                }
                "leave" => self.list.retain(|x| x.name != c.name),
                _ => {}
            }
        }
        self.sort();
        self.keep_selection(name);
    }

    /// 排序与服务端一致：等级从高到低，再按名字的 UTF-8 字节序。真人队上线后队伍成员要排在最前。
    fn sort(&mut self) {
        self.list.sort_by(|a, b| match b.level.cmp(&a.level) {
            Ordering::Equal => a.name.cmp(&b.name),
            o => o,
        });
    }

    fn keep_selection(&mut self, name: Option<String>) {
        self.index = name
            .and_then(|n| self.list.iter().position(|p| p.name == n))
            .unwrap_or(self.index)
            .min(self.list.len().saturating_sub(1));
    }

    /// 列的空状态：加载中、失败（可 F5 重试）、本图没有其他玩家。
    pub fn placeholder(&self) -> Option<String> {
        match &self.load {
            LoadState::Loading => Some("加载中…".into()),
            LoadState::Failed(e) => Some(format!("加载失败：{e} · F5 重试")),
            LoadState::Ready if self.list.is_empty() => Some("本图没有其他玩家".into()),
            LoadState::Ready => None,
        }
    }
}
