use crossterm::event::{KeyCode,KeyEvent,KeyModifiers};
use ratatui::{Terminal,backend::TestBackend};
use stoneage_client::{app::{App,Options,WorldState,Screen},proto::{WorldEntry,NpcWindow,NpcOption,NpcInput,NpcTrade}};

fn app() -> App {
    let mut a=App::new(Options::default());
    a.world=Some(WorldState::new(WorldEntry::default()));a.screen=Screen::World;a
}
fn key(a:&mut App,k:KeyCode){a.on_key(KeyEvent::new(k,KeyModifiers::NONE));}
fn window(a:&mut App)->&mut NpcWindow{a.world.as_mut().unwrap().npc_window.get_or_insert_with(||NpcWindow{window_id:"current".into(),..Default::default()})}

#[test]
fn menu_reaches_all_33_items_without_hidden_typing(){
    let mut a=app();window(&mut a).options=(1..=33).map(|no|NpcOption{no,label:format!("Product{no}")}).collect();
    key(&mut a,KeyCode::End);
    assert_eq!(a.world.as_ref().unwrap().dialog.index,32);
    key(&mut a,KeyCode::Char('x'));
    assert!(a.world.as_ref().unwrap().input.is_empty());
    for (w,h) in [(80,24),(110,30),(120,40),(200,60)]{
        let mut t=Terminal::new(TestBackend::new(w,h)).unwrap();
        t.draw(|f|stoneage_client::ui::render(f,&mut a)).unwrap();
        let rendered=t.backend().buffer().content.iter().map(|c|c.symbol()).collect::<String>();
        assert!(rendered.contains("Product33"),"{w}x{h}: {rendered}");
    }
}

#[test]
fn keyword_form_keeps_spaces_slash_and_fullwidth(){
    let mut a=app();window(&mut a).input=Some(NpcInput{kind:"text".into(),max_chars:220,..Default::default()});
    for c in " Ab /暗号Ａ ".chars(){key(&mut a,KeyCode::Char(c));}
    assert_eq!(a.world.as_ref().unwrap().dialog.input.text()," Ab /暗号Ａ ");
    assert!(a.world.as_ref().unwrap().input.is_empty());
}

#[test]
fn trade_starts_with_cancel_and_switches_only_explicitly(){
    let mut a=app();window(&mut a).trade=Some(NpcTrade{count:2,total:50,..Default::default()});
    assert!(!a.world.as_ref().unwrap().dialog.confirm);
    key(&mut a,KeyCode::Char('y'));assert!(!a.world.as_ref().unwrap().dialog.confirm);
    key(&mut a,KeyCode::Right);assert!(a.world.as_ref().unwrap().dialog.confirm);
}

#[test]
fn invalid_quantity_stays_in_form(){
    for value in ["0","-1","1.5","16"]{
        let mut a=app();window(&mut a).input=Some(NpcInput{kind:"integer".into(),min:1,max:15,max_chars:10});
        a.world.as_mut().unwrap().dialog.input.set(value);
        key(&mut a,KeyCode::Enter);
        assert_eq!(a.world.as_ref().unwrap().dialog.input.text(),value);
    }
}
