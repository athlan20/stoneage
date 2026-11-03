//! STAGE-37 至 39 验收（场景）：模拟服务端按战斗用例回放，客户端用按键或文字命令出招（`docs/battle-ui.md`「指令输入」）。
//! 严格模式下发出的指令要与用例逐字段一致；回放、跳过、速度和结果窗口在界面状态上检查。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use std::time::Instant;

use crossterm::event::KeyCode::{Char, Down, Enter, Esc, Tab};
use stoneage_client::app::battle::{Actor, BattleState, Phase};
use stoneage_client::app::{App, Screen};
use support::{Harness, opts};

/// 登录并等到角色选择页（同 scenarios_pets.rs）。
fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

fn battle(a: &App) -> Option<&BattleState> {
    a.world.as_ref()?.battle.as_ref()
}

/// 这个回合轮到这个角色出招。
fn input(a: &App, turn: u32, actor: Actor) -> bool {
    battle(a).is_some_and(|b| b.turn == turn && b.phase == Phase::Input(actor))
}

/// 进入世界，等战斗的第一回合轮到人物出招（`battle_start` 和 `battle_turn` 都到了）。
fn enter_battle(h: &mut Harness) {
    login_to_characters(h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.wait("第 1 回合轮到人物出招", 5, |a| input(a, 1, Actor::Player));
}

/// STAGE-37：模拟服务端回放一场战斗，进入世界后战场的单位、倒计时都在；战斗中数字键不切换菜单。
#[test]
fn battle_screen_follows_the_replayed_battle() {
    let mut h = Harness::new("battle_win", opts);
    enter_battle(&mut h);
    {
        let b = battle(&h.app).unwrap();
        assert_eq!(b.units.len(), 4, "开局的四个单位都在");
        assert_eq!(b.here.len(), 4);
        assert_eq!(b.name(15), "乌力");
        assert!(b.remaining_secs(Instant::now()).is_some_and(|s| s > 0), "倒计时在走");
    }
    // 2 在指令条上是精灵，这个用例的角色没学过精灵：只给一句说明，菜单不切换。
    h.key(Char('2'));
    assert_eq!(h.app.world.as_ref().unwrap().menu, 0, "战斗中 1–7 不切换页面");
    assert!(h.last_error().contains("还没有学会精灵"), "{}", h.last_error());
    // 这个用例在出招之前就停下：只验证开局的界面。
    h.abort();
}

/// STAGE-25/48：数字键按精灵编号放精灵（用例 battle_magic：角色在 0、1 号栏位有精灵）。
/// 按 2 进精灵列表，按 0 选 0 号精灵（岩石撞击），再按目标；结算里的魔法事件照常播放。
#[test]
fn battle_magic_is_cast_with_number_keys() {
    let mut h = Harness::new("battle_magic", opts);
    enter_battle(&mut h);
    // 回合 1：人物用 0 号精灵（攻击，只能选敌方）打敌方 3 号（协议 15 号）：选中精灵后焦点已在敌方，直接按 3；宠物攻击同一目标。
    h.keys(&[Char('2'), Char('0'), Char('3')]);
    h.wait("宠物轮到出招", 5, |a| input(a, 1, Actor::Pet));
    h.keys(&[Char('1'), Char('3')]);
    h.wait("第 2 回合轮到人物出招", 10, |a| input(a, 2, Actor::Player));
    // 回合 2 补刀敌方 3 号（15 号），回合 3 转火敌方 2 号（16 号）；反击改了随机序列后，打完第 3 回合就是胜利（STAGE-292）。
    for turn in 2..=3u32 {
        let target = if turn == 2 { '3' } else { '2' };
        h.keys(&[Char('1'), Char(target)]);
        h.wait("宠物轮到出招", 5, |a| input(a, turn, Actor::Pet));
        h.keys(&[Char('1'), Char(target)]);
        if turn < 3 {
            h.wait("下一回合轮到人物出招", 10, |a| input(a, turn + 1, Actor::Player));
        }
    }
    h.wait("结果窗口弹出", 10, |a| battle(a).is_some_and(|b| b.result.is_some()));
    h.key(Enter);
    assert!(battle(&h.app).is_none());
    // 战斗后 MP 少了 10（岩石撞击），角色页清单照常显示。
    h.wait("MP 更新", 5, |a| a.world.as_ref().is_some_and(|w| w.entry.status.mp == 90 && w.entry.status.magic.len() == 2));
    h.finish();
}

/// STAGE-78：道具选目标的默认位置。小块肉对己方一个，选中后焦点落在自己身上，回车发出的是 target 0
/// （用例 battle_target_pick：回复 next 为 pet 后用例结束，宠物轮到出招时就停下）。
#[test]
fn ally_item_targets_self_by_default() {
    let mut h = Harness::new("battle_target_pick", opts);
    enter_battle(&mut h);
    // 3 是道具，列表里高亮落在 5 号栏位的小块肉（回复类，默认高亮）上：回车选中，它只能选己方，焦点已经在自己身上，再回车确认。
    h.keys(&[Char('3'), Enter, Enter, Enter]);
    h.wait("宠物轮到出招", 5, |a| input(a, 1, Actor::Pet));
    h.finish();
}

/// STAGE-38/39：用数字键打完一场胜利（服务端骨架用例是四个回合，见 docs/protocol/cases/battle_win.jsonl）。
/// 结算逐条播放，播完确认，结束后弹结果窗口，Enter 关闭。
#[test]
fn battle_win_is_played_with_number_keys() {
    let mut h = Harness::new("battle_win", opts);
    enter_battle(&mut h);
    // 回合 1、2 攻击敌方 3 号（协议 15 号，界面按原版站位表编号），回合 3、4 转火敌方 2 号（16 号）。
    for turn in 1..=4u32 {
        let target = if turn <= 2 { '3' } else { '2' };
        h.keys(&[Char('1'), Char(target)]);
        h.wait("宠物轮到出招", 5, |a| input(a, turn, Actor::Pet));
        h.keys(&[Char('1'), Char(target)]);
        if turn < 4 {
            h.wait("下一回合轮到人物出招", 10, |a| input(a, turn + 1, Actor::Player));
        }
    }
    h.wait("结果窗口弹出", 10, |a| battle(a).is_some_and(|b| b.result.is_some()));
    assert!(battle(&h.app).is_some_and(|b| b.phase == Phase::Ended));
    // Enter 关闭结果窗口：战斗结束，回到进入战斗前的页面。
    h.key(Enter);
    assert!(battle(&h.app).is_none());
    // 文字命令 battle 查看现状，warp 照常可用（服务端都按用例回复）。
    h.command("battle");
    h.wait("查询到不在战斗中", 5, |a| a.log.all().iter().any(|m| m.text == "当前不在战斗中"));
    h.command("warp 1");
    h.wait("已传送", 5, |a| a.log.all().iter().any(|m| m.text == "已传送到 萨姆吉尔村"));
    h.finish();
}

/// STAGE-126：合击。宠物和人物打同一个怪，服务端回一个 combo 事件（用例 battle_combo，只放第一回合）；
/// 回放照常播完并确认，战斗记录里是一行“乌力 + 勇者 合击 乌力 …”，两个出手者的伤害都在。
#[test]
fn combo_event_is_played_and_recorded() {
    let mut h = Harness::new("battle_combo", opts);
    enter_battle(&mut h);
    h.keys(&[Char('1'), Char('3')]);
    h.wait("宠物轮到出招", 5, |a| input(a, 1, Actor::Pet));
    h.keys(&[Char('1'), Char('3')]);
    h.wait("第 2 回合轮到人物出招", 10, |a| input(a, 2, Actor::Player));
    let b = battle(&h.app).unwrap();
    assert!(
        b.records
            .iter()
            .any(|r| r.starts_with("乌力 + 勇者小明 合击 乌力 -5 -6")),
        "记录里应有合击一行（宠物先出手）：{:?}",
        b.records
    );
    assert_eq!(b.units[&15].hp_pct, 21, "两下伤害都打到了目标上");
    h.finish();
}
/// STAGE-38：文字命令出招，宠物技能能发出；结算里有麻痹、中毒、怪物逃走，第 2 回合怪物被打倒、其余逃走，按胜利收场（STAGE-193）。
#[test]
fn battle_events_are_played_with_typed_commands() {
    let mut h = Harness::new("battle_events", opts);
    enter_battle(&mut h);
    // 第 1 回合：人物攻击敌方 2 号（协议 16 号），宠物用技能 1 打敌方 3 号（15 号）。
    h.command("attack e2");
    h.wait("宠物轮到出招", 5, |a| input(a, 1, Actor::Pet));
    h.command("skill 1 e3");
    h.wait("第 2 回合轮到人物出招", 10, |a| input(a, 2, Actor::Player));
    // 第 2 回合：人物攻击敌方 3 号（15 号），宠物继续技能；怪物全体逃走，平局收场。
    h.command("attack e3");
    h.wait("宠物轮到出招", 5, |a| input(a, 2, Actor::Pet));
    h.command("skill 1 e3");
    h.wait("结果窗口弹出", 10, |a| battle(a).is_some_and(|b| b.result.is_some()));
    h.key(Char('x'));
    assert!(battle(&h.app).is_some(), "普通字母不关闭结果");
    h.key(Enter);
    assert!(battle(&h.app).is_none());
    h.finish();
}

/// STAGE-39 / STAGE-173：宠物先被打倒后只剩人物指令，防御到第 6 回合也被打倒，
/// 结果窗口写明原地复活（用例 battle_lose 是 6 个回合的失败，被打死不是被打飞，STAGE-194）。
/// 回放速度与「跳过」在 STAGE-173 收进系统页，战斗里不再有 `f` / `空格`——这里只验证整场打完。
#[test]
fn battle_lose_plays_out_and_shows_respawn() {
    let mut h = Harness::new("battle_lose", opts);
    enter_battle(&mut h);
    for turn in 1..=6u32 {
        h.wait("人物轮到出招", 5, |a| input(a, turn, Actor::Player));
        h.key(Char('4')); // 防御
        if turn == 1 { // 宠物在第 1 回合就被打倒（怪物挑中了它，STAGE-60 的怪物 AI），第 2 回合起只剩人物指令
            h.wait("宠物轮到出招", 5, |a| input(a, turn, Actor::Pet));
            h.key(Char('3')); // 宠物指令条上的防御
        }
        h.wait("回合开始结算", 5, |a| battle(a).is_some_and(|b| b.phase == Phase::Replay));
        if turn < 6 {
            h.wait("下一回合轮到人物出招", 10, |a| input(a, turn + 1, Actor::Player));
        }
    }
    h.wait("结果窗口弹出", 10, |a| battle(a).is_some_and(|b| b.result.is_some()));
    h.key(Enter);
    assert!(battle(&h.app).is_none());
    h.wait("原地复活，位置不变", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.entry.position.map_name.contains("萨姆吉尔的村长家"))
    });
    h.finish();
}

