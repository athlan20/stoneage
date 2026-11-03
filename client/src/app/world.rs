//! 世界快照与页面状态。按键、导航和命令分别见 world_keys/world_nav/world_command。
use super::bag_action::BagStep;
use super::battle::BattleState;
use super::char_action::CharStep;
use super::confirm::Confirm;
use super::nav::{Layer,Nav};
use super::pet_action::PetStep;
use super::party_action::PartyStep;
use super::scene_action::SceneStep;
use super::quest_guide::QuestGuide;
use super::quests::QuestFilter;
use super::select::PageFocus;
use super::{AgentState,History,MessageView,TextInput,TravelState,WikiState};
use crate::proto::{Inventory,Npc,NpcWindow,Pets,Place,Quests,WarpMap,WorldEntry};

/// 「地图」页在菜单里的位置（从 0 数）。
pub const MAP_MENU: usize = 0;

/// 「角色」页在菜单里的位置（从 0 数）。
pub const CHAR_MENU: usize = 1;

/// 「系统」页在菜单里的位置（从 0 数）。
pub const SYSTEM_MENU: usize = 7;

/// 「消息」页在菜单里的位置（从 0 数）。
pub const MESSAGES_MENU: usize = 6;

/// 「背包」页在菜单里的位置（从 0 数）。
pub const BAG_MENU: usize = 3;

/// 「任务」页在菜单里的位置（从 0 数，STAGE-86）。
pub const QUEST_MENU: usize = 4;

/// 「宠物」页在菜单里的位置（从 0 数）。
pub const PETS_MENU: usize = 2;

/// 「百科」页在菜单里的位置（从 0 数，STAGE-83）。
pub const WIKI_MENU: usize = 8;

/// 「社交」页在菜单里的位置（从 0 数）：STAGE-162 起有「队伍」一栏，名片簿、邮件、社交开关仍未做。
pub const SOCIAL_MENU: usize = 5;

/// 「自动战斗」页（STAGE-173）：顶层功能页，进页就是五栏设置编辑器。
/// 数字键只到 9，这一页用游戏菜单或 `page 10` / `page 自动战斗` 打开。
pub const AUTO_MENU: usize = 9;

pub const MENU_TITLES: [&str; 10] = [
    "1快捷传送", "2角色", "3宠物", "4背包", "5任务", "6社交", "7消息", "8系统", "9百科", "10自动战斗",
];

/// 游戏菜单的项（STAGE-131，`docs/cli-ui-redesign.md`「场景与传送」）：显示名和它打开的功能页下标。
/// 顺序按方案的菜单走（角色、宠物、背包、任务、快捷传送、社交、消息、百科、系统），和顶部标签的顺序无关。
/// STAGE-161 起「地图与传送」改名为「快捷传送」，打开的是按包含层级逐级选择的目录页。
pub const GAME_MENU: [(&str, usize); 10] = [
    ("角色", CHAR_MENU),
    ("宠物", PETS_MENU),
    ("背包", BAG_MENU),
    ("任务", QUEST_MENU),
    ("快捷传送", MAP_MENU),
    ("社交", SOCIAL_MENU),
    ("消息", MESSAGES_MENU),
    ("自动战斗", AUTO_MENU),
    ("百科", WIKI_MENU),
    ("系统", SYSTEM_MENU),
];

/// 列表和详情并排所需的最小页面宽度（列）；更窄时详情替换列表（任务攻略、百科词条）。
pub const SIDE_BY_SIDE: u16 = 110;

/// 背包格子的编号（docs/ui.md 背包页）：0–4 装备（头、身、手、饰品×2），5–19 背包。
pub const BAG_LAST_SLOT: usize = 19;
/// 装备位的部位名，下标就是槽位编号。
pub const EQUIP_PARTS: [&str; 5] = ["头", "身", "手", "饰品", "饰品"];

