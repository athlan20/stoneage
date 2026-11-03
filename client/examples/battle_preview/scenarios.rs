//! STAGE-167：离线验收用事件数据，所有场景走正式回放与渲染路径。
use stoneage_client::proto::*;
pub struct Case {
    pub name: String,
    pub events: Vec<BattleEvent>,
    pub full: bool,
}

pub fn unit(no: u8, name: &str) -> BattleUnit {
    BattleUnit {
        no,
        name: name.into(),
        kind: if no % 10 < 5 { "player" } else { "pet" }.into(),
        level: 12,
        hp_pct: 100,
        hp: (no < 10).then_some(100),
        max_hp: (no < 10).then_some(100),
        ..Default::default()
    }
}

fn effect(target: u8, result: &str) -> BattleEffect {
    BattleEffect {
        target,
        result: result.into(),
        delta: -23,
        hp_pct: Some(77),
        ..Default::default()
    }
}

fn attack(weapon: &str, target: u8) -> BattleEvent {
    BattleEvent {
        kind: "attack".into(),
        actor: Some(0),
        weapon: weapon.into(),
        effects: vec![effect(target, "hit")],
        ..Default::default()
    }
}

pub fn cases() -> Vec<Case> {
    let mut out = vec![];
    for (weapon, name) in [
        ("fist", "空手"),
        ("axe", "近战斧"),
        ("club", "棍棒"),
        ("spear", "长矛"),
        ("claw", "爪击"),
        ("bow", "弓箭"),
        ("throw_axe", "投斧"),
        ("throw_stone", "投石"),
    ] {
        out.push(Case {
            name: name.into(),
            events: vec![attack(weapon, 15)],
            full: false,
        });
    }
    for (row, targets, name) in [
        (3, vec![19, 17, 15, 16, 18], "回旋镖·前排五人"),
        (2, vec![14, 12, 10, 11, 13], "回旋镖·后排五人"),
    ] {
        let mut ev = attack("boomerang", targets[0]);
        ev.effects = targets.into_iter().map(|no| effect(no, "hit")).collect();
        ev.row = Some(row);
        out.push(Case {
            name: name.into(),
            events: vec![ev],
            full: true,
        });
    }
    for (func, elem, name) in [
        ("attack", "earth", "地精灵·岩柱"),
        ("attack", "water", "水精灵·涟漪"),
        ("attack", "fire", "火精灵·火舌"),
        ("attack", "wind", "风精灵·风弧"),
        ("heal", "", "治疗"),
        ("revive", "", "复活"),
        ("cure", "", "净化"),
    ] {
        let healing = func != "attack";
        let mut hit = effect(if healing { 5 } else { 15 }, "hit");
        if healing {
            hit.delta = 30;
            hit.hp_pct = Some(100);
        }
        out.push(Case {
            name: name.into(),
            full: false,
            events: vec![BattleEvent {
                kind: "magic".into(),
                actor: Some(0),
                magic: Some(SpellView {
                    func: func.into(),
                    elem: elem.into(),
                    ..Default::default()
                }),
                effects: vec![hit],
                ..Default::default()
            }],
        });
    }
    for (status, name) in [
        ("poison", "毒"),
        ("paralysis", "麻痹"),
        ("sleep", "睡眠"),
        ("stone", "石化"),
        ("drunk", "酒醉"),
        ("confusion", "混乱"),
    ] {
        let mut hit = effect(15, "hit");
        hit.delta = 0;
        hit.status_add = vec![status.into()];
        out.push(Case {
            name: format!("异常·{name}"),
            full: false,
            events: vec![BattleEvent {
                kind: "magic".into(),
                actor: Some(0),
                magic: Some(SpellView {
                    func: "status".into(),
                    status: status.into(),
                    ..Default::default()
                }),
                effects: vec![hit],
                ..Default::default()
            }],
        });
    }
    for (result, name) in [
        ("crit", "暴击"),
        ("dodge", "闪避"),
        ("miss", "落空"),
        ("block", "格挡"),
    ] {
        let mut ev = attack("axe", 15);
        ev.effects[0].result = result.into();
        if result != "crit" {
            ev.effects[0].delta = 0;
            ev.effects[0].hp_pct = Some(100);
        }
        out.push(Case {
            name: name.into(),
            events: vec![ev],
            full: false,
        });
    }
    let mut counter = attack("claw", 0);
    counter.kind = "counter".into();
    counter.actor = Some(15);
    out.push(Case {
        name: "接触现场反击".into(),
        events: vec![attack("axe", 15), counter],
        full: false,
    });
    let mut combo = attack("fist", 15);
    combo.kind = "combo".into();
    combo.actors = vec![0, 5];
    combo.effects.push(effect(15, "crit"));
    out.push(Case {
        name: "人宠合击".into(),
        events: vec![combo],
        full: false,
    });
    for (owner, old, name) in [(0, 5, "本人换宠"), (10, 15, "敌人换宠·回它的主人")] {
        out.push(Case {
            name: name.into(),
            full: false,
            events: vec![BattleEvent {
                kind: "swap_pet".into(),
                actor: Some(owner),
                left: vec![old],
                joined: vec![unit(old, "贝鲁卡")],
                ..Default::default()
            }],
        });
    }
    let mut flying = attack("axe", 15);
    flying.effects[0].flown = true;
    flying.effects[0].down = true;
    out.push(Case {
        name: "打飞离场".into(),
        events: vec![flying],
        full: false,
    });
    out.push(Case {
        name: "二十人站位".into(),
        events: vec![],
        full: true,
    });
    out
}
