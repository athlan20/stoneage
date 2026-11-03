use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::{
    app::{App, Options, Screen},
    proto::{Character, CharacterList},
};

fn key(a: &mut App, k: KeyCode) {
    a.on_key(KeyEvent::new(k, KeyModifiers::NONE));
}

#[test]
fn existing_character_opens_actions_then_visible_delete_form() {
    let mut a = App::new(Options::default());
    a.screen = Screen::Characters;
    a.chars.list = CharacterList {
        max_slots: 2,
        characters: vec![Character {
            name: "勇者小明".into(),
            ..Default::default()
        }],
    };
    key(&mut a, KeyCode::Enter);
    assert_eq!(a.chars.actions, Some(0));
    key(&mut a, KeyCode::Down);
    key(&mut a, KeyCode::Enter);
    assert!(a.chars.confirm.is_some());
    a.chars.confirm.as_mut().unwrap().input.set("勇者小明 ");
    key(&mut a, KeyCode::Enter);
    assert!(
        !a.chars.confirm.as_ref().unwrap().buttons,
        "不能 trim 名字后误删"
    );
    a.chars.confirm.as_mut().unwrap().input.set("勇者小明");
    key(&mut a, KeyCode::Enter);
    assert!(a.chars.confirm.as_ref().unwrap().buttons);
    assert!(!a.chars.confirm.as_ref().unwrap().delete);
    key(&mut a, KeyCode::Enter);
    assert!(a.chars.confirm.is_none(), "默认取消，不发删除");
}

#[test]
fn ctrl_c_requires_visible_confirmation_on_every_screen() {
    for screen in [
        Screen::Login,
        Screen::Register,
        Screen::Characters,
        Screen::Create,
        Screen::World,
    ] {
        let mut a = App::new(Options::default());
        a.screen = screen;
        a.on_key(KeyEvent::new(KeyCode::Char('c'), KeyModifiers::CONTROL));
        assert!(a.exit_dialog.is_some());
        assert!(!a.should_quit);
        key(&mut a, KeyCode::Enter);
        assert!(!a.should_quit);
        assert!(a.exit_dialog.is_none());
        a.on_key(KeyEvent::new(KeyCode::Char('c'), KeyModifiers::CONTROL));
        key(&mut a, KeyCode::Right);
        key(&mut a, KeyCode::Enter);
        assert!(a.should_quit);
    }
}
