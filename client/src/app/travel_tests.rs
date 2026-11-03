//! 快捷传送目录状态的单元测试（从 travel.rs 拆出，STAGE-251 之前的行为不变）。

use super::*;

fn node(id: &str, parent: Option<&str>, name: &str, floor: Option<u32>, order: u32) -> TravelNode {
    TravelNode {
        id: id.into(),
        parent_id: parent.map(Into::into),
        name: name.into(),
        floor,
        order,
    }
}

/// 萨伊那斯 → 萨姆吉尔村 → 村长家，外加一个纯分组。
fn catalog() -> Vec<TravelNode> {
    vec![
        node("group:sainasu", None, "萨伊那斯", Some(100), 0),
        node("map:1000", Some("group:sainasu"), "萨姆吉尔村", Some(1000), 0),
        node("map:1006", Some("map:1000"), "萨姆吉尔的村长家", Some(1006), 0),
        node("group:dungeons", Some("group:sainasu"), "洞窟与通路", None, 1),
        node(
            "map:10001",
            Some("group:dungeons"),
            "阿布的洞窟地下１楼",
            Some(10001),
            0,
        ),
    ]
}

fn ready() -> TravelState {
    let mut state = TravelState::default();
    state.install("v1".into(), &catalog());
    state.here_floor = Some(1006);
    state
}

#[test]
fn root_layer_lists_regions_in_order() {
    let state = ready();
    assert!(state.ready());
    // 根层末尾多一项「1级宠物」（STAGE-251）。
    assert_eq!(state.entries(), vec![Entry::Node("group:sainasu".into()), Entry::Pets, Entry::Leveling]);
    assert_eq!(state.path(), Vec::<String>::new());
}

#[test]
fn map_with_children_offers_self_first() {
    let mut state = ready();
    state.set_cursor(0);
    assert_eq!(state.enter(), TravelAction::Descend, "地图节点有子项，Enter 下钻");
    assert_eq!(
        state.entries(),
        vec![
            Entry::Here("group:sainasu".into()),
            Entry::Node("map:1000".into()),
            Entry::Node("group:dungeons".into()),
        ]
    );
    assert_eq!(state.title(), "萨伊那斯");
}

#[test]
fn here_entry_warps_to_the_layer_itself() {
    let mut state = ready();
    state.enter(); // 进入萨伊那斯
    assert_eq!(
        state.enter(),
        TravelAction::Warp {
            floor: 100,
            name: "萨伊那斯".into()
        },
        "首项是「传送到本图」"
    );
}

#[test]
fn leaf_map_warps_directly() {
    let mut state = ready();
    state.enter(); // 萨伊那斯
    state.set_cursor(1); // 萨姆吉尔村（有下级）
    assert_eq!(state.enter(), TravelAction::Descend, "选中有下级的地图，Enter 下钻");
    // 村庄这一层：首项是「传送到本图」，下面是它的下级
    state.set_cursor(0);
    assert_eq!(
        state.enter(),
        TravelAction::Warp {
            floor: 1000,
            name: "萨姆吉尔村".into()
        },
        "村庄本身也能传送"
    );
    state.set_cursor(1); // 萨姆吉尔的村长家（无下级）
    assert_eq!(
        state.enter(),
        TravelAction::AlreadyHere("萨姆吉尔的村长家".into()),
        "已经是当前所在地，只提示不发送"
    );
}

#[test]
fn already_here_is_not_sent_again() {
    let mut state = ready();
    state.here_floor = Some(1006);
    state.enter(); // 萨伊那斯
    state.set_cursor(1);
    state.enter(); // 萨姆吉尔村
    state.set_cursor(1); // 村长家
    assert!(matches!(state.enter(), TravelAction::AlreadyHere(_)));
}

