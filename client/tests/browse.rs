use crossterm::event::{KeyCode, KeyCode::*, KeyEvent, KeyModifiers};
use stoneage_client::app::{
    App, BrowseStep, Layer, Options, QUEST_MENU, Screen, WIKI_MENU, WorldState,
};
use stoneage_client::proto::{Quest, WorldEntry};

fn app(menu: usize) -> App {
    let mut a = App::new(Options::default());
    let mut w = WorldState::new(WorldEntry::default());
    w.menu = menu;
    w.nav.push(Layer::Page(menu));
    w.quests.quests = vec![Quest {
        no: 1,
        name: "成人仪式".into(),
        state: "doing".into(),
    }];
    a.world = Some(w);
    a.screen = Screen::World;
    a
}
fn key(a: &mut App, k: KeyCode) {
    a.on_key(KeyEvent::new(k, KeyModifiers::NONE));
}

#[test]
fn quest_actions_guide_and_back_keep_object() {
    let mut a = app(QUEST_MENU);
    key(&mut a, Enter);
    assert!(matches!(
        a.world.as_ref().unwrap().browse.step,
        Some(BrowseStep::Quest { .. })
    ));
    key(&mut a, Enter);
    assert!(a.world.as_ref().unwrap().quest_guide.is_some());
    key(&mut a, PageDown);
    key(&mut a, Esc);
    assert!(a.world.as_ref().unwrap().quest_guide.is_none());
    assert!(matches!(
        a.world.as_ref().unwrap().browse.step,
        Some(BrowseStep::Quest { .. })
    ));
    key(&mut a, Esc);
    assert!(a.world.as_ref().unwrap().browse.step.is_none());
}

#[test]
fn wiki_search_is_an_explicit_text_form() {
    let mut a = app(WIKI_MENU);
    key(&mut a, Tab);
    for c in "乌力 / ".chars() {
        key(&mut a, Char(c));
    }
    match &a.world.as_ref().unwrap().browse.step {
        Some(BrowseStep::Search { input }) => assert_eq!(input.text(), "乌力 / "),
        _ => panic!("没有搜索框"),
    }
    key(&mut a, Esc);
    assert!(a.world.as_ref().unwrap().wiki.query.is_empty());
    key(&mut a, Char('x'));
    assert!(a.world.as_ref().unwrap().input.is_empty());
}

#[test]
fn quest_change_invalidates_the_selected_object_before_action() {
    let mut a = app(QUEST_MENU);
    key(&mut a, Enter);
    a.world.as_mut().unwrap().quests.quests[0].state = "done".into();
    key(&mut a, Enter);
    assert!(a.world.as_ref().unwrap().quest_guide.is_none());
}
