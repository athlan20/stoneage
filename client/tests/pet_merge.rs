//! 宠物合成界面（STAGE-208）：技能页入口、勾选材料、混合与件数拦截、默认取消的确认、结果页、80×24 渲染。
//! 不连服务端：真正发出去的请求和结果见 `scenarios_pet_merge.rs`。

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::{
    App, Confirm, Layer, MergeKind, MergeReport, Options, PETS_MENU, PetStep, Screen, WorldState, key_hint,
};
use stoneage_client::proto::{Inventory, InventorySlot, Pet, PetSkill, Pets, TargetType, WorldEntry};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn item(slot: u8, name: &str, kind: u32) -> InventorySlot {
    InventorySlot {
        slot,
        item_id: i32::from(slot),
        name: name.into(),
        kind,
        count: 1,
        ..Default::default()
    }
}

/// 一只会攻击、加工、料理的宠物，背包里 5、6 是食物，7、8 是道具。
fn merge_app() -> App {
    let mut a = App::new(Options {
        server: "127.0.0.1:7065".into(),
        unicode: true,
        ..Options::default()
    });
    let mut w = WorldState::new(WorldEntry::default());
    w.menu = PETS_MENU;
    w.nav.push(Layer::Page(PETS_MENU));
    let skills = vec![
        PetSkill {
            slot: 1,
            id: 1,
            name: "攻击".into(),
            target_type: TargetType::Enemy,
        },
        PetSkill {
            slot: 2,
            id: 200,
            name: "加工".into(),
            target_type: TargetType::None,
        },
        PetSkill {
            slot: 3,
            id: 201,
            name: "料理".into(),
            target_type: TargetType::None,
        },
    ];
    let pet = Pet {
        slot: 0,
        name: "乌力".into(),
        state: "rest".into(),
        hp: 20,
        max_hp: 40,
        level: 5,
        skills,
        ..Pet::default()
    };
    w.pets = Pets {
        max_slots: 5,
        pets: vec![pet],
    };
    w.inventory = Inventory {
        gold: 0,
        slots: vec![
            item(5, "小块肉", 20),
            item(6, "大块肉", 20),
            item(7, "小斧头", 1),
            item(8, "石头", 3),
        ],
    };
    a.world = Some(w);
    a.screen = Screen::World;
    a
}

fn key(a: &mut App, code: KeyCode) {
    a.on_key(KeyEvent::new(code, KeyModifiers::NONE));
}

fn keys(a: &mut App, codes: &[KeyCode]) {
    for c in codes {
        key(a, *c);
    }
}

fn world(a: &App) -> &WorldState {
    a.world.as_ref().expect("已进入世界")
}

fn logged(a: &App, s: &str) -> bool {
    a.log.all().iter().any(|m| m.text.contains(s))
}

/// 宠物页 → 动作 → 技能，光标移到第 `skill_row` 行（0 起）。
fn open_skills(a: &mut App, skill_row: usize) {
    keys(
        a,
        &[
            KeyCode::Enter,
            KeyCode::Down,
            KeyCode::Down,
            KeyCode::Down,
            KeyCode::Down,
            KeyCode::Down,
            KeyCode::Enter,
        ],
    );
    for _ in 0..skill_row {
        key(a, KeyCode::Down);
    }
}

/// 提示行的文字（各段拼在一起）。
fn hint(a: &App) -> String {
    key_hint(world(a)).join(" ")
}

fn picked(a: &App) -> Vec<u8> {
    match world(a).pet_step.as_ref() {
        Some(PetStep::Merge { picked, .. }) => picked.clone(),
        other => panic!("不在选材料步骤：{other:?}"),
    }
}

fn draw(app: &mut App, w: u16, h: u16) -> String {
    let mut term = Terminal::new(TestBackend::new(w, h)).unwrap();
    term.draw(|f| ui::render(f, app)).unwrap();
    let buf = term.backend().buffer().clone();
    (0..h)
        .map(|y| {
            let mut line = String::new();
            let mut x = 0;
            while x < w {
                let sym = buf[(x, y)].symbol();
                line.push_str(sym);
                x += UnicodeWidthStr::width(sym).max(1) as u16;
            }
            line
        })
        .collect::<Vec<_>>()
        .join("\n")
}