#[test]
fn grouping_node_cannot_be_warped() {
    let mut state = ready();
    state.enter(); // 萨伊那斯
    state.set_cursor(2); // 洞窟与通路（纯分组）
    assert_eq!(state.enter(), TravelAction::Descend);
    assert_eq!(state.title(), "洞窟与通路");
    // 纯分组这一层没有「传送到本图」首项，首项就是它下面的地图
    assert_eq!(state.entries(), vec![Entry::Node("map:10001".into())]);
}

#[test]
fn back_restores_cursor_and_leaves_at_root() {
    let mut state = ready();
    state.enter(); // 萨伊那斯
    state.set_cursor(2); // 洞窟与通路
    state.enter();
    assert_eq!(state.back(), TravelAction::Ascend);
    assert_eq!(state.cursor(), 2, "回到上一层要停在原来那一项");
    assert_eq!(state.back(), TravelAction::Ascend);
    assert_eq!(state.back(), TravelAction::Leave, "根层再退就退出页面");
}

#[test]
fn navigation_stops_at_edges() {
    let mut state = ready();
    state.enter();
    state.set_cursor(0);
    assert!(!state.navigate(TravelNav::Up), "第一项再往上不动");
    assert!(state.navigate(TravelNav::End));
    assert_eq!(state.cursor(), 2);
    assert!(!state.navigate(TravelNav::Down), "最后一项再往下不动");
    assert!(state.navigate(TravelNav::Home));
    assert_eq!(state.cursor(), 0);
}

#[test]
fn missing_catalog_is_unavailable_not_empty() {
    let mut state = TravelState::default();
    state.install(String::new(), &[]);
    assert!(!state.ready());
    assert!(matches!(state.status, TravelStatus::Unavailable(_)));
    // 地图树不可用时根层只剩「1级宠物」（STAGE-251），不是地图。
    assert_eq!(state.entries(), vec![Entry::Pets, Entry::Leveling]);
}

#[test]
fn orphan_nodes_do_not_enter_the_tree() {
    let mut state = TravelState::default();
    state.install(
        "v1".into(),
        &[
            node("group:root", None, "根", None, 0),
            node("map:1", Some("group:missing"), "孤儿", Some(1), 0),
        ],
    );
    assert_eq!(state.floors(), vec![1], "节点的 floor 还在表里");
    assert_eq!(
        state.entries(),
        vec![Entry::Node("group:root".into()), Entry::Pets, Entry::Leveling],
        "但树上没有它"
    );
}

// STAGE-251：「1级宠物」只在根层、目录就绪时出现；选中它返回 OpenPets，不当成地图。
#[test]
fn pets_entry_is_root_only_and_opens_the_directory() {
    let mut state = ready();
    // STAGE-279：「练级传送」紧跟在「1级宠物」之后，成为最后一项。
    assert_eq!(state.entries().iter().rev().nth(1), Some(&Entry::Pets));
    assert!(state.view().iter().rev().nth(1).is_some_and(|row| row.pets && !row.here));
    assert_eq!(state.selected_id(), Some("group:sainasu".into()), "默认光标不在它上面");
    state.set_cursor(state.entries().len() - 2);
    assert_eq!(state.selected_id(), None, "「1级宠物」没有节点 id");
    assert_eq!(state.enter(), TravelAction::OpenPets);
    // 下钻到子层后，子层里没有它。
    state.set_cursor(0);
    assert_eq!(state.enter(), TravelAction::Descend);
    assert!(!state.entries().contains(&Entry::Pets));
}

#[test]
fn pets_entry_stays_when_the_tree_is_not_ready() {
    // STAGE-251：「1级宠物」不依赖地图目录，目录加载中或不可用时根层也要有它，能进去。
    let mut state = TravelState::default();
    state.install(String::new(), &[]);
    assert!(!state.ready());
    assert_eq!(state.entries(), vec![Entry::Pets, Entry::Leveling], "不可用时根层只剩「1级宠物」「练级传送」");
    assert_eq!(state.enter(), TravelAction::OpenPets);
    assert_eq!(TravelState::default().entries(), vec![Entry::Pets, Entry::Leveling], "加载中（未 install）同样有它");
}