/// STAGE-38：方向键加回车也能打完整场（同用例 battle_win）：焦点跳过不能用的技能，目标用方向键移到 3 号。
#[test]
fn battle_win_with_arrows_and_enter() {
    let mut h = Harness::new("battle_win", opts);
    enter_battle(&mut h);
    // 回合 1：回车选中攻击，默认目标是敌方 2 号（空位），下移一格到 3 号，回车确认。
    h.keys(&[Enter, Down, Enter]);
    h.wait("宠物轮到出招", 5, |a| input(a, 1, Actor::Pet));
    h.keys(&[Enter, Down, Enter]);
    h.wait("第 2 回合轮到人物出招", 10, |a| input(a, 2, Actor::Player));
    // 回合 2：同样打敌方 3 号。
    h.keys(&[Enter, Down, Enter]);
    h.wait("宠物轮到出招", 5, |a| input(a, 2, Actor::Pet));
    h.keys(&[Enter, Down, Enter]);
    h.wait("第 3 回合轮到人物出招", 10, |a| input(a, 3, Actor::Player));
    // 回合 3：敌方 3 号已倒下，默认目标就是敌方 2 号，直接回车；回合 4 同样。
    for turn in 3..=4u32 {
        h.keys(&[Enter, Enter]);
        h.wait("宠物轮到出招", 5, |a| input(a, turn, Actor::Pet));
        h.keys(&[Enter, Enter]);
        if turn < 4 {
            h.wait("第 4 回合轮到人物出招", 10, |a| input(a, 4, Actor::Player));
        }
    }
    h.wait("结果窗口弹出", 10, |a| battle(a).is_some_and(|b| b.result.is_some()));
    h.key(Enter);
    assert!(battle(&h.app).is_none());
    // 用例结尾还有 battle 查询和 warp（同 battle_win_is_played_with_number_keys）。
    h.command("battle");
    h.wait("查询到不在战斗中", 5, |a| a.log.all().iter().any(|m| m.text == "当前不在战斗中"));
    h.command("warp 1");
    h.wait("已传送", 5, |a| a.log.all().iter().any(|m| m.text == "已传送到 萨姆吉尔村"));
    h.finish();
}

