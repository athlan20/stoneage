//! STAGE-42 / STAGE-173 验收（场景，`docs/battle-ui.md`「观察模式与托管」）：托管开着每回合自动出招，
//! 发出的指令与手动出招完全一样（严格模式下模拟服务端逐字段校验），一整场不用按任何战斗键。
//!
//! 用「全程防御」的 `battle_lose`：防御不带目标，与「默认攻击目标」（合击顺序 / 随机）无关——
//! 攻击目标的顺序由 `app/battle/auto.rs` 的单测覆盖。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::Enter;
use stoneage_client::app::alert;
use stoneage_client::app::auto_battle::{ActKind, PetActKind};
use stoneage_client::app::battle::{Actor, BattleState, Phase};
use stoneage_client::app::{App, Screen};
use support::{Harness, opts};

fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[crossterm::event::KeyCode::Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

fn battle(a: &App) -> Option<&BattleState> {
    a.world.as_ref()?.battle.as_ref()
}

fn input(a: &App, turn: u32, actor: Actor) -> bool {
    battle(a).is_some_and(|b| b.turn == turn && b.phase == Phase::Input(actor))
}

fn auto(a: &App) -> bool {
    a.world.as_ref().is_some_and(|w| w.auto_battle)
}

/// 进入世界，等战斗的第一回合轮到人物出招。
/// STAGE-131 起进入世界的落点是场景层；战斗里没有可进的页面，命令走 `/` 打开的命令栏。
fn enter_battle(h: &mut Harness) {
    login_to_characters(h);
    h.key(Enter);

    h.key(Enter);

    h.wait_screen(Screen::World);
    h.wait("第 1 回合轮到人物出招", 5, |a| input(a, 1, Actor::Player));
}

fn auto_on(h: &mut Harness) {
    // 战斗中打字照旧进输入行（STAGE-131：有模态层时导航栈不接管）。
    h.text("auto-battle on");
    h.key(Enter);
    h.wait("自动挂机已开启", 5, auto);
}

/// 托管全自动打完一整场：不按一个战斗键，人物每回合防御、宠物第 1 回合防御（用例要求），
/// 打输后托管自动关，结果窗口手动关掉。
#[test]
fn auto_battle_plays_a_whole_battle_without_keys() {
    let mut h = Harness::new("battle_lose", opts);
    enter_battle(&mut h);
    // 用例要求每回合防御：方案配成人宠都防御，托管照着做（攻击目标是什么与本用例无关）。
    // 顺带关掉「补给不足时停止」：本用例要一整场跑完，不带药的低血会正常触发停止保护。
    {
        let p = h.app.auto_store.as_mut().expect("进入世界时已经加载方案").profiles.active_mut();
        p.player.act = ActKind::Guard;
        p.pet.act = PetActKind::Guard;
        p.stop.no_supply = false;
    }
    // 顺带验证「跳过」档：回放不播动画，整场几秒跑完。
    h.app.skip_replay = true;
    // 打输后会响提示音（STAGE-265）：测试里静音，只数次数。
    alert::set_muted(true);
    let rings = alert::lose_rings();
    auto_on(&mut h);
    h.wait("托管自动打输并弹出结果", 60, |a| battle(a).is_some_and(|b| b.result.is_some()));
    assert!(!auto(&h.app), "打输后托管自动关");
    assert_eq!(alert::lose_rings(), rings + 1, "挂机开着打输，响一次");
    assert!(
        h.app.log.all().iter().any(|m| m.text == "被打输了，已停止自动挂机和自动遇敌"),
        "消息栏要说明托管关闭的原因：{:?}",
        h.last_error()
    );
    // 结果窗口（失败）玩家手动关掉。
    h.key(Enter);
    h.wait("战斗界面收起", 5, |a| battle(a).is_none());
    h.finish();
}

// STAGE-173 起托管中不再让玩家手动选招（数字/方向/回车/Esc 都被拦，只留 `8` 停止），
// 所以原来那两个「边托管边手动出招」的用例不再成立：
// - 按键被拦、按 8 能停止、停止后恢复选招 → `tests/keys_battle.rs::auto_battle_locks_the_action_grid_until_stopped`；
// - 打输自动关托管 → 本文件上面那个用例 + `app/battle/auto.rs` 的单测。
// 本文件保留「全自动打完一整场」这条主链路。