#[test]
fn enter_on_a_battle_skill_only_explains() {
    let mut a = merge_app();
    open_skills(&mut a, 0);
    key(&mut a, KeyCode::Enter);
    assert!(
        matches!(world(&a).pet_step, Some(PetStep::Skills { .. })),
        "攻击不进合成"
    );
    assert!(logged(&a, "不用材料"));
}

#[test]
fn enter_on_process_opens_the_material_picker_and_esc_returns_to_that_skill() {
    let mut a = merge_app();
    open_skills(&mut a, 1);
    key(&mut a, KeyCode::Enter);
    assert!(matches!(
        world(&a).pet_step,
        Some(PetStep::Merge { slot: 0, skill: 2, .. })
    ));
    assert_eq!(world(&a).pet_step_rows(), 4, "背包里 4 件东西都能选");
    key(&mut a, KeyCode::Esc);
    assert!(
        matches!(world(&a).pet_step, Some(PetStep::Skills { slot: 0, index: 1 })),
        "回到加工那一行"
    );
}

#[test]
fn space_picks_in_order_and_toggles_off() {
    let mut a = merge_app();
    open_skills(&mut a, 1);
    keys(
        &mut a,
        &[KeyCode::Enter, KeyCode::Down, KeyCode::Down, KeyCode::Char(' ')],
    );
    keys(&mut a, &[KeyCode::Up, KeyCode::Up, KeyCode::Char(' ')]);
    assert_eq!(picked(&a), vec![7, 5], "按勾选顺序，不是格号顺序");
    keys(&mut a, &[KeyCode::Char(' ')]);
    assert_eq!(picked(&a), vec![7], "再按空格取消");
}

#[test]
fn too_few_or_mixed_materials_never_reach_the_confirm() {
    let mut a = merge_app();
    open_skills(&mut a, 1);
    keys(&mut a, &[KeyCode::Enter, KeyCode::Char(' '), KeyCode::Enter]);
    assert!(logged(&a, "至少要选 2 件"));
    assert!(world(&a).confirm.is_none());
    // 5 是食物，再勾 7 的小斧头：混合。
    keys(
        &mut a,
        &[KeyCode::Down, KeyCode::Down, KeyCode::Char(' '), KeyCode::Enter],
    );
    assert!(logged(&a, "食物和道具不能混着合成"));
    assert!(world(&a).confirm.is_none(), "混合在本地拦下");
    assert!(
        matches!(world(&a).pet_step, Some(PetStep::Merge { .. })),
        "留在选材料里改"
    );
}

#[test]
fn enough_materials_ask_a_default_cancel_confirm_and_cancel_keeps_the_picks() {
    let mut a = merge_app();
    open_skills(&mut a, 2);
    keys(
        &mut a,
        &[
            KeyCode::Enter,
            KeyCode::Char(' '),
            KeyCode::Down,
            KeyCode::Char(' '),
            KeyCode::Enter,
        ],
    );
    match world(&a).confirm.as_ref() {
        Some(Confirm::PetMerge {
            slot: 0,
            skill: 3,
            items,
            names,
            ..
        }) => {
            assert_eq!(items, &vec![5, 6]);
            assert_eq!(names, &vec!["小块肉".to_string(), "大块肉".to_string()]);
        }
        other => panic!("应当在等合成确认：{other:?}"),
    }
    assert!(logged(&a, "用料理合成：小块肉、大块肉（共 2 件"));
    // 默认取消：直接 Enter 取消，回到选材料，勾选还在。
    key(&mut a, KeyCode::Enter);
    assert!(world(&a).confirm.is_none());
    assert!(logged(&a, "已取消合成"));
    assert_eq!(picked(&a), vec![5, 6]);
}

