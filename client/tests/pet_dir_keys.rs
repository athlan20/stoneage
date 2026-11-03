//! STAGE-251 验收（界面）：快捷传送根层的「1级宠物」→ 目录（取一次，之后用缓存）→ 筛选 → L2 遇敌点 → L3 动作。
//! 模拟服务端按用例 `ui_pet_dir` 回放：目录内容与两次 `pet_encounter_start` 都写死在用例里。
//! 第一次选 331 在区域 38 的「只传送」，服务端回 `PET_ENCOUNTER_NOT_FOUND`，客户端必须留在 L3；
//! 第二次选 11 在同一区域的「只传送」，位置换到萨伊那斯 547,115，成功回场景。

// 每个测试文件只用到 support 的一部分。
#[allow(dead_code)]
mod support;

use crossterm::event::KeyCode::{Char, Down, End, Enter, Esc, Tab};
use stoneage_client::app::pet_dir::{PetDir, PetDirStatus, PetLevel};
use stoneage_client::app::{App, Kind, Layer, MAP_MENU, Screen};
use support::{Harness, opts};

fn login_to_characters(h: &mut Harness) {
    h.text("alice");
    h.keys(&[Tab]);
    h.text("secret1");
    h.key(Enter);
    h.wait_screen(Screen::Characters);
}

/// 场景 → 游戏菜单 → 快捷传送（菜单第 5 项）。
fn open_travel(h: &mut Harness) {
    h.key(Esc);
    h.repeat(Down, 4);
    h.key(Enter);
}

/// 逐个字符按键（不用 `h.text`：它会先按 `/`，这里 `/` 已经是筛选框的开关）。
fn type_keys(h: &mut Harness, s: &str) {
    for c in s.chars() {
        h.key(Char(c));
    }
}

fn dir(app: &App) -> &PetDir {
    &app.world.as_ref().unwrap().travel.pets
}

fn visible_names(app: &App) -> Vec<String> {
    dir(app).visible_pets().iter().map(|p| p.name.clone()).collect()
}

fn on_travel_page(app: &App) -> bool {
    app.world.as_ref().is_some_and(|w| w.nav.is(&Layer::Page(MAP_MENU)))
}

fn has_log(app: &App, needle: &str) -> bool {
    app.log.all().iter().any(|m| m.text.contains(needle))
}

