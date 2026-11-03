//! `warp` 的目标查找（STAGE-83）：地点编号、地点名、地图编号、地图名。
//! 地点表（`docs/protocol.md` 5.15）和地图表（5.42）都由服务端给，这里只把输入换算成发给服务端的 `place` 或 `floor`。

use crate::proto::{Place, WarpMap};

/// 解析出的传送目标。
#[derive(Debug, PartialEq, Eq)]
pub enum Dest<'a> {
    Place(&'a Place),
    Map(&'a WarpMap),
    /// 纯数字，既不是地点编号也不在地图表里：照样发给服务端，由它决定能不能传。
    Floor(u32),
}

/// 找传送目标。纯数字先当地点编号，再当地图编号；名字先精确匹配地点，再精确匹配地图，最后才是唯一的包含。
/// 和地点重合的地图（同一个 floor，如“萨姆吉尔村”）只算地点，不会重复。
pub fn find_destination<'a>(places: &'a [Place], maps: &'a [WarpMap], arg: &str) -> Result<Dest<'a>, String> {
    if let Ok(n) = arg.parse::<u32>() {
        if let Some(p) = places.iter().find(|p| p.id == n) {
            return Ok(Dest::Place(p));
        }
        return Ok(maps.iter().find(|m| m.floor == n).map_or(Dest::Floor(n), Dest::Map));
    }
    if places.is_empty() && maps.is_empty() {
        return Err("地点表还没有取到，请稍后再试".to_string());
    }
    if let Some(p) = places.iter().find(|p| p.name == arg) {
        return Ok(Dest::Place(p));
    }
    let exact: Vec<&WarpMap> = maps.iter().filter(|m| m.name == arg).collect();
    match exact.as_slice() {
        [m] => return Ok(Dest::Map(m)),
        [_, _, ..] => {
            let floors: Vec<String> = exact.iter().take(LIST_LIMIT).map(|m| m.floor.to_string()).collect();
            return Err(format!(
                "“{arg}”对应好几张地图（{}），请写地图编号，如 warp {}",
                floors.join("、"),
                exact[0].floor
            ));
        }
        [] => {}
    }
    let mut hits: Vec<Dest> = places.iter().filter(|p| p.name.contains(arg)).map(Dest::Place).collect();
    hits.extend(
        maps.iter().filter(|m| m.name.contains(arg) && !places.iter().any(|p| p.floor == m.floor)).map(Dest::Map),
    );
    match hits.len() {
        1 => Ok(hits.remove(0)),
        0 => Err(format!("没有叫“{arg}”的地点或地图（地图名可以按 Tab 补全，也能写地图编号）")),
        _ => {
            let mut names: Vec<String> = vec![];
            for h in &hits {
                let name = match h {
                    Dest::Place(p) => p.name.clone(),
                    Dest::Map(m) => m.name.clone(),
                    Dest::Floor(_) => continue,
                };
                if !names.contains(&name) {
                    names.push(name);
                }
            }
            names.truncate(LIST_LIMIT);
            Err(format!("“{arg}”对应好几个地点或地图（{}），请写完整的名字或编号", names.join("、")))
        }
    }
}

/// 错误提示里最多列出的候选数。
const LIST_LIMIT: usize = 8;

#[cfg(test)]
mod tests {
    use super::*;

    fn places() -> Vec<Place> {
        let p = |id, name: &str, floor| Place { id, name: name.into(), floor, x: 1, y: 1 };
        vec![
            p(1, "萨姆吉尔村", 1000),
            p(2, "玛丽娜丝渔村", 2000),
            p(3, "加加村", 3000),
            p(4, "卡鲁它那村", 4000),
            p(5, "萨姆吉尔近郊", 100),
        ]
    }

    fn maps() -> Vec<WarpMap> {
        let m = |floor, name: &str| WarpMap { floor, name: name.into() };
        vec![
            m(41, "梦幻的洞窟"),
            m(42, "梦幻的洞窟"),
            m(117, "迷宫"),
            m(118, "迷宫"),
            m(1000, "萨姆吉尔村"),
            m(1006, "萨姆吉尔的村长家"),
        ]
    }

    #[test]
    fn numbers_are_place_ids_then_floors() {
        let (ps, ms) = (places(), maps());
        assert!(matches!(find_destination(&ps, &ms, "3"), Ok(Dest::Place(p)) if p.name == "加加村"));
        assert!(matches!(find_destination(&ps, &ms, "1006"), Ok(Dest::Map(m)) if m.name == "萨姆吉尔的村长家"));
        assert_eq!(find_destination(&ps, &ms, "99"), Ok(Dest::Floor(99)), "不在两张表里的数字照样发给服务端");
        assert_eq!(find_destination(&[], &[], "1006"), Ok(Dest::Floor(1006)), "表还没取到时数字照样发");
    }

    #[test]
    fn names_match_places_then_maps() {
        let (ps, ms) = (places(), maps());
        assert!(matches!(find_destination(&ps, &ms, "加加村"), Ok(Dest::Place(p)) if p.id == 3));
        assert!(
            matches!(find_destination(&ps, &ms, "萨姆吉尔村"), Ok(Dest::Place(p)) if p.id == 1),
            "地点和同 floor 的地图只算地点"
        );
        assert!(matches!(find_destination(&ps, &ms, "近郊"), Ok(Dest::Place(p)) if p.id == 5), "唯一的包含也算");
        assert!(matches!(find_destination(&ps, &ms, "渔村"), Ok(Dest::Place(p)) if p.id == 2));
        assert!(matches!(find_destination(&ps, &ms, "村长"), Ok(Dest::Map(m)) if m.floor == 1006), "地图名的唯一包含");
        assert!(matches!(find_destination(&ps, &ms, "萨姆吉尔的村长家"), Ok(Dest::Map(m)) if m.floor == 1006));
    }

    #[test]
    fn ambiguous_and_missing_names_explain_why() {
        let (ps, ms) = (places(), maps());
        assert!(find_destination(&ps, &ms, "萨姆吉尔").unwrap_err().contains("好几个"), "村、近郊和村长家都带萨姆吉尔");
        assert!(find_destination(&ps, &ms, "村").unwrap_err().contains("好几个"));
        assert!(find_destination(&ps, &ms, "迷宫").unwrap_err().contains("117、118"), "同名地图要写编号");
        assert!(find_destination(&ps, &ms, "梦幻").unwrap_err().contains("好几个"));
        assert!(find_destination(&ps, &ms, "不存在").unwrap_err().contains("没有叫"));
        assert!(find_destination(&[], &[], "加加村").unwrap_err().contains("还没有取到"));
    }
}
