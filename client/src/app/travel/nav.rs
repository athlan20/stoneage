//! 快捷传送目录的下钻与退回（STAGE-161）：`enter()` 决定 Enter 做什么，`back()` 处理 Esc。
//! 从 `travel.rs` 拆出，状态（路径栈、光标）仍在 `travel.rs` 里，这里只用它的字段。

use super::{Entry, TravelAction, TravelState};

impl TravelState {
    /// Enter：能下钻就下钻，纯分组没有可下钻的层时不动，否则传送。
    pub fn enter(&mut self) -> TravelAction {
        let Some(entry) = self.entries().get(self.cursor()).cloned() else {
            return TravelAction::None;
        };
        // 根层的「1级宠物」不是地图节点：交给页面打开目录（STAGE-251）。
        let id = match &entry {
            Entry::Pets => return TravelAction::OpenPets,
            Entry::Leveling => return TravelAction::OpenLeveling,
            Entry::Here(id) | Entry::Node(id) => id,
        };
        let Some(node) = self.nodes.get(id).cloned() else {
            return TravelAction::None;
        };
        let floor = match node.floor {
            Some(floor) => floor,
            // 纯分组只能往里走，不能传送；真进去是空的就显示「这一层没有可去的目录」。
            None => return self.descend(node.id),
        };
        if matches!(entry, Entry::Node(_)) && self.children.contains_key(&node.id) {
            return self.descend(node.id);
        }
        if Some(floor) == self.here_floor {
            return TravelAction::AlreadyHere(node.name);
        }
        TravelAction::Warp { floor, name: node.name }
    }

    fn descend(&mut self, id: String) -> TravelAction {
        self.stack.push(id);
        self.cursors.push(0);
        self.scrolls.push(0);
        TravelAction::Descend
    }

    /// Esc：退一层；已经到根层就退出页面。
    pub fn back(&mut self) -> TravelAction {
        if self.stack.pop().is_none() {
            return TravelAction::Leave;
        }
        if self.cursors.len() > 1 {
            self.cursors.pop();
            self.scrolls.pop();
        }
        TravelAction::Ascend
    }
}
