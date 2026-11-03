use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::{
    App, Layer, Options, SYSTEM_MENU, Screen, WorldState, system::SystemStep,
};
use stoneage_client::proto::WorldEntry;
fn app() -> App {
    let mut a = App::new(Options::default());
    a.screen = Screen::World;
    a.world = Some(WorldState::new(WorldEntry::default()));
    let w = a.world.as_mut().unwrap();
    w.menu = SYSTEM_MENU;
    w.nav.show_page(SYSTEM_MENU);
    a
}
fn key(a: &mut App, k: KeyCode) {
    a.on_key(KeyEvent::new(k, KeyModifiers::NONE));
}
#[test]
fn create_form_keeps_literal_text_and_cancel_is_safe() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 5;
    key(&mut a, KeyCode::Enter);
    for c in "我的 agent / 2".chars() {
        key(&mut a, KeyCode::Char(c));
    }
    key(&mut a, KeyCode::Tab);
    key(&mut a, KeyCode::Tab);
    assert!(
        matches!(a.world.as_ref().unwrap().system.step.as_ref(),Some(SystemStep::Create{label,confirm:false,..}) if label.text()=="我的 agent / 2")
    );
    key(&mut a, KeyCode::Enter);
    assert!(a.world.as_ref().unwrap().system.step.is_none());
    assert!(!a.session.busy());
}
#[test]
fn offline_agent_actions_do_not_create_requests_and_local_settings_work() {
    let mut a = app();
    key(&mut a, KeyCode::Enter);
    assert!(a.log.all().iter().any(|m| m.text.contains("不在线")));
    assert!(!a.session.busy());
    a.world.as_mut().unwrap().system.index = 8;
    let before = a.unicode;
    key(&mut a, KeyCode::Enter);
    assert_ne!(before, a.unicode);
    a.world.as_mut().unwrap().system.index = 9;
    key(&mut a, KeyCode::Enter);
    assert_eq!(a.default_battle_speed, 2);
    a.world.as_mut().unwrap().system.index = 14;
    key(&mut a, KeyCode::Enter);
    assert!(a.exit_dialog.is_some_and(|d| !d.confirm));
}
#[test]
fn system_selection_is_visible_at_supported_sizes() {
    use ratatui::{Terminal, backend::TestBackend};
    let mut a = app();
    for (x, y) in [(80, 24), (120, 40), (200, 60), (80, 24)] {
        for i in [0, 5, 10, 15] {
            a.world.as_mut().unwrap().system.index = i;
            let mut t = Terminal::new(TestBackend::new(x, y)).unwrap();
            t.draw(|f| stoneage_client::ui::render(f, &mut a)).unwrap();
            let text = t
                .backend()
                .buffer()
                .content
                .iter()
                .map(|c| c.symbol())
                .collect::<String>();
            assert!(
                text.replace(' ', "")
                    .contains(&stoneage_client::app::system::ROWS[i].replace(' ', "")),
                "{x} {y} index {i}"
            );
        }
    }
    assert!(a.world.as_ref().unwrap().nav.is(&Layer::Page(SYSTEM_MENU)));
}

/// STAGE-173：回放三档 1× → 2× → 跳过 → 1×，战斗中不再有 f 键。
#[test]
fn playback_cycles_through_three_modes() {
    let mut a = app();
    a.world.as_mut().unwrap().system.index = 9;
    assert_eq!(a.playback_label(), "1×");
    key(&mut a, KeyCode::Enter);
    assert_eq!(a.playback_label(), "2×");
    key(&mut a, KeyCode::Enter);
    assert_eq!(a.playback_label(), "跳过");
    assert!(a.skip_replay, "跳过档：回放不播动画");
    key(&mut a, KeyCode::Enter);
    assert_eq!(a.playback_label(), "1×");
    assert!(!a.skip_replay);
}
