//! 导航栈（STAGE-131，`docs/interaction.md`「按键分层」）：世界里一次只有最上面一层接收输入，
//! 渲染和按键读同一份状态。栈底是场景，按 Esc 打开游戏菜单，功能页从菜单进入，命令栏用 `/` 打开。
//! 更细的动作、详情、表单和确认层由后续子需求（STAGE-132 起）接入，层类型先在这里定好。
//!
//! 每层记自己的上下文（功能页的分类、对象键、滚动位置），退出再进来时恢复；`Scene` 和 `Menu`
//! 本身没有可记的上下文，它们的选择状态留在各自的界面状态里。

/// 世界里的一层界面，从下到上叠成栈。
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Layer {
    /// 场景根页：进入世界后的默认层，显示所在位置、角色状态、本地图 NPC 和传送、查找入口。
    Scene,
    /// 游戏菜单：场景根页按 Esc 打开，列出角色、宠物、背包、任务、快捷传送、社交、消息、百科、系统。
    Menu,
    /// 功能页：`usize` 是菜单下标（同 `world::MENU_TITLES`）。左右键在这里切分类。
    Page(usize),
    /// 对象动作菜单：在列表里选中一个对象后打开的动作列表（STAGE-132 起接入）。
    Action,
    /// 详情：任务攻略、百科词条、物品说明这类只读内容（STAGE-115 已有，STAGE-136 统一进来）。
    Detail,
    /// 表单：数量、名字、暗号等文字输入（STAGE-132 起把现有输入收进来）。
    Form,
    /// 确认：有损失的操作，默认取消（并入现有 `confirm.rs`，STAGE-131 起）。
    Confirm,
    /// 命令栏：非文字输入时按 `/` 打开的辅助入口，Esc 收起并保留草稿。
    Command,
    /// 聊天输入框（STAGE-278）：场景「附近玩家」列的「聊天…」或 `say`/`world` 命令打开，Esc 收起并保留草稿。
    Chat,
}

impl Layer {
    /// 简短路径提示里这一层的名字（`docs/cli-ui-redesign.md`「场景与传送」的路径条）。
    pub fn label(&self) -> &'static str {
        match self {
            Layer::Scene => "场景",
            Layer::Menu => "菜单",
            Layer::Page(_) => "页面",
            Layer::Action => "动作",
            Layer::Detail => "详情",
            Layer::Form => "输入",
            Layer::Confirm => "确认",
            Layer::Command => "命令",
            Layer::Chat => "聊天",
        }
    }

    /// 这一层是不是模态的：打开时挡住下层，Esc 只退它自己（`docs/interaction.md`「按键分层」）。
    pub fn is_modal(&self) -> bool {
        matches!(self, Layer::Action | Layer::Detail | Layer::Form | Layer::Confirm | Layer::Command)
    }
}

/// 世界里的导航栈。栈底永远是场景；`pop` 不会把场景退掉。
#[derive(Clone, Debug)]
pub struct Nav {
    stack: Vec<Layer>,
}

impl Default for Nav {
    fn default() -> Self {
        Self::new()
    }
}

impl Nav {
    /// 进入世界时的栈：只有场景。
    pub fn new() -> Self {
        Self { stack: vec![Layer::Scene] }
    }

    /// 最上面一层：接收输入的那一层。
    pub fn top(&self) -> &Layer {
        self.stack.last().unwrap_or(&Layer::Scene)
    }

    /// 最上面一层的下标（从 0 数），渲染判断层用。
    pub fn depth(&self) -> usize {
        self.stack.len()
    }

    /// 栈顶是不是这一层。
    pub fn is(&self, layer: &Layer) -> bool {
        self.top() == layer
    }

    /// 栈里有没有这一层（不在栈顶也算）。
    pub fn contains(&self, layer: &Layer) -> bool {
        self.stack.contains(layer)
    }

    /// 压入一层。
    pub fn push(&mut self, layer: Layer) {
        self.stack.push(layer);
    }

    /// 退一层；已经在栈底（只剩场景）时不动，返回 false。
    pub fn pop(&mut self) -> bool {
        if self.stack.len() <= 1 {
            return false;
        }
        self.stack.pop();
        true
    }

    /// 换掉栈顶那一层（功能页里换页时不叠层）；栈是空的时等同于压入。
    pub fn replace(&mut self, layer: Layer) {
        match self.stack.last_mut() {
            Some(top) => *top = layer,
            None => self.stack.push(layer),
        }
    }

    /// 回到场景，栈里其他层全部丢掉（离开世界再进来、角色切换等）。
    pub fn reset(&mut self) {
        self.stack.truncate(1);
    }

    /// 从下到上的层，路径提示用。
    pub fn layers(&self) -> &[Layer] {
        &self.stack
    }

    pub fn close_command(&mut self) {
        if let Some(i) = self.stack.iter().rposition(|l| *l == Layer::Command) { self.stack.remove(i); }
    }

    /// 去掉最上面的动作层，不论它在栈的哪一层（STAGE-256：放生确认答 y 时，命令栏可能压在动作层上面）。
    pub fn close_action(&mut self) {
        if let Some(i) = self.stack.iter().rposition(|l| *l == Layer::Action) {
            self.stack.remove(i);
        }
    }

    pub fn show_page(&mut self, menu: usize) {
        self.close_command();
        while matches!(self.top(), Layer::Action | Layer::Detail | Layer::Form | Layer::Confirm) { self.pop(); }
        if matches!(self.top(), Layer::Page(_)) { self.replace(Layer::Page(menu)); }
        else { self.push(Layer::Page(menu)); }
    }
}

#[cfg(test)]
mod tests {
    use super::{Layer, Nav};

    #[test]
    fn starts_at_scene_and_never_pops_it() {
        let mut nav = Nav::new();
        assert!(nav.is(&Layer::Scene));
        assert!(!nav.pop(), "只剩场景时退不动");
        assert_eq!(nav.layers(), &[Layer::Scene]);
    }

    #[test]
    fn pushing_and_popping_keeps_the_scene_at_the_bottom() {
        let mut nav = Nav::new();
        nav.push(Layer::Menu);
        nav.push(Layer::Page(3));
        assert!(nav.is(&Layer::Page(3)));
        assert_eq!(nav.depth(), 3);
        assert!(nav.contains(&Layer::Scene), "场景一直留在栈底");
        assert!(nav.pop());
        assert!(nav.is(&Layer::Menu));
        assert!(nav.pop());
        assert!(nav.is(&Layer::Scene));
    }

    #[test]
    fn reset_returns_to_the_scene() {
        let mut nav = Nav::new();
        nav.push(Layer::Menu);
        nav.push(Layer::Command);
        nav.reset();
        assert!(nav.is(&Layer::Scene));
        assert_eq!(nav.depth(), 1);
    }

    #[test]
    fn modal_layers_are_the_ones_that_hold_esc() {
        assert!(!Layer::Scene.is_modal());
        assert!(!Layer::Menu.is_modal());
        assert!(!Layer::Page(0).is_modal());
        for layer in [Layer::Action, Layer::Detail, Layer::Form, Layer::Confirm, Layer::Command] {
            assert!(layer.is_modal(), "{layer:?} 应当是模态层");
        }
    }
}
