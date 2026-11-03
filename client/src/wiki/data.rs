//! 百科数据快照：编译期由 `build.rs` 从 `internal/gamewiki/snapshot/` 嵌入（STAGE-83）。

include!(concat!(env!("OUT_DIR"), "/wiki_data.rs"));