#[test]
fn all_fifteen_slots_is_the_ceiling() {
    let mut a = merge_app();
    let w = a.world.as_mut().unwrap();
    w.inventory.slots = (5..=19).map(|n| item(n, "木头", 3)).collect();
    open_skills(&mut a, 1);
    key(&mut a, KeyCode::Enter);
    for _ in 0..15 {
        keys(&mut a, &[KeyCode::Char(' '), KeyCode::Down]);
    }
    assert_eq!(picked(&a).len(), 15);
    key(&mut a, KeyCode::Up);
    keys(&mut a, &[KeyCode::Char(' ')]);
    assert_eq!(picked(&a).len(), 14, "取消一件");
}

#[test]
fn result_page_continues_or_returns() {
    let mut a = merge_app();
    open_skills(&mut a, 1);
    key(&mut a, KeyCode::Enter);
    let report = MergeReport {
        kind: MergeKind::Created,
        lines: vec!["加工成功：合成枪1".into(), "用掉了：A、B".into()],
    };
    a.world.as_mut().unwrap().pet_step = Some(PetStep::MergeResult {
        slot: 0,
        skill: 2,
        report,
    });
    key(&mut a, KeyCode::Enter);
    assert!(
        matches!(world(&a).pet_step, Some(PetStep::Merge { skill: 2, .. })),
        "Enter 再合成"
    );
    assert!(picked(&a).is_empty());
    a.world.as_mut().unwrap().pet_step = Some(PetStep::MergeResult {
        slot: 0,
        skill: 2,
        report: MergeReport {
            kind: MergeKind::Fallback,
            lines: vec!["x".into()],
        },
    });
    key(&mut a, KeyCode::Esc);
    assert!(matches!(
        world(&a).pet_step,
        Some(PetStep::Skills { slot: 0, index: 1 })
    ));
}

#[test]
fn hints_follow_the_step() {
    let mut a = merge_app();
    open_skills(&mut a, 0);
    assert!(!hint(&a).contains("Enter 选材料"), "攻击上没有选材料");
    key(&mut a, KeyCode::Down);
    assert!(hint(&a).contains("Enter 选材料"));
    key(&mut a, KeyCode::Enter);
    let hint = hint(&a);
    assert!(hint.contains("空格 勾选") && hint.contains("Enter 合成"), "{hint}");
}

#[test]
fn picker_and_result_fit_in_80x24_with_both_glyph_sets() {
    for unicode in [true, false] {
        let mut a = merge_app();
        a.unicode = unicode;
        a.world.as_mut().unwrap().inventory.slots = (5..=19).map(|n| item(n, "迪所斯Lv1斧头", 1)).collect();
        open_skills(&mut a, 1);
        keys(&mut a, &[KeyCode::Enter, KeyCode::Char(' ')]);
        let screen = draw(&mut a, 80, 24);
        assert!(screen.contains("加工选材料 · 乌力"), "{screen}");
        assert!(screen.contains("[1]  5 迪所斯Lv1斧头"), "已勾选显示顺序号\n{screen}");
        assert!(screen.contains("已选 1 件"), "{screen}");
        key(&mut a, KeyCode::End);
        let screen = draw(&mut a, 80, 24);
        assert!(screen.contains("19 迪所斯Lv1斧头"), "光标在末尾时看得到末尾\n{screen}");
        a.world.as_mut().unwrap().pet_step = Some(PetStep::MergeResult {
            slot: 0,
            skill: 2,
            report: MergeReport {
                kind: MergeKind::Cooldown,
                lines: vec![
                    "合成料理过於频繁，休息一下比较好哟。".into(),
                    "材料照样用掉了，还回一件：斧头".into(),
                    "用掉了：斧头、斧头".into(),
                ],
            },
        });
        let screen = draw(&mut a, 80, 24);
        assert!(
            screen.contains("过於频繁") && screen.contains("Enter 再合成"),
            "{screen}"
        );
    }
}
