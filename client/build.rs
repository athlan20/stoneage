//! STAGE-83：把石器百科的数据快照（`internal/gamewiki/snapshot/`）嵌进客户端二进制。
//! 只嵌 `catalog.json.gz` 指向的当前版本：各分类的列表行（`category-*.json.gz`）和
//! 256 个词条详情分片（`00`–`ff`）；全站索引 `index.json.gz` 与列表行重复，不嵌。
//! 生成 `$OUT_DIR/wiki_data.rs`，由 `src/wiki/data.rs` 引入。

use std::env;
use std::fmt::Write as _;
use std::fs;
use std::io::Read;
use std::path::{Path, PathBuf};

use flate2::read::GzDecoder;

fn gunzip(path: &Path) -> Vec<u8> {
    let raw = fs::read(path).unwrap_or_else(|e| panic!("读不到百科快照 {}：{e}", path.display()));
    let mut out = Vec::new();
    GzDecoder::new(raw.as_slice()).read_to_end(&mut out).unwrap_or_else(|e| panic!("{} 不是 gzip：{e}", path.display()));
    out
}

fn main() {
    let manifest = PathBuf::from(env::var("CARGO_MANIFEST_DIR").unwrap());
    let snapshot = manifest.join("../internal/gamewiki/snapshot").canonicalize().expect("找不到 internal/gamewiki/snapshot");
    println!("cargo:rerun-if-changed={}", snapshot.display());
    println!("cargo:rerun-if-changed=build.rs");

    let catalog_path = snapshot.join("catalog.json.gz");
    let catalog: serde_json::Value = serde_json::from_slice(&gunzip(&catalog_path)).expect("catalog.json.gz 不是 JSON");
    let revision = catalog["revision"].as_str().expect("catalog 没有 revision");
    let dir = snapshot.join(revision);
    let path_of = |p: &Path| p.canonicalize().unwrap().display().to_string().replace('\\', "/");

    let mut code = String::new();
    writeln!(code, "pub const REVISION: &str = {revision:?};").unwrap();
    writeln!(code, "pub static CATALOG: &[u8] = include_bytes!({:?});", path_of(&catalog_path)).unwrap();
    writeln!(code, "pub static CATEGORIES: &[(&str, &[u8])] = &[").unwrap();
    for c in catalog["categories"].as_array().expect("catalog 没有 categories") {
        let id = c["id"].as_str().unwrap();
        writeln!(code, "    ({id:?}, include_bytes!({:?})),", path_of(&dir.join(format!("category-{id}.json.gz")))).unwrap();
    }
    writeln!(code, "];").unwrap();
    writeln!(code, "pub static SHARDS: [&[u8]; 256] = [").unwrap();
    for i in 0..256 {
        writeln!(code, "    include_bytes!({:?}),", path_of(&dir.join(format!("{i:02x}.json.gz")))).unwrap();
    }
    writeln!(code, "];").unwrap();
    fs::write(PathBuf::from(env::var("OUT_DIR").unwrap()).join("wiki_data.rs"), code).unwrap();
}
