//! Response races and cache invalidation exercise the real App response handlers.
use super::{App, LoadState, Options, SceneQuery, WorldState, session::Pending};
use crate::proto::WorldEntry;
use serde_json::json;
fn app() -> App {
    let mut a = App::new(Options::default());
    a.world = Some(WorldState::new(WorldEntry::default()));
    a.world.as_mut().unwrap().entry.position.floor = 2000;
    a
}
fn query(a: &App, npc: bool) -> SceneQuery {
    let w = a.world.as_ref().unwrap();
    SceneQuery {
        floor: w.entry.position.floor,
        epoch: w.scene.epoch,
        query: if npc {
            w.scene.npc_query
        } else {
            w.scene.exit_query
        },
    }
}
fn exits(floor: u32, ids: &[&str]) -> serde_json::Value {
    json!({"floor":floor,"exits":ids.iter().map(|id|json!({"id":id,"to_floor":100,"to_name":id,"trigger_cells":[{"x":1,"y":2}],"time_gate":"all","available":true,"unavailable_reason":null,"one_way":false})).collect::<Vec<_>>(),"server_time_ms":1000,"next_change_ms":2000})
}
#[test]
fn a_b_a_old_success_and_error_never_replace_new_lists() {
    let mut a = app();
    let old = query(&a, false);
    let old_npc = query(&a, true);
    {
        let w = a.world.as_mut().unwrap();
        w.entry.position.floor = 100;
        w.scene.invalidate();
        w.entry.position.floor = 2000;
        w.scene.invalidate();
    }
    let fresh = query(&a, false);
    a.on_ok(Pending::Exits(fresh), exits(2000, &["new"]));
    a.on_ok(Pending::Exits(old), exits(2000, &["old"]));
    a.on_error(
        Pending::Exits(old),
        crate::proto::ProtoError {
            code: "INTERNAL".into(),
            message: "late".into(),
            ..Default::default()
        },
    );
    a.on_ok(
        Pending::Npcs(old_npc),
        json!({"floor":2000,"npcs":[{"id":99}]}),
    );
    let w = a.world.as_ref().unwrap();
    assert_eq!(w.scene.exits[0].id, "new");
    assert_eq!(w.scene.exit_load, LoadState::Ready);
    assert!(w.npcs.is_empty());
    assert!(!a.log.all().iter().any(|m| m.text.contains("late")));
}
#[test]
fn same_map_refresh_keeps_stable_selection_and_rejects_wrong_floor() {
    let mut a = app();
    let c = query(&a, false);
    a.on_ok(Pending::Exits(c), exits(2000, &["a", "b", "c"]));
    a.world.as_mut().unwrap().scene_index = 1;
    a.world.as_mut().unwrap().scene.exit_query += 1;
    let fresh = query(&a, false);
    a.on_ok(Pending::Exits(fresh), exits(2000, &["b", "a"]));
    assert_eq!(a.world.as_ref().unwrap().scene_index, 0);
    a.on_ok(Pending::Exits(c), exits(2000, &["stale"]));
    assert_eq!(a.world.as_ref().unwrap().scene.exits[0].id, "b");
    a.on_ok(Pending::Exits(fresh), exits(100, &["wrong"]));
    assert!(matches!(
        a.world.as_ref().unwrap().scene.exit_load,
        LoadState::Failed(_)
    ));
    assert_eq!(a.world.as_ref().unwrap().scene.exits[0].id, "b");
    a.on_ok(Pending::Exits(fresh), json!({"floor":2000}));
    assert!(matches!(
        a.world.as_ref().unwrap().scene.exit_load,
        LoadState::Failed(_)
    ));
}
#[test]
fn offline_invalidates_pending_and_clears_exit_submission() {
    let mut a = app();
    let c = query(&a, false);
    a.world.as_mut().unwrap().scene.using_exit = true;
    a.world.as_mut().unwrap().readonly();
    a.on_ok(Pending::Exits(c), exits(2000, &["late"]));
    let w = a.world.as_ref().unwrap();
    assert!(w.scene.exits.is_empty());
    assert!(!w.scene.using_exit);
    assert!(!w.online);
}

#[test]
fn pending_initial_load_preserves_a_fixed_entry_selected_by_user() {
    let mut a = app();
    {
        let w = a.world.as_mut().unwrap();
        w.scene.exit_query = 1;
        w.scene.left_touched = true;
        w.scene_index = 2;
    }
    let c = query(&a, false);
    a.on_ok(Pending::Exits(c), exits(2000, &["door"]));
    assert_eq!(
        a.world.as_ref().unwrap().scene_index,
        3,
        "回城仍然选中，分隔线不算导航项"
    );
}
#[test]
fn malformed_reconnect_reply_restores_readonly_snapshot() {
    let mut a = app();
    a.world.as_mut().unwrap().entry.character.name = "勇者".into();
    a.begin_reconnect();
    assert!(a.stage_reconnect("勇者", false));
    a.world = Some(WorldState::new(WorldEntry::default()));
    a.world.as_mut().unwrap().online = false;
    let c = query(&a, false);
    a.on_ok(Pending::Exits(c), json!({"floor":c.floor}));
    assert_eq!(a.world.as_ref().unwrap().entry.character.name, "勇者");
    assert!(!a.world.as_ref().unwrap().online);
}
