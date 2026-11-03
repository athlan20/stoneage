//! 任务 → 百科攻略的对照表（STAGE-89，`docs/ui.md`「任务页」）。对照表 `tools/quest-wiki-map.json`
//! 是人工维护的，编译时嵌入；空数组表示百科里没有这个任务的攻略。同一篇攻略（第一个 key 相同）的任务
//! 按编号排序，用来标“第几步 / 共几步”。

use std::collections::BTreeMap;
use std::sync::OnceLock;

const MAP: &str = include_str!("../../../tools/quest-wiki-map.json");

fn table() -> &'static BTreeMap<u32, Vec<String>> {
    static TABLE: OnceLock<BTreeMap<u32, Vec<String>>> = OnceLock::new();
    TABLE.get_or_init(|| {
        // 以 `_` 开头的键是说明文字，不是任务。
        let raw: BTreeMap<String, serde_json::Value> = serde_json::from_str(MAP).expect("quest-wiki-map.json 不是合法的 JSON");
        raw.into_iter()
            .filter(|(k, _)| !k.starts_with('_'))
            .map(|(k, v)| {
                let no: u32 = k.parse().expect("对照表的键必须是任务编号");
                let keys: Vec<String> = serde_json::from_value(v).expect("对照表的值必须是词条 key 数组");
                (no, keys)
            })
            .collect()
    })
}

/// 任务对应的百科词条 key，按显示顺序；没有攻略（或编号不在表里）时为空。
pub fn keys(no: u32) -> &'static [String] {
    table().get(&no).map_or(&[], Vec::as_slice)
}

/// 任务在同一篇攻略下的步骤：第几步、共几步。没有攻略时为 None。
pub fn step(no: u32) -> Option<(usize, usize)> {
    let first = keys(no).first()?;
    let same: Vec<u32> = table().iter().filter(|(_, ks)| ks.first() == Some(first)).map(|(n, _)| *n).collect();
    let pos = same.iter().position(|n| *n == no)?;
    Some((pos + 1, same.len()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::wiki::wiki;

    /// 服务端的任务表（`tools/gen-quests.py` 生成）：每个任务都要在对照表里有结论。
    const QUESTS_TSV: &str = include_str!("../../../server/gamedata/quests.tsv");

    #[test]
    fn every_task_in_the_table_has_a_verdict() {
        let nos: Vec<u32> = QUESTS_TSV
            .lines()
            .filter(|l| !l.starts_with('#'))
            .filter_map(|l| l.split('\t').next()?.parse().ok())
            .collect();
        assert_eq!(nos.len(), 110, "任务表应有 110 个任务（61 号事件号为 0 跳过；111 为 STAGE-333 自定义任务）");
        for no in &nos {
            assert!(table().contains_key(no), "任务 {no} 没有在 quest-wiki-map.json 里登记（有攻略填词条，没有就填空数组）");
        }
        assert_eq!(table().len(), nos.len(), "对照表里多出了任务表里没有的编号");
    }

    #[test]
    fn every_mapped_key_exists_in_the_snapshot() {
        for (no, keys) in table() {
            for key in keys {
                let hit = wiki().locate(key);
                assert!(hit.is_some(), "任务 {no} 指向的词条 {key} 不在当前百科快照里（快照重新生成后需要更新对照表）");
            }
        }
    }

    #[test]
    fn steps_count_tasks_sharing_the_same_first_entry() {
        assert_eq!(step(1), Some((1, 1)));
        assert_eq!(step(6), Some((1, 2)), "恐龙博士 抓鲁尼帖斯 是第 1 步");
        assert_eq!(step(7), Some((2, 2)), "恐龙博士 抓贝鲁卡 是第 2 步");
        assert_eq!(step(38), Some((1, 2)));
        assert_eq!(step(39), Some((2, 2)));
        assert_eq!(step(9), None, "没有攻略的任务没有步骤");
        assert!(keys(36).len() == 2, "琉璃洞窟在百科里有两篇");
        assert!(keys(9).is_empty());
    }
}
