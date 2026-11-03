//! 动作层的界面测试（STAGE-167）：精灵形态落在真实目标上、治疗与异常各有形态、
//! 换宠先收回主人身上再放出、反击时攻击者停在接触现场。不经过网络，直接搭战场和动作层。

use std::time::{Duration, Instant};

use ratatui::Terminal;
use ratatui::backend::TestBackend;
use stoneage_client::app::battle::{
    BattleState, Elem, Flown, Motion, SpellFx, SpellKind, Style as MotionStyle, SwapFx,
};
use stoneage_client::app::{App, Layer, Options, Screen, WorldState};
use stoneage_client::proto::{
    Appearance, BattleAllow, BattleStart, BattleUnit, Character, Location, WorldEntry,
};
use stoneage_client::ui;
use unicode_width::UnicodeWidthStr;

fn app(unicode: bool) -> App {
    App::new(Options { server: "127.0.0.1:7065".into(), unicode, ..Options::default() })
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

fn unit(no: u8, kind: &str, name: &str) -> BattleUnit {
    BattleUnit { no, kind: kind.into(), name: name.into(), level: 5, hp_pct: 100, ..BattleUnit::default() }
}

/// 我方：勇者（0）、乌力（5）；敌方：哥布林（10）、两只野猪（15、16）。
fn battle(units: Vec<BattleUnit>) -> BattleState {
    let start = BattleStart { party: None,
            field: None,
        battle_id: "b_1".into(),
        kind: "wild".into(),
        scene: "野外".into(),
        you: 0,
        allow: BattleAllow::default(),
        units,
        npc: None,
    };
    BattleState::from_start(start)
}

fn base() -> Vec<BattleUnit> {
    vec![
        unit(0, "player", "勇者小明"),
        unit(5, "pet", "乌力"),
        unit(10, "monster", "哥布林"),
        unit(15, "monster", "野猪"),
        unit(16, "monster", "野猪乙"),
    ]
}

fn battle_app(b: BattleState) -> App {
    let mut a = app(true);
    let mut entry = WorldEntry::default();
    entry.character = Character {
        slot: 0,
        name: "勇者小明".into(),
        level: 12,
        appearance: Appearance { body: 3, color: 2, eye: 1, mouth: 4 },
        location: Location { floor: 1000, map_name: "萨姆吉尔村".into() },
    };
    let mut world = WorldState::new(entry);
    world.nav.push(Layer::Page(0));
    world.battle = Some(b);
    a.world = Some(world);
    a.screen = Screen::World;
    a
}

/// 一帧里某个符号出现的次数与位置（行、列），列按显示宽度算。
fn find(screen: &str, needle: &str) -> Vec<(usize, usize)> {
    let mut out = Vec::new();
    for (row, line) in screen.lines().enumerate() {
        let mut col = 0;
        let mut rest = line;
        while let Some(i) = rest.find(needle) {
            let before = &rest[..i];
            out.push((row, col + UnicodeWidthStr::width(before)));
            col += UnicodeWidthStr::width(&rest[..i + needle.len()]);
            rest = &rest[i + needle.len()..];
        }
    }
    out
}

fn spell(kind: SpellKind, elem: Elem, targets: Vec<u8>) -> SpellFx {
    let now = Instant::now();
    SpellFx {
        caster: 0,
        targets,
        kind,
        elem,
        all: false,
        status: None,
        start: now - Duration::from_millis(100),
        hit: now - Duration::from_millis(10),
        end: now + Duration::from_millis(600),
    }
}

#[test]
fn element_spells_land_on_the_real_target_with_their_own_shape() {
    for (elem, glyph) in [(Elem::Fire, '♨'), (Elem::Earth, '▲'), (Elem::Water, '≈'), (Elem::Wind, '≋')] {
        let mut b = battle(base());
        b.spells.push(spell(SpellKind::Damage, elem, vec![15]));
        let mut a = battle_app(b);
        let s = draw(&mut a, 120, 40);
        let marks = find(&s, &glyph.to_string());
        assert!(!marks.is_empty(), "属性 {elem:?} 应在目标身上留下 {glyph}\n{s}");
        // 只落在被作用的那一个目标上：野猪乙（16）不能被一起点亮。
        let on_16 = find(&s, "野猪乙");
        assert!(!on_16.is_empty(), "另一只野猪还在场上");
        assert_eq!(marks.len(), 1, "单体精灵只标一个目标：{marks:?}\n{s}");
    }
}

#[test]
fn all_targets_spell_marks_every_effected_unit_only() {
    let mut b = battle(base());
    let mut fx = spell(SpellKind::Damage, Elem::Fire, vec![10, 15, 16]);
    fx.all = true;
    b.spells.push(fx);
    let mut a = battle_app(b);
    let s = draw(&mut a, 120, 40);
    assert_eq!(find(&s, "♨").len(), 3, "三个被作用的目标各一个标记\n{s}");
    assert!(find(&s, "乌力").len() >= 1, "己方没被作用，不标");
}

#[test]
fn heal_rises_and_is_not_drawn_as_a_damage_element() {
    let mut b = battle(base());
    b.spells.push(spell(SpellKind::HealSelf, Elem::None, vec![0]));
    let mut a = battle_app(b);
    let s = draw(&mut a, 120, 40);
    assert_eq!(find(&s, "↑").len(), 1, "自疗只在施法者自己身上\n{s}");
    assert!(find(&s, "▲").is_empty() && find(&s, "♨").is_empty(), "治疗不能画成攻击属性");
}

#[test]
fn revive_and_cure_have_their_own_marks() {
    let mut b = battle(base());
    b.spells.push(spell(SpellKind::Revive, Elem::None, vec![15]));
    let mut a = battle_app(b);
    let s = draw(&mut a, 120, 40);
    assert_eq!(find(&s, "✝").len(), 1, "复活用十字，不是回血箭头\n{s}");

    let mut b = battle(base());
    let mut cure = spell(SpellKind::Cure, Elem::None, vec![16]);
    cure.status = Some("poison".into());
    b.spells.push(cure);
    let mut a = battle_app(b);
    let s = draw(&mut a, 120, 40);
    assert_eq!(find(&s, "×").len(), 1, "净化是分解，不是回血\n{s}");
}

#[test]
fn status_spells_show_the_shape_of_that_status() {
    for (status, glyph) in [("poison", '☠'), ("paralysis", 'ϟ'), ("sleep", 'z'), ("stone", '▓')] {
        let mut b = battle(base());
        let mut fx = spell(SpellKind::Status, Elem::None, vec![15]);
        fx.status = Some(status.into());
        b.spells.push(fx);
        let mut a = battle_app(b);
        let s = draw(&mut a, 120, 40);
        assert!(!find(&s, &glyph.to_string()).is_empty(), "{status} 应有自己的施加形态 {glyph}\n{s}");
    }
}

#[test]
fn old_pet_stays_until_it_reaches_the_owner_then_the_new_one_appears() {
    let now = Instant::now();
    let mut b = battle(base());
    b.swaps.push(SwapFx {
        owner: 0,
        old: Some(5),
        new: None,
        start: now - Duration::from_millis(100),
        hit: now + Duration::from_millis(150),
        end: now + Duration::from_millis(600),
    });
    let mut a = battle_app(b);
    let s = draw(&mut a, 120, 40);
    assert!(!find(&s, "乌力").is_empty(), "收回途中旧宠物还没消失，名字、血量都留着\n{s}");

    // 命中拍之后：旧宠物已经从场上移除，新宠物从主人处出现。
    let now = Instant::now();
    let mut b = battle(base());
    b.here.remove(&5);
    b.units.insert(6, unit(6, "pet", "贝鲁卡"));
    b.here.insert(6);
    b.swaps.push(SwapFx {
        owner: 0,
        old: Some(5),
        new: Some(6),
        start: now - Duration::from_millis(300),
        hit: now - Duration::from_millis(40),
        end: now + Duration::from_millis(400),
    });
    let mut a = battle_app(b);
    let s = draw(&mut a, 120, 40);
    assert!(find(&s, "乌力").is_empty(), "旧宠物收起后不能还留在格子上\n{s}");
    assert!(!find(&s, "贝鲁卡").is_empty(), "新宠物从主人处放出\n{s}");
}

#[test]
fn a_pet_returns_to_its_own_owner_not_to_the_local_player() {
    // 敌方 14 号是哥布林的主人，15 号是它的宠物；换宠要朝 14 收，不能朝本地玩家 0。
    let mut units = base();
    units.push(unit(14, "monster", "哥布林首领"));
    let now = Instant::now();
    let mut b = battle(units);
    b.swaps.push(SwapFx {
        owner: 14,
        old: Some(15),
        new: None,
        start: now - Duration::from_millis(200),
        hit: now + Duration::from_millis(50),
        end: now + Duration::from_millis(500),
    });
    let mut a = battle_app(b);
    let s = draw(&mut a, 120, 40);
    let pet = find(&s, "野猪");
    let owner = find(&s, "哥布林");
    assert!(!pet.is_empty() && !owner.is_empty(), "{s}");
    // 收回途中宠物卡片的列应该落在主人的列附近（沿水平方向移动），而不是跑向己方那一排。
    let (pet_row, pet_col) = pet[0];
    let (owner_row, owner_col) = owner[0];
    assert!(pet_row != owner_row || pet_col != owner_col);
    assert!(pet_col + 20 > owner_col, "宠物朝自己的主人收，不朝本地玩家（{pet_col} vs {owner_col}）");
}

#[test]
fn counter_keeps_the_attacker_at_the_contact_point() {
    let now = Instant::now();
    let start = now - Duration::from_millis(400);
    let hit = now - Duration::from_millis(150);
    let ev = stoneage_client::proto::BattleEvent {
        kind: "attack".into(),
        actor: Some(0),
        weapon: "axe".into(),
        effects: vec![stoneage_client::proto::BattleEffect {
            target: 15,
            result: "hit".into(),
            ..Default::default()
        }],
        ..Default::default()
    };
    let holding = Motion::from_event_holding(&ev, start, hit, now + Duration::from_millis(400), true)
        .expect("有出手者和目标");
    assert_eq!(holding.style, MotionStyle::Melee);
    assert_eq!(holding.approach(now), 1.0, "反击期间攻击者停在接触现场，不退回出生位");

    let normal = Motion::from_event(&ev, start, hit, now + Duration::from_millis(400)).expect("同上");
    assert!(normal.approach(now) < 1.0, "没有反击时照常回位");
}

#[test]
fn sixty_by_twenty_four_still_shows_the_marks_inside_the_field() {
    let mut b = battle(base());
    b.spells.push(spell(SpellKind::Damage, Elem::Fire, vec![15]));
    let mut a = battle_app(b);
    let s = draw(&mut a, 80, 24);
    assert!(!find(&s, "♨").is_empty(), "窄窗也要能看见属性形态\n{s}");
    assert!(s.contains("野猪"), "{s}");
}

#[test]
fn unicode_off_falls_back_to_ascii_marks() {
    let mut b = battle(base());
    b.spells.push(spell(SpellKind::Damage, Elem::Earth, vec![15]));
    let mut a = battle_app(b);
    // 关掉 Unicode：地属性退成 ^，不能还画 ▲。
    a.unicode = false;
    let s = draw(&mut a, 120, 40);
    assert!(!find(&s, "^").is_empty(), "关掉 Unicode 时用 ASCII 形态\n{s}");
    assert!(find(&s, "▲").is_empty(), "关掉 Unicode 不画 Unicode 符号\n{s}");
}

#[test]
fn reduce_motion_drops_the_projectile_but_keeps_the_contact_mark() {
    let now = Instant::now();
    let ev = stoneage_client::proto::BattleEvent {
        kind: "attack".into(),
        actor: Some(0),
        weapon: "bow".into(),
        effects: vec![stoneage_client::proto::BattleEffect {
            target: 15,
            result: "hit".into(),
            ..Default::default()
        }],
        ..Default::default()
    };
    let m = Motion::from_event(
        &ev,
        now - Duration::from_millis(300),
        now - Duration::from_millis(40),
        now + Duration::from_millis(300),
    )
    .expect("有出手者和目标");
    let mut b = battle(base());
    b.motions.push(m);
    let mut a = battle_app(b);
    a.reduce_motion = true;
    let s = draw(&mut a, 120, 40);
    assert!(find(&s, "➤").is_empty(), "减少动态效果时不画飞行物\n{s}");
    assert!(!find(&s, "·").is_empty(), "接触点仍然要画：这是判断命中的依据\n{s}");
}

#[test]
fn flown_unit_slides_outward_and_keeps_the_arrow() {
    // STAGE-248：120×40 的日志区占 9 行后，战场只剩 22 行，落到紧凑档；这里要测的是完整档的写法，终端高度改用 44。
    let plain = draw(&mut battle_app(battle(base())), 120, 44);
    // 15 号在上、16 号在下，第一个匹配就是打飞的这只（base() 里等级都是 5）。
    let before = find(&plain, "Lv.5 野猪").first().copied().expect("野猪在场上");

    let now = Instant::now();
    let mut b = battle(base());
    // 400ms 前被打飞，方向往右：应该在飞出去的途中，还没到边缘。
    b.flown.insert(15, Flown { at: now - Duration::from_millis(400), dir: 1 });
    let s = draw(&mut battle_app(b), 120, 44);
    let after = find(&s, "野猪").into_iter().find(|&(row, _)| row == before.0).expect("打飞途中身体标签还在原来的行");
    assert!(after.1 > before.1, "沿攻击方向往右飞：{before:?} -> {after:?}\n{s}");
    assert!(!find(&s, "↗").is_empty(), "打飞要留离场标记，和普通倒下分开\n{s}");
}
