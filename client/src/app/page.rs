//! `page 编号|页名`（STAGE-103，`docs/interaction.md`「按键表」）：切换菜单页。弹窗开着时也能用，
//! 只换弹窗底下的页面，弹窗不关；战斗中战场占满中间区域，不能切页。输入行为空时数字键切页，效果相同。

use super::commands;
use super::world::{AUTO_MENU, MENU_TITLES, SYSTEM_MENU};
use super::App;

/// `page` 的参数换成菜单下标（从 0 数）：数字 1–9，或者页名（地图、背包……，不带编号）。
pub fn page_index(arg: &str) -> Option<usize> {
    if let Ok(n) = arg.parse::<usize>() {
        return (1..=MENU_TITLES.len()).contains(&n).then(|| n - 1);
    }
    MENU_TITLES.iter().position(|title| title.trim_start_matches(|c: char| c.is_ascii_digit()) == arg)
}

impl App {
    /// `page 编号|页名`：参数不对给出用法；战斗中不切页。
    pub(super) fn page_command(&mut self, rest: &str) {
        let arg = rest.trim();
        if arg.is_empty() {
            return self.log.error(commands::usage_of("page"));
        }
        let Some(menu) = page_index(arg) else {
            return self.log.error(format!("没有叫“{arg}”的页面，用 1–10 或页名（地图、角色、宠物、背包、任务、社交、消息、系统、百科、自动战斗）切换"));
        };
        if self.world.as_ref().is_some_and(|w| w.battle_front()) {
            return self.log.error("战斗中不能切页");
        }
        self.switch_menu(menu);
    }

    /// 切到第 `menu` 页（从 0 数）：离开页面时的临时状态一并清掉，切到「系统」页时取令牌列表。
    /// 数字键和 `page` 命令都走这里。导航栈（STAGE-131）：从场景/菜单进来时压一层功能页，
    /// 已经在功能页里换页时换掉栈顶，不叠层。
    pub(super) fn switch_menu(&mut self, menu: usize) {
        let Some(world) = self.world.as_mut() else {
            return;
        };
        world.menu = menu;
        world.party_step = None; // 换页时社交页的动作流程作废（导航栈的动作层由 show_page 弹掉）
        world.system = Default::default();
        world.agent.fresh = None; // 新令牌的原文只显示到离开系统页为止
        // 离开自动战斗页时丢掉草稿（STAGE-173）：下次进来重新按当前方案生成。
        world.auto_edit = None;
        world.nav.show_page(menu);
        if menu == SYSTEM_MENU {
            self.request_tokens();
        }
        if menu == AUTO_MENU {
            self.open_auto_settings(super::auto_battle::edit::Origin::TopPage);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::page_index;

    #[test]
    fn digits_and_names_map_to_menu_index() {
        assert_eq!(page_index("1"), Some(0));
        assert_eq!(page_index("9"), Some(8));
        assert_eq!(page_index("背包"), Some(3));
        assert_eq!(page_index("百科"), Some(8));
        assert_eq!(page_index("0"), None, "没有 0 页");
        assert_eq!(page_index("10"), Some(9), "第 10 页是自动战斗（STAGE-173）");
        assert_eq!(page_index("自动战斗"), Some(9));
        assert_eq!(page_index("不存在"), None);
        assert_eq!(page_index("4背包"), None, "页名不带编号");
    }
}
