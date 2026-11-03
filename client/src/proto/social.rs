//! STAGE-276：社交开关（`docs/protocol.md` 5.69）。两个开关都是布尔值，服务端没给的字段取默认值（都开）。

use serde::Deserialize;

/// 社交开关的当前值：`visible` 为假就是隐身，`invite` 为假就是拒绝组队邀请。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Deserialize)]
#[serde(default)]
pub struct Social {
    pub visible: bool,
    pub invite: bool,
    /// 接收交易邀请（STAGE-295）：为假就是拒绝交易邀请。
    pub trade: bool,
    /// 接受决斗（STAGE-307，5.69）：默认关，不存库，打完一场或重新登录后自动关。
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

/// `social_settings` 的响应与 `social_changed` 推送：`data.social` 是改完之后的完整值。
#[derive(Debug, Clone, Copy, Default, Deserialize)]
pub struct SocialEnvelope {
    #[serde(default)]
    pub social: Social,
}
