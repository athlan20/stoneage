//! STAGE-276：社交开关的自动应答（`docs/protocol.md` 5.69）。用例没写 `social_settings` 时按这里应答，并记住两个开关的值；
//! 值变了先推 `social_changed`，再回响应。环境变量 `STONEAGE_MOCK_SOCIAL_FAIL` 有值时回 `WRONG_STATE`，用来测失败回滚。

use serde_json::{Value, json};

/// 四个开关：visible、invite、trade 默认都开；duel（STAGE-307）默认关，不存库。
#[derive(Clone, Copy, Debug)]
pub struct Social {
    pub visible: bool,
    pub invite: bool,
    pub trade: bool,
    pub duel: bool,
}

impl Default for Social {
    fn default() -> Self {
        Self {
            visible: true,
            invite: true,
            trade: true,
            duel: false,
        }
    }
}

impl Social {
    pub fn json(self) -> Value {
        json!({"visible": self.visible, "invite": self.invite, "trade": self.trade, "duel": self.duel})
    }
}

/// 处理一条 `social_settings`：返回（要先发的推送，响应）。
pub fn settings(id: u64, data: &Value, state: &mut Social, fail: bool) -> (Option<Value>, Value) {
    let bad = |message: &str| json!({"id": id, "type": "social_settings", "ok": false, "error": {"code": "BAD_REQUEST", "message": message}});
    let Some(obj) = data.as_object() else {
        return (None, bad("data 必须是对象"));
    };
    if obj
        .iter()
        .any(|(k, v)| !matches!(k.as_str(), "visible" | "invite" | "trade" | "duel") || !v.is_boolean())
    {
        return (None, bad("只接受 visible、invite、trade、duel 四个布尔字段"));
    }
    if fail {
        let message = "社交设置暂时不能修改（模拟失败）";
        return (
            None,
            json!({"id": id, "type": "social_settings", "ok": false, "error": {"code": "WRONG_STATE", "message": message}}),
        );
    }
    let before = *state;
    if let Some(v) = obj.get("visible").and_then(Value::as_bool) {
        state.visible = v;
    }
    if let Some(v) = obj.get("invite").and_then(Value::as_bool) {
        state.invite = v;
    }
    if let Some(v) = obj.get("trade").and_then(Value::as_bool) {
        state.trade = v;
    }
    if let Some(v) = obj.get("duel").and_then(Value::as_bool) {
        state.duel = v;
    }
    let changed = before.visible != state.visible
        || before.invite != state.invite
        || before.trade != state.trade
        || before.duel != state.duel;
    let push = changed.then(|| json!({"type": "social_changed", "data": {"social": state.json()}}));
    (
        push,
        json!({"id": id, "type": "social_settings", "ok": true, "data": {"social": state.json()}}),
    )
}
