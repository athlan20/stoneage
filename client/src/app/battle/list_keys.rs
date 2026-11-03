//! 列表面板的按键（STAGE-107，`docs/battle-ui.md`「列表面板」）：↑↓ 移动高亮、PgUp/PgDn 跨 `PAGE_STEP` 行、
//! Enter 确认高亮项、数字键按栏位号直选（道具不直选，见 `list_digit`）。列表的内容见 `list.rs`，Esc 在 `keys.rs`。

use super::Pick;
use super::command::{Act, Actor};
use super::list::{self, Row};
use super::list_focus;
use crate::app::App;

impl App {
    /// 当前列表的行；没有世界数据时为空。
    fn list_rows(&self, pick: Pick) -> Vec<Row> {
        self.world.as_ref().map(|w| list::rows(w, pick)).unwrap_or_default()
    }

    /// 高亮移动：`n` 为 1 是方向键，`PAGE_STEP` 是 PgUp、PgDn。
    pub(super) fn list_move(&mut self, pick: Pick, n: usize, down: bool) {
        let rows = self.list_rows(pick);
        let Some(b) = self.battle_mut() else {
            return;
        };
        let from = list_focus::clamp(&rows, b.list_focus);
        b.list_focus = list_focus::step(&rows, from, n, down);
    }

    /// 数字键：精灵、换宠、技能按栏位号直选，号对不上（空栏位、没有这个号）时说明，留在列表里。
    /// 道具不直选（槽位 5–19 是两位数，列表上的号和命令要的号必须一样），提示用 ↑↓ 或 `i 槽位号`。
    pub(super) fn list_digit(&mut self, actor: Actor, pick: Pick, number: u8) {
        if pick == Pick::Item {
            self.log
                .error("道具用 ↑↓ 选、Enter 确认，也可以输入 i 加槽位号，例如 i 7");
            return;
        }
        // 0 键进来的是 10（见 `battle_key`），这里转回栏位 0。
        let slot = i64::from(number % 10);
        let rows = self.list_rows(pick);
        let Some(index) = rows.iter().position(|r| r.key == slot && r.number.is_some()) else {
            self.log.error(format!("没有 {slot} 号栏位"));
            return;
        };
        if let Some(b) = self.battle_mut() {
            b.list_focus = index;
        }
        self.list_choose(actor, pick, &rows[index]);
    }

    /// Enter：确认高亮的一行。
    pub(super) fn list_enter(&mut self, actor: Actor, pick: Pick) {
        let rows = self.list_rows(pick);
        let focus = self.battle_ref().map_or(0, |b| list_focus::clamp(&rows, b.list_focus));
        match rows.get(focus) {
            Some(row) => self.list_choose(actor, pick, row),
            None => self.log.error("这里没有可以选的"),
        }
    }

    /// 选中一行：不能选的说明原因、留在列表里；能选的按动作出招。道具、精灵、技能看要不要选目标（`choose_thing`），
    /// 换宠直接换上场，收回那一行发 `swap -`。
    fn list_choose(&mut self, actor: Actor, pick: Pick, row: &Row) {
        if let Some(reason) = &row.blocked {
            self.log.error(reason.clone());
            return;
        }
        let epoch=self.world.as_ref().map_or(0,|w|w.inventory_epoch);
        if let Some(b)=self.battle_mut(){b.prepared_row=Some((pick,row.clone()));b.prepared_epoch=epoch;}
        match pick {
            Pick::Item => self.choose_thing(actor, Act::Item, row.key),
            Pick::Magic => self.choose_thing(actor, Act::Magic, row.key),
            Pick::Skill => self.choose_thing(actor, Act::Skill, row.key),
            Pick::Pet => {
                self.send_command(Actor::Player, Act::SwapPet, None, Some(row.key));
            }
            Pick::Action | Pick::Target { .. } | Pick::Clones | Pick::CloneAim { .. } => {}
        }
    }
}
