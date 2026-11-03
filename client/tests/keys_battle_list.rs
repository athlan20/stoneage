//! 列表面板的按键（STAGE-107，`docs/battle-ui.md`「编号规则」「列表面板」）：道具用 ↑↓ 和 Enter，PgUp、PgDn 翻页，
//! 精灵、换宠、技能按栏位号直选；空栏位和灰色项留在列表里说明原因；收回宠物只能用方向键选。不连服务端，只走到“发出之前”。

use std::time::Instant;

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use stoneage_client::app::battle::{Act, BattleState, Pick};
use stoneage_client::app::{App, Kind, Options, Screen, WorldState};
use stoneage_client::proto::{
    BattleAllow, BattleStart, BattleTurn, BattleUnit, Inventory, InventorySlot, Pet, PetSkill, Progress, TargetType,
    WorldEntry,
};

fn unit(no: u8, kind: &str, name: &str) -> BattleUnit {
    BattleUnit {
        no,
        kind: kind.into(),
        name: name.into(),
        level: 3,
        hp_pct: 100,
        ..BattleUnit::default()
    }
}

fn item(slot: u8, name: &str, target: TargetType, effect: &str) -> InventorySlot {
    InventorySlot {
        slot,
        item_id: i32::from(slot),
        name: name.into(),
        count: 1,
        effect: effect.into(),
        target_type: target,
        ..InventorySlot::default()
    }
}

/// 背包 5–19 号各有一件：9 号是小斧头（战斗中不能用），12 号是回复药（回复类，默认高亮），其余是杂物（己方一个）。
fn bag() -> Vec<InventorySlot> {
    (5..=19u8)
        .map(|slot| match slot {
            9 => item(slot, "小斧头", TargetType::None, ""),
            12 => item(slot, "回复药", TargetType::Ally, "体100回复（1体）"),
            _ => item(slot, &format!("杂物{slot}"), TargetType::Ally, ""),
        })
        .collect()
}

/// 一场战斗：我方勇者和乌力，敌方两只野猪。背包和宠物由调用方给。
fn list_app(needs: &[&str], pets: Vec<Pet>) -> App {
    let mut a = App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    });
    let start = BattleStart {
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow {
            escape: true,
            capture: true,
        },
        units: vec![
            unit(0, "player", "勇者小明"),
            unit(5, "pet", "乌力"),
            unit(15, "monster", "野猪"),
            unit(16, "monster", "野猪"),
        ],
        ..BattleStart::default()
    };
    let mut battle = BattleState::from_start(start);
    battle.on_turn(
        BattleTurn {
                field: None,
            battle_id: "b_1".into(),
            turn: 1,
            deadline_ms: 1_030_000,
            server_time_ms: 1_000_000,
            needs: needs.iter().map(|s| s.to_string()).collect(),
            optional: vec![],
            progress: Progress { done: 0, total: 1 },
        },
        Instant::now(),
    );
    let mut world = WorldState::new(WorldEntry::default());
    world.nav.push(stoneage_client::app::Layer::Page(0));
    world.inventory = Inventory { gold: 0, slots: bag() };
    world.pets.pets = pets;
    world.battle = Some(battle);
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

