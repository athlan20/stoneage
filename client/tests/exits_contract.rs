//! Replay the shared wire contract against the Rust mock, including invalid requests.
#[allow(dead_code)]
#[path = "../src/bin/mock_server/script.rs"]
mod script;
use std::{
    io::{BufRead, BufReader, Write},
    net::TcpStream,
    path::Path,
    process::{Command, Stdio},
    time::Duration,
};
#[test]
fn exits_contract_replays_in_rust_mock() {
    let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("../docs/protocol/cases");
    let plan = script::load(&root.join("world_exits.jsonl")).unwrap();
    let mut child = Command::new(env!("CARGO_BIN_EXE_mock-server"))
        .args([
            "--case",
            "world_exits",
            "--listen",
            "127.0.0.1:0",
            "--strict",
            "--once",
            "--cases-dir",
        ])
        .arg(&root)
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    let mut log = BufReader::new(child.stderr.take().unwrap());
    let mut line = String::new();
    let addr = loop {
        line.clear();
        assert!(log.read_line(&mut line).unwrap() > 0);
        if let Some(s) = line.split("监听 ").nth(1) {
            break s.split('，').next().unwrap().to_string();
        }
    };
    // Drain child output to avoid a full pipe on the 13-exit payload.
    let drain = std::thread::spawn(move || {
        use std::io::Read;
        let mut text = String::new();
        log.read_to_string(&mut text).unwrap();
        text
    });
    let mut stream = TcpStream::connect(addr).unwrap();
    stream
        .set_read_timeout(Some(Duration::from_secs(5)))
        .unwrap();
    let mut reader = BufReader::new(stream.try_clone().unwrap());
    let mut vars = script::Vars::new();
    for step in plan.steps {
        if step.dir == script::Dir::C2s {
            writeln!(stream, "{}", script::materialize(&step.msg, &mut vars)).unwrap();
        } else {
            line.clear();
            reader.read_line(&mut line).unwrap();
            let actual = serde_json::from_str(&line).unwrap();
            let mut diffs = vec![];
            script::diff_strict(&step.msg, &actual, "msg", &mut diffs, &vars);
            assert!(diffs.is_empty(), "{diffs:?}");
        }
    }
    drop(reader);
    drop(stream);
    assert!(child.wait().unwrap().success(), "{}", drain.join().unwrap());
}
