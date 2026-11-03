#!/usr/bin/env node
// 石器时代命令行客户端的启动器（STAGE-98）。主包只负责找到当前平台的子包，把子包里的二进制当子进程跑：
// 参数、终端和退出码原样转交，所以 TUI 照常工作。子包由 build.mjs 生成，不要手改这里的包名规则。
'use strict';

const { spawnSync } = require('node:child_process');

const exe = process.platform === 'win32' ? 'stoneage.exe' : 'stoneage';
// 包名里 Windows 写 windows（见 build.mjs 的 slug）。
const platform = process.platform === 'win32' ? 'windows' : process.platform;
const pkg = `stoneage-cli-${platform}-${process.arch}`;

let bin;
try {
  bin = require.resolve(`${pkg}/bin/${exe}`);
} catch {
  console.error(`石器时代客户端没有 ${process.platform}-${process.arch} 的预编译包（${pkg}）。`);
  console.error('如果安装时带了 --omit=optional 或 --no-optional，去掉它重装；否则请用 cargo install 从源码构建。');
  process.exit(1);
}

const result = spawnSync(bin, process.argv.slice(2), { stdio: 'inherit' });
if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}
if (result.signal) process.kill(process.pid, result.signal);
process.exit(result.status ?? 1);