fn pet(slot: u8, name: &str, state: &str) -> Pet {
    Pet {
        slot,
        name: name.into(),
        state: state.into(),
        hp: 20,
        max_hp: 20,
        level: 3,
        ..Pet::default()
    }
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn battle(a: &App) -> &BattleState {
    a.world.as_ref().unwrap().battle.as_ref().unwrap()
}

fn battle_mut(a: &mut App) -> &mut BattleState {
    a.world.as_mut().unwrap().battle.as_mut().unwrap()
}

fn last_error(a: &App) -> String {
    a.log
        .all()
        .iter()
        .rev()
        .find(|m| m.kind == Kind::Error)
        .map(|m| m.text.clone())
        .unwrap_or_default()
}

#[test]
fn inventory_push_invalidates_a_prepared_item_target() {
    let mut a=list_app(&["player"],vec![]);
    key(&mut a,KeyCode::Char('3'));key(&mut a,KeyCode::Enter);
    assert!(matches!(battle(&a).pick,Pick::Target {act:Act::Item,..}));
    a.world.as_mut().unwrap().inventory_epoch+=1;
    key(&mut a,KeyCode::Enter);
    assert!(last_error(&a).contains("背包已更新"));
    assert!(!battle(&a).submitting);
}

#[test]
fn items_start_on_the_last_used_one_then_the_first_heal_item() {
    let mut a = list_app(&["player"], vec![]);
    key(&mut a, KeyCode::Char('3'));
    assert_eq!(
        battle(&a).list_focus,
        7,
        "没有用过道具：高亮回复药（12 号，列表第 8 行）"
    );
    key(&mut a, KeyCode::Esc);
    battle_mut(&mut a).last_item = Some(17);
    key(&mut a, KeyCode::Char('3'));
    assert_eq!(
        battle(&a).list_focus,
        12,
        "本场上次用过 17 号（列表第 13 行），高亮落在它上面"
    );
    key(&mut a, KeyCode::Enter);
    assert!(
        matches!(
            battle(&a).pick,
            Pick::Target {
                act: Act::Item,
                extra: Some(17),
                kind: TargetType::Ally
            }
        ),
        "Enter 选中 17 号，要选己方一个"
    );
}

#[test]
fn page_keys_move_by_five_rows() {
    let mut a = list_app(&["player"], vec![]);
    key(&mut a, KeyCode::Char('3'));
    key(&mut a, KeyCode::PageDown);
    assert_eq!(battle(&a).list_focus, 12, "PgDn 下移 5 行（12 号到 17 号）");
    key(&mut a, KeyCode::PageUp);
    key(&mut a, KeyCode::PageUp);
    assert_eq!(battle(&a).list_focus, 2, "PgUp 上移 5 行，第一页的 0–4 行之间");
    key(&mut a, KeyCode::PageUp);
    assert_eq!(battle(&a).list_focus, 0, "不足一页时停在第一行，不循环");
}

#[test]
fn unusable_items_stay_in_the_list_and_say_why() {
    let mut a = list_app(&["player"], vec![]);
    key(&mut a, KeyCode::Char('3'));
    // 小斧头在第 5 行（列表下标 4），战斗中不能用：Enter 说明原因，留在列表里。
    battle_mut(&mut a).list_focus = 4;
    key(&mut a, KeyCode::Enter);
    assert!(last_error(&a).contains("小斧头战斗中不能用"), "{}", last_error(&a));
    assert_eq!(battle(&a).pick, Pick::Item);
    // 方向键跳过它：从 10 号（下标 5）往上，跳过灰色的 9 号，停在 8 号（下标 3）。
    battle_mut(&mut a).list_focus = 5;
    key(&mut a, KeyCode::Up);
    assert_eq!(battle(&a).list_focus, 3, "往上跳过灰色的小斧头");
}

#[test]
fn pet_list_numbers_are_the_pet_slots() {
    let mut a = list_app(&["player"], vec![pet(1, "波利", "standby")]);
    // 7 是换宠（指令条第 7 项）：列表里 1 号栏位是波利，0 号栏位是空的。
    key(&mut a, KeyCode::Char('7'));
    assert_eq!(battle(&a).pick, Pick::Pet);
    key(&mut a, KeyCode::Char('0'));
    assert!(last_error(&a).contains("0 号栏位没有宠物"), "{}", last_error(&a));
    assert_eq!(battle(&a).pick, Pick::Pet, "空栏位留在列表里");
    key(&mut a, KeyCode::Char('1'));
    assert!(!battle(&a).submitting && !a.world.as_ref().unwrap().online, "无连接时不能假装换宠成功");
}

#[test]
fn recall_is_only_reached_with_arrows() {
    let mut a = list_app(&["player"], vec![pet(0, "乌力", "battle"), pet(1, "波利", "standby")]);
    key(&mut a, KeyCode::Char('7'));
    assert_eq!(battle(&a).list_focus, 1, "默认高亮第一只能上场的宠物");
    key(&mut a, KeyCode::Char('0'));
    assert!(last_error(&a).contains("乌力已经在场上"), "{}", last_error(&a));
    assert_eq!(battle(&a).list_focus, 0, "数字键按到哪一行，高亮就在哪一行");
    key(&mut a, KeyCode::Down);
    assert_eq!(battle(&a).list_focus, 1, "往下跳过在场上的乌力，到波利");
    key(&mut a, KeyCode::Down);
    assert_eq!(battle(&a).list_focus, 20, "再往下跳过 2–19 号空栏位，到“收回出战宠物”");
    key(&mut a, KeyCode::Enter);
    assert!(!battle(&a).submitting && !a.world.as_ref().unwrap().online, "无连接时不能假装已收回宠物");
}

#[test]
fn skills_are_picked_by_their_skill_slot_even_with_gaps() {
    let skill = PetSkill {
        slot: 2,
        id: 1,
        name: "撞击".into(),
        target_type: TargetType::Enemy,
    };
    let mut battle_pet = pet(0, "乌力", "battle");
    battle_pet.skills = vec![skill];
    let mut a = list_app(&["pet"], vec![battle_pet]);
    key(&mut a, KeyCode::Char('2'));
    assert_eq!(battle(&a).pick, Pick::Skill, "宠物的 2 号是技能");
    key(&mut a, KeyCode::Char('1'));
    assert!(last_error(&a).contains("1 号栏位没有技能"), "{}", last_error(&a));
    key(&mut a, KeyCode::Char('2'));
    assert!(
        matches!(
            battle(&a).pick,
            Pick::Target {
                act: Act::Skill,
                extra: Some(2),
                kind: TargetType::Enemy
            }
        ),
        "2 号技能按栏位号直选，不是列表第二行"
    );
}