#[derive(Debug)]
pub struct WorldState {
    pub online: bool,
    pub catalog_query:u64,
    pub system: super::system::SystemState,
    pub entry: WorldEntry,
    pub menu: usize,
    pub input: TextInput,
    pub agent: AgentState,
    /// 公开聊天的输入框（STAGE-278）：频道、草稿、在途状态。
    pub chat: super::chat::ChatState,
    /// 服务端的地点表（进入世界后取一次），`warp` 用它把名字换成编号。
    pub places: Vec<Place>,
    /// 能传到的地图（进入世界后取一次，STAGE-83）：`warp 地图名` 搜索和 Tab 补全用，没有坐标。
    pub maps: Vec<WarpMap>,
    /// 快捷传送目录（STAGE-161，`travel.rs`）：父子索引、路径栈、每层的光标与滚动。
    pub travel: TravelState,
    /// 「消息」页的滚动状态（`docs/ui.md` 消息页）。
    pub msg_view: MessageView,
    /// 背包（进入世界后取一次，之后靠 `inventory_changed` 推送，STAGE-28）。
    pub inventory: Inventory,
    pub inventory_epoch: u64,
    /// 「背包」页物品表的滚动状态：顶部显示的格子编号与上次渲染的可见行数。
    pub bag_scroll: usize,
    pub bag_rows: usize,
    /// 宠物（进入世界后取一次，之后靠 `pets_changed` 推送，STAGE-31）。
    pub pets: Pets,
    /// 当前地图上的 NPC（进入世界后取一次，换地图时重新取，STAGE-68，`docs/protocol.md` 5.32）。
    pub npcs: Vec<Npc>,
    /// 服务端驱动的 NPC 对话窗口（STAGE-69，`docs/protocol.md` 5.34）：有值时弹窗显示，
    /// 数字键和 Esc 由 `dialog_key` 处理。
    pub npc_window: Option<NpcWindow>,
    pub dialog: super::dialog::DialogState,
    pub browse: super::browse::BrowseState,
    /// 等玩家回答 y 的确认（STAGE-103，`confirm.rs`）：放生、丢弃、跨图找 NPC 各自问过的那一件事。
    pub confirm: Option<Confirm>,
    pub confirm_native:bool,
    pub confirm_choice:bool,
    pub confirm_text:String,
    /// 真人组队的请求（STAGE-274）：收到的与发出的，弹窗和请求栏共用。
    pub party_requests: super::party_requests::PartyRequests,
    /// STAGE-295：进行中的交易窗口（只存内存，断线即清）。
    pub trade: Option<super::trade_state::TradeWindow>,
    /// 社交开关正在等答复的那一个（STAGE-276）：显示「处理中」，期间不重复发。
    pub social_busy: Option<super::social_settings::SocialField>,
    pub confirm_revision:(u64,u64),
    pub pets_epoch:u64,
    pub bag_revision:u64,
    pub pet_revision:u64,
    /// 正在进行的战斗（STAGE-37）：有值时中间区域换成战场，菜单页不能切换。
    pub battle: Option<BattleState>,
    pub battle_return: Option<Nav>,
    /// 观察模式下玩家按 Esc 打开了游戏菜单（STAGE-182）：战斗在后台继续推进、顶部信息条保留，
    /// 中间区域和按键走普通的菜单与功能页路径。菜单退回场景、玩家 `take` 收回控制权时清掉。
    pub battle_veil: bool,
    /// 托管（自动战斗，STAGE-42）：开着时每回合自动出招，跨战斗持续到关掉为止。
    pub auto_battle: bool,
    /// 战后补血与丢弃（STAGE-260）：战斗收起后跑一轮，平时是默认值。
    pub post_battle: crate::app::battle::auto_post::PostBattle,
    /// 自动战斗设置页（STAGE-173）：有值时设置页打开着，草稿和光标都在里面。
    pub auto_edit: Option<super::auto_battle::edit::Editor>,
    /// 主动停机协调（STAGE-153）：停止意图在途、上一次失败的原因。
    pub halt: super::battle::HaltState,
    /// 任务（进入世界后取一次，之后靠 `quests_changed` 推送，STAGE-86）。
    pub quests: Quests,
    /// 「任务」页的显示范围（`quest` 命令设定）；滚动位置是第一行在列表里的下标，rows 是上次渲染的可见行数。
    pub quest_filter: QuestFilter,
    pub quest_scroll: usize,
    pub quest_rows: usize,
    /// 任务页打开的百科攻略（STAGE-89，`quest 编号`）：有值时任务页右边（窄屏时替换列表）显示攻略。
    pub quest_guide: Option<QuestGuide>,
    /// 本次登录以来输入过的命令（↑↓ 翻看，STAGE-26）。
    pub history: History,
    /// 「百科」页的浏览状态（STAGE-83）。
    pub wiki: WikiState,
    /// 每页的选择状态（STAGE-113，按菜单下标）：焦点栏和每栏的选中项，`docs/interaction.md`「页面选择」。
    pub select: Vec<PageFocus>,
    /// 中间页面区域的宽度，上次渲染时记下（页面选择要按它判断详情是并排还是替换列表）。第一次渲染前按宽屏算。
    pub page_width: u16,
    /// 中间页面区域的高度，同上（地图页的 NPC 表、角色页的能力表画不画得下，按它判断，STAGE-116）。
    pub page_height: u16,
    /// 导航栈（STAGE-131）：场景 → 菜单 → 功能页 → ……，只有最上面一层接收输入。
    pub nav: Nav,
    /// 场景左列的选中行（STAGE-148）：出口之后是三个固定入口。
    pub scene_index: usize,
    pub scene: super::scene_state::SceneState,
    /// 场景列表的顶部行（STAGE-147）：光标走出可见范围时列表跟着滚。
    pub scene_scroll: usize,
    /// 游戏菜单的选中行（STAGE-131）：`GAME_MENU` 的下标。
    pub menu_index: usize,
    /// 背包对象动作流程的当前一步（STAGE-132）：动作列表 → 目标 / 目的格 / 数量表单。
    pub bag_step: Option<BagStep>,
    /// 宠物对象动作流程的当前一步（STAGE-133）：动作列表 / 改名 / 技能。
    pub pet_step: Option<PetStep>,
    /// 角色页的加点表单（STAGE-133）。
    pub char_step: Option<CharStep>,
    /// 社交页的动作流程（STAGE-162）：动作列表 / 成员详情。
    pub party_step: Option<PartyStep>,
    /// 场景页的步骤（STAGE-134）：传送地点列表或按名字查找 NPC。
    pub scene_step: Option<SceneStep>,
}