/// STAGE-109：宠物指令阶段按 Esc 退回人物指令，重新选的防御覆盖先交的攻击，宠物轮到出招（用例 battle_resubmit）。
#[test]
fn pet_phase_esc_resends_the_player_command() {
    let mut h = Harness::new("battle_resubmit", opts);
    enter_battle(&mut h);
    h.command("attack e3");
    h.wait("宠物轮到出招", 5, |a| input(a, 1, Actor::Pet));
    h.key(Esc);
    h.wait("退回人物指令重选", 5, |a| {
        battle(a).is_some_and(|b| b.revising && b.phase == Phase::Input(Actor::Player))
    });
    h.command("defend");
    h.wait("防御覆盖后又轮到宠物出招", 5, |a| input(a, 1, Actor::Pet));
    assert!(battle(&h.app).is_some_and(|b| b.can_revert() && !b.revising));
    // 用例最后一步是“重交选了己方宠物（INVALID_TARGET）”，界面在本地就挡住了，发不出去，所以停在这里。
    h.finish_stopped_early();
}

/// STAGE-229：魔抗系状态（STAGE-228）回放在战斗单位上（用例 battle_magic_resist，严格模式）：魔法防御挂魔抗，
/// 再用火之精灵 miss（已带魔抗，互斥），防御后魔抗减完；火之精灵挂上火抗，人物的状态带着 fire_resist，卡片标记画“火”。
#[test]
fn magic_resist_statuses_are_shown_on_the_unit() {
    let mut h = Harness::new("battle_magic_resist", opts);
    enter_battle(&mut h);
    // 回合 1：2 精灵 → 0 号（魔法防御，己方单体，焦点落在自己身上，回车确认）。
    h.keys(&[Char('2'), Char('0'), Enter]);
    h.wait("第 2 回合轮到人物出招", 10, |a| input(a, 2, Actor::Player));
    let has = |a: &App, st: &str| {
        battle(a).is_some_and(|b| b.units.get(&0).is_some_and(|u| u.status.iter().any(|s| s == st)))
    };
    assert!(has(&h.app, "magic_resist"), "魔法防御挂上魔抗");
    // 回合 2：1 号精灵（火之精灵）对自己，已带魔抗，miss。
    h.keys(&[Char('2'), Char('1'), Enter]);
    h.wait("第 3 回合轮到人物出招", 10, |a| input(a, 3, Actor::Player));
    assert!(has(&h.app, "magic_resist") && !has(&h.app, "fire_resist"), "互斥：火之精灵没挂上");
    // 回合 3：防御，回合末魔抗减到 0（status_remove），之后才能再挂。
    h.key(Char('4'));
    h.wait("第 4 回合轮到人物出招", 10, |a| input(a, 4, Actor::Player));
    assert!(!has(&h.app, "magic_resist"), "魔抗回合末解除");
    // 回合 4：火之精灵挂上火抗（只占槽）。
    h.keys(&[Char('2'), Char('1'), Enter]);
    h.wait("第 5 回合轮到人物出招", 10, |a| input(a, 5, Actor::Player));
    assert!(has(&h.app, "fire_resist"), "火之精灵挂上火抗");
    // 回合 5：魔法防御对自己 miss（已带火抗）。
    h.keys(&[Char('2'), Char('0'), Enter]);
    h.wait("第 6 回合轮到人物出招", 10, |a| input(a, 6, Actor::Player));
    assert!(has(&h.app, "fire_resist") && !has(&h.app, "magic_resist"), "火抗还在，没有挂上魔抗");
    // 用例最后一步是 battle 查询，这里不发，停在第 6 回合（同 pet_phase_esc_resends_the_player_command）。
    h.finish_stopped_early();
}
