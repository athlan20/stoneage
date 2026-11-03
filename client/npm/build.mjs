// STAGE-98：把 cargo-dist 产出的发布包拆成 npm 包（只生成目录，发布见 .github/workflows/publish-npm.yml）。
// 每个平台一个子包，只含一个二进制，靠 os/cpu（linux 另有 libc）字段让 npm 按平台自动选装；
// 主包 stoneage-cli 只放启动器 bin/stoneage.js，把子包列为 optionalDependencies。
// 用法: node client/npm/build.mjs --version 0.1.0 --archives DIR --out DIR
//   DIR 里要有 cargo-dist 的 stoneage-client-<target>.tar.xz（Windows 是 .zip），四个平台缺一不可。
import { execFileSync } from 'node:child_process';
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';

const LAUNCHER = join(dirname(fileURLToPath(import.meta.url)), 'launcher.js');
const MAIN = 'stoneage-cli';
const BIN = 'stoneage';
const DESCRIPTION = '石器时代 2.5 命令行（TUI）客户端';
const REPOSITORY = { type: 'git', url: 'git+https://github.com/athlan20/stoneage.git' };
// 子包名是 stoneage-cli-<os>-<cpu>（os 取 slug，没有就用 npm 的 os 值），和 launcher.js 拼出的名字一致。
const PLATFORMS = [
  { target: 'aarch64-apple-darwin', os: 'darwin', cpu: 'arm64', ext: 'tar.xz' },
  { target: 'x86_64-apple-darwin', os: 'darwin', cpu: 'x64', ext: 'tar.xz' },
  { target: 'x86_64-unknown-linux-gnu', os: 'linux', cpu: 'x64', libc: 'glibc', ext: 'tar.xz' },
  // 包名里写 windows 而不是 win32：npm 的垃圾包检测拒绝过 stoneage-cli-win32-x64（403 spam detection）。
  { target: 'x86_64-pc-windows-msvc', os: 'win32', slug: 'windows', cpu: 'x64', ext: 'zip' },
];
const README = `# ${MAIN}

${DESCRIPTION}。

    npm i -g ${MAIN}     # 安装后执行 stoneage
    npx ${MAIN}          # 免安装试玩

支持 macOS（Intel / Apple 芯片）、Linux x64（glibc）、Windows x64。
`;

const { values } = parseArgs({
  options: {
    version: { type: 'string' },
    archives: { type: 'string' },
    out: { type: 'string' },
  },
});
const { version, archives, out } = values;
if (!version || !archives || !out) throw new Error('用法: node client/npm/build.mjs --version X.Y.Z --archives DIR --out DIR');
if (!/^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$/.test(version)) throw new Error(`版本号不合法：${version}`);

const work = mkdtempSync(join(tmpdir(), 'stoneage-npm-'));
rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

const optionalDependencies = {};
for (const p of PLATFORMS) {
  const name = `${MAIN}-${p.slug ?? p.os}-${p.cpu}`;
  const exe = p.os === 'win32' ? `${BIN}.exe` : BIN;
  const archive = join(archives, `stoneage-client-${p.target}.${p.ext}`);
  if (!existsSync(archive)) throw new Error(`找不到 ${archive}`);

  const unpacked = join(work, p.target);
  extract(archive, unpacked);
  const found = findFile(unpacked, exe);
  if (!found) throw new Error(`${archive} 里没有 ${exe}`);

  const dir = join(out, name);
  mkdirSync(join(dir, 'bin'), { recursive: true });
  copyFileSync(found, join(dir, 'bin', exe));
  chmodSync(join(dir, 'bin', exe), 0o755);
  writeJson(join(dir, 'package.json'), {
    name,
    version,
    description: `${DESCRIPTION}（${p.os}-${p.cpu}）`,
    repository: REPOSITORY,
    os: [p.os],
    cpu: [p.cpu],
    ...(p.libc ? { libc: [p.libc] } : {}),
    files: ['bin/'],
  });
  optionalDependencies[name] = version;
}

const mainDir = join(out, MAIN);
mkdirSync(join(mainDir, 'bin'), { recursive: true });
copyFileSync(LAUNCHER, join(mainDir, 'bin', `${BIN}.js`));
chmodSync(join(mainDir, 'bin', `${BIN}.js`), 0o755);
writeFileSync(join(mainDir, 'README.md'), README);
writeJson(join(mainDir, 'package.json'), {
  name: MAIN,
  version,
  description: DESCRIPTION,
  repository: REPOSITORY,
  bin: { [BIN]: `bin/${BIN}.js` },
  files: ['bin/', 'README.md'],
  engines: { node: '>=18' },
  optionalDependencies,
});

rmSync(work, { recursive: true, force: true });
console.log(`已生成 ${PLATFORMS.length} 个平台子包和主包 ${MAIN}@${version} → ${out}`);

function extract(archive, into) {
  mkdirSync(into, { recursive: true });
  if (archive.endsWith('.zip')) execFileSync('unzip', ['-q', archive, '-d', into]);
  else execFileSync('tar', ['-xJf', archive, '-C', into]);
}

function findFile(dir, name) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      const hit = findFile(path, name);
      if (hit) return hit;
    } else if (entry.name === name) {
      return path;
    }
  }
  return null;
}

function writeJson(path, value) {
  writeFileSync(path, `${JSON.stringify(value, null, 2)}\n`);
}
