//! STAGE-276：社交页「设置」栏（`docs/ui.md`「社交页的开关」，`docs/protocol.md` 5.69）：隐身与拒绝邀请两个开关。
//! 切换发 `social_settings`，只带被改的字段；不先改显示，响应或 `social_changed` 推送才改，失败只写消息栏、显示保持原值。

use serde_json::{Value, json};

use super::party::party_columns;
use super::select::{self, Column, Item, OnEnter};
use super::session::Pending;
use super::world::SOCIAL_MENU;
use super::{App, WorldState};
use crate::proto::{Social, SocialEnvelope};

/// 社交页「设置」栏（第三栏）。
pub const SETTINGS_COLUMN: &str = "设置";

/// 四个开关（STAGE-307 起含 duel）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SocialField {
    Visible,
    Invite,
    /// 接收交易邀请（STAGE-295，5.69 的 trade 字段）。
    Trade,
    /// 接受决斗（STAGE-307，5.69 的 duel 字段）：默认关，不存库。
    Duel,
}

impl SocialField {
    pub const ALL: [SocialField; 4] = [SocialField::Visible, SocialField::Invite, SocialField::Trade, SocialField::Duel];

    /// 设置栏里的标签。
    pub fn label(self) -> &'static str {
        match self {
            Self::Visible => "对附近玩家可见",
            Self::Invite => "接收组队邀请",
            Self::Trade => "接收交易邀请",
            Self::Duel => "接受决斗",
        }
    }

    /// 协议里的字段名。
    pub fn wire(self) -> &'static str {
        match self {
            Self::Visible => "visible",
            Self::Invite => "invite",
            Self::Trade => "trade",
            Self::Duel => "duel",
        }
    }

    pub fn get(self, s: Social) -> bool {
        match self {
            Self::Visible => s.visible,
            Self::Invite => s.invite,
            Self::Trade => s.trade,
            Self::Duel => s.duel,
        }
    }

    pub fn set(self, s: &mut Social, on: bool) {
        match self {
            Self::Visible => s.visible = on,
            Self::Invite => s.invite = on,
            Self::Trade => s.trade = on,
            Self::Duel => s.duel = on,
        }
    }

    /// 切换成功后写进消息栏的一句（`docs/ui.md`）。
    pub fn message(self, on: bool) -> &'static str {
        match (self, on) {
            (Self::Visible, false) => "你已隐身，别人看不到你",
            (Self::Visible, true) => "你已恢复可见",
            (Self::Invite, false) => "已拒绝接收组队邀请",
            (Self::Invite, true) => "已恢复接收组队邀请",
            (Self::Trade, false) => "已拒绝接收交易邀请",
            (Self::Trade, true) => "已恢复接收交易邀请",
            (Self::Duel, false) => "已关闭接受决斗",
            (Self::Duel, true) => "已开启接受决斗：同图的人可以直接向你发起决斗",
        }
    }

    fn key(self) -> i64 {
        match self {
            Self::Visible => 0,
            Self::Invite => 1,
            Self::Trade => 2,
            Self::Duel => 3,
        }
    }

    fn from_key(key: i64) -> Option<Self> {
        Self::ALL.into_iter().find(|f| f.key() == key)
    }
}

/// 设置栏：两行开关，都能选、都能按 Enter 切换。
pub fn settings_column() -> Column {
    Column {
        name: SETTINGS_COLUMN,
        items: SocialField::ALL
            .iter()
            .map(|f| Item {
                key: f.key(),
                usable: true,
                enter: Some(OnEnter::Open),
            })
            .collect(),
        shown: true,
        detail: false,
        default_key: None,
    }
}

/// 社交页现在停在设置栏（第三栏，下标 2）。
pub fn on_settings_column(world: &WorldState) -> bool {
    select::focus_column(&party_columns(world), &world.select[SOCIAL_MENU]) == 2
}

/// 设置栏里选中的开关。
pub fn selected_field(world: &WorldState) -> Option<SocialField> {
    select::selections(&party_columns(world), &world.select[SOCIAL_MENU])
        .get(2)
        .copied()
        .flatten()
        .and_then(SocialField::from_key)
}

impl App {
    /// 设置栏 Enter 或空格：切换选中的开关。观察模式下拦；上一次还没有答复时不重复发。
    pub(super) fn social_toggle(&mut self) {
        let Some(w) = self.world.as_ref() else {
            return;
        };
        let Some(field) = selected_field(w) else {
            return;
        };
        let value = !field.get(w.entry.social);
        self.social_set(field, value);
    }

    /// 发出一个开关的修改（只带这一个字段，`docs/protocol.md` 5.69）。观察模式下拦；上一次还没有答复时不重复发。
    /// 设置栏的 Enter 和命令 `duel on|off`（STAGE-307）共用。
    pub(super) fn social_set(&mut self, field: SocialField, value: bool) {
        if self.observer_blocks() {
            return;
        }
        let Some(w) = self.world.as_ref() else {
            return;
        };
        if w.social_busy.is_some() {
            return;
        }
        let sent = self.send(
            "social_settings",
            json!({ field.wire(): value }),
            Pending::SocialSettings { field, value },
        );
        if sent && let Some(w) = self.world.as_mut() {
            w.social_busy = Some(field);
        }
    }

    /// 响应成功：以响应里的 `social` 为准更新显示，并写一句消息。
    pub(super) fn on_social_ok(&mut self, field: SocialField, value: bool, data: Value) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        w.social_busy = None;
        let mut next = w.entry.social;
        match serde_json::from_value::<SocialEnvelope>(data) {
            Ok(env) => next = env.social,
            Err(_) => field.set(&mut next, value),
        }
        w.entry.social = next;
        self.log.party(field.message(field.get(next)));
    }

    /// 响应失败：解除「处理中」，显示保持原值（没有先改，所以无需回滚）；原因由通用的错误处理写进消息栏。
    pub(super) fn on_social_failed(&mut self) {
        if let Some(w) = self.world.as_mut() {
            w.social_busy = None;
        }
    }

    /// 推送 `social_changed`：只在值变了时由服务端推，直接换掉显示（同账号的其他连接也会收到）。
    pub(super) fn on_social_changed(&mut self, data: Value) {
        let Ok(env) = serde_json::from_value::<SocialEnvelope>(data) else {
            return;
        };
        if let Some(w) = self.world.as_mut() {
            w.entry.social = env.social;
        }
    }
}