#[test]
fn pet_dir_filters_then_travels_to_the_spot() {
    let mut h = Harness::new("ui_pet_dir", opts);
    login_to_characters(&mut h);
    h.key(Enter);
    h.key(Enter);
    h.wait_screen(Screen::World);
    h.wait("拿到快捷传送目录", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.travel.ready())
    });

    open_travel(&mut h);
    h.wait("打开快捷传送页", 5, |a| {
        a.world
            .as_ref()
            .is_some_and(|w| w.menu == MAP_MENU && w.nav.is(&Layer::Page(MAP_MENU)))
    });
    // 根层的「1级宠物」固定在末尾，目录还没打开，所以根层只有地图（不含它）以外的一项。
    assert_eq!(
        h.app.world.as_ref().unwrap().travel.view().iter().rev().nth(1).map(|r| r.pets),
        Some(true),
        "根层末尾是「1级宠物」"
    );

    // 光标到末尾 Enter：打开目录，目录这时才去取（用例里的第一条 pet_encounter_catalog）。
    // 「练级传送」（STAGE-279）是新的最后一项，「1级宠物」在它上面一行。
    h.key(End);
    h.key(crossterm::event::KeyCode::Up);
    h.key(Enter);
    h.wait("目录取到", 5, |a| dir(a).status == PetDirStatus::Ready);
    assert!(dir(&h.app).is_open());
    assert_eq!(dir(&h.app).depth(), 1, "进来是 L1");
    assert_eq!(visible_names(&h.app), vec!["利则诺顿", "布比", "凯比特"], "目录按服务端顺序列出");

    // `/` 把焦点交给筛选框；输入实时过滤；Enter 应用并回列表。
    h.key(Char('/'));
    assert!(dir(&h.app).typing, "/ 开筛选框");
    type_keys(&mut h, "利则");
    assert_eq!(dir(&h.app).filter, "利则");
    assert_eq!(visible_names(&h.app), vec!["利则诺顿"]);
    h.key(Enter);
    assert!(!dir(&h.app).typing, "Enter 交回列表");

    // L1 → L2（遇敌点）→ L3（动作）。
    h.key(Enter);
    assert_eq!(dir(&h.app).level(), &PetLevel::Spots { pet_id: 331 });
    h.key(Enter);
    assert_eq!(dir(&h.app).level(), &PetLevel::Actions { pet_id: 331, spot: 0 });

    // L3 选「只传送不遇敌」：服务端回 PET_ENCOUNTER_NOT_FOUND，留在 L3，消息栏写原因。
    h.repeat(Down, 2);
    h.key(Enter);
    h.wait("报错写进消息栏", 5, |a| has_log(a, "没有可遇点"));
    assert_eq!(
        dir(&h.app).level(),
        &PetLevel::Actions { pet_id: 331, spot: 0 },
        "失败留在原层"
    );
    assert!(dir(&h.app).is_open());
    assert!(on_travel_page(&h.app), "失败不回场景");
    assert_eq!(
        h.app
            .log
            .all()
            .iter()
            .rev()
            .find(|m| m.kind == Kind::Error)
            .map(|m| m.text.clone())
            .unwrap_or_default(),
        "这只宠物在这个区域没有可遇点"
    );

    // 逐层 Esc：L3 → L2 → L1（筛选词保留）→ 清空筛选 → 离开目录。
    h.key(Esc);
    assert_eq!(dir(&h.app).level(), &PetLevel::Spots { pet_id: 331 });
    h.key(Esc);
    assert_eq!(dir(&h.app).level(), &PetLevel::Pets);
    assert_eq!(dir(&h.app).filter, "利则", "返回列表时保留筛选词");
    h.key(Esc);
    assert!(dir(&h.app).filter.is_empty(), "有筛选词时 Esc 先清空");
    assert!(dir(&h.app).is_open());
    assert_eq!(visible_names(&h.app).len(), 3);
    h.key(Esc);
    assert!(!dir(&h.app).is_open(), "没有筛选词时 Esc 离开目录");
    assert!(on_travel_page(&h.app), "回到快捷传送根层");
    assert!(h.app.world.as_ref().unwrap().travel.path().is_empty());

    // 再进来：目录已经取过，直接用缓存；光标还停在根层的「1级宠物」上。
    h.key(Enter);
    assert!(dir(&h.app).is_open());
    assert_eq!(dir(&h.app).status, PetDirStatus::Ready);

    // 第二只：布比，在同一区域只传送，成功回场景并写消息。
    h.key(Char('/'));
    type_keys(&mut h, "布比");
    h.key(Enter);
    assert_eq!(visible_names(&h.app), vec!["布比"]);
    h.key(Enter);
    h.key(Enter);
    assert_eq!(dir(&h.app).level(), &PetLevel::Actions { pet_id: 11, spot: 0 });
    h.repeat(Down, 2);
    h.key(Enter);
    h.wait("回到场景", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.nav.is(&Layer::Scene))
    });
    assert!(has_log(&h.app, "已传送到 萨伊那斯"), "成功后消息栏写已传送");
    assert!(!dir(&h.app).is_open(), "成功后目录关闭");

    // 布比的遇敌点就在脚下（区域 38）：直接开战只提示“已经在这里”，不发请求，留在 L3。
    open_travel(&mut h);
    h.wait("再次打开快捷传送页", 5, on_travel_page);
    h.key(End);
    h.key(crossterm::event::KeyCode::Up);
    h.key(Enter);
    assert!(dir(&h.app).is_open(), "再进来用缓存");
    h.key(Char('/'));
    type_keys(&mut h, "布比");
    h.key(Enter);
    h.keys(&[Enter, Enter]);
    assert_eq!(dir(&h.app).level(), &PetLevel::Actions { pet_id: 11, spot: 0 });
    h.key(Enter);
    h.wait("提示已经在这里", 5, |a| has_log(a, "已经在这里：萨伊那斯"));
    assert_eq!(dir(&h.app).level(), &PetLevel::Actions { pet_id: 11, spot: 0 }, "已经在这里不离开 L3");
    assert!(on_travel_page(&h.app), "已经在这里不发请求，也不回场景");

    // 凯比特在区域 39（别的地图）：直接开战，mode=once，成功回场景、位置换到 600,200。
    h.keys(&[Esc, Esc, Esc]); // L3 → L2 → L1（筛选词保留）→ 清空筛选
    assert!(dir(&h.app).filter.is_empty());
    h.key(Char('/'));
    type_keys(&mut h, "凯比");
    h.key(Enter);
    assert_eq!(visible_names(&h.app), vec!["凯比特"]);
    h.keys(&[Enter, Enter, Enter]);
    assert_eq!(dir(&h.app).level(), &PetLevel::Actions { pet_id: 22, spot: 0 });
    h.key(Enter);
    h.wait("直接开战回到场景", 5, |a| a.world.as_ref().is_some_and(|w| w.nav.is(&Layer::Scene)));
    assert!(has_log(&h.app, "已传送到 萨伊那斯，开始遇敌"), "直接开战写“开始遇敌”");
    assert!(!dir(&h.app).is_open(), "成功后目录关闭");

    // 布比在区域 38：连续遇敌（infinite），从区域 39 换回来，成功回场景。
    open_travel(&mut h);
    h.wait("再次打开快捷传送页", 5, on_travel_page);
    h.key(End);
    h.key(crossterm::event::KeyCode::Up);
    h.key(Enter);
    h.key(Char('/'));
    type_keys(&mut h, "布比");
    h.key(Enter);
    h.keys(&[Enter, Enter, Down, Enter]);
    h.wait("连续遇敌回到场景", 5, |a| {
        a.world.as_ref().is_some_and(|w| w.nav.is(&Layer::Scene) && w.entry.position.x == 547)
    });
    assert!(!dir(&h.app).is_open(), "成功后目录关闭");

    h.command("leave");
    h.finish();
}