/// 第一次渲染前页面高度按这个算（足够高，所有栏都画得出来）。
const DEFAULT_PAGE_HEIGHT: u16 = 40;

impl WorldState {
    /// 页面窄到详情要替换列表（见 `SIDE_BY_SIDE`）。
    pub fn is_narrow(&self) -> bool {
        self.page_width < SIDE_BY_SIDE
    }

    pub fn new(entry: WorldEntry) -> Self {
        Self {
            party_requests: Default::default(),
            trade: None,
            social_busy: None,
            entry,
            online: true,
            catalog_query:0,
            system: super::system::SystemState::default(),
            menu: 0,
            input: TextInput::new(220),
            chat: Default::default(),
            agent: AgentState::default(),
            places: vec![],
            maps: vec![],
            travel: TravelState::default(),
            msg_view: MessageView::default(),
            inventory: Inventory::default(),
            inventory_epoch: 0,
            bag_scroll: 5,
            bag_rows: 0,
            pets: Pets::default(),
            npcs: vec![],
            npc_window: None,
            dialog: super::dialog::DialogState::default(),
            browse: super::browse::BrowseState::default(),
            confirm: None,
            confirm_native:false,confirm_choice:false,confirm_text:String::new(),confirm_revision:(0,0),pets_epoch:0,bag_revision:0,pet_revision:0,
            battle: None,
            battle_return: None,
            battle_veil: false,
            auto_battle: false,
            post_battle: Default::default(),
            auto_edit: None,
            halt: super::battle::HaltState::default(),
            history: History::default(),
            quests: Quests::default(),
            quest_filter: QuestFilter::default(),
            quest_scroll: 0,
            quest_rows: 0,
            quest_guide: None,
            wiki: WikiState::default(),
            select: vec![PageFocus::default(); MENU_TITLES.len()],
            page_width: SIDE_BY_SIDE,
            page_height: DEFAULT_PAGE_HEIGHT,
            nav: Nav::new(),
            scene_index: 0,
            scene: Default::default(),
            scene_scroll: 0,
            menu_index: 0,
            bag_step: None,
            pet_step: None,
            char_step: None,
            party_step: None,
            scene_step: None,
        }
    }

    /// 场景焦点列的逻辑项数，占位和分隔线不计入。
    pub fn scene_rows(&self) -> usize {
        // 出口列含三个固定入口；NPC 列只有 NPC；遇敌列只有区域。
        self.scene.len(self.npcs.len())
    }

    /// 栈顶是不是场景根页。
    /// 明确的文字输入层；供驱动和渲染辨认，不由草稿是否为空推断焦点。
    pub fn text_entry_active(&self)->bool {
        self.nav.is(&Layer::Command) || self.confirm.is_some()
        || matches!(self.bag_step,Some(BagStep::DropCount{..}))
        || matches!(self.pet_step,Some(PetStep::Rename{..})) || self.char_step.is_some()
        || self.scene_step.is_some() || matches!(self.browse.step,Some(super::BrowseStep::Search{..}))
        || matches!(self.system.step,Some(super::system::SystemStep::Chat(_)|super::system::SystemStep::Create{..}))
        || self.npc_window.as_ref().is_some_and(|n|n.window_id.is_empty() || n.accepts_text || n.input.is_some())
    }
    pub fn at_scene(&self) -> bool {
        self.nav.is(&Layer::Scene)
    }

    /// 栈顶是不是游戏菜单。
    pub fn at_menu(&self) -> bool {
        self.nav.is(&Layer::Menu)
    }
}

