// STAGE-167：Canvas 播放正式终端帧；不在动画期间创建/删除文字 DOM 节点。
(() => {
  'use strict';
  const scenes = JSON.parse(document.getElementById('frames').textContent);
  const select = document.getElementById('scene');
  const seek = document.getElementById('seek');
  const canvas = document.getElementById('screen');
  const play = document.getElementById('play');
  const time = document.getElementById('time');
  const ctx = canvas.getContext('2d', {alpha: false});
  const cellW = 9, cellH = 18, padding = 12, frameMs = 33;
  let index = 0, playing = false, timer = null, start = 0, startIndex = 0;
  let paintedScene = -1, paintedIndex = -1;
  scenes.forEach((s, i) => select.add(new Option(s.name, i)));
  const current = () => scenes[Number(select.value)];

  function draw() {
    const scene = current(), sceneIndex = Number(select.value);
    if (paintedScene === sceneIndex && paintedIndex === index) return;
    if (paintedScene !== sceneIndex) {
      canvas.width = scene.width * cellW + padding * 2;
      canvas.height = scene.height * cellH + padding * 2;
      seek.max = scene.frames.length - 1;
    }
    const before = performance.now();
    ctx.fillStyle = '#0c0c0c';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.font = '15px Consolas,"Microsoft YaHei",monospace';
    ctx.textBaseline = 'top';
    for (const [x, y, color, text] of scene.frames[index]) {
      ctx.fillStyle = color;
      let column = x;
      for (const ch of text) {
        const code = ch.codePointAt(0);
        const wide = (code >= 0x2e80 && code <= 0xa4cf) || (code >= 0xac00 && code <= 0xd7af)
          || (code >= 0xf900 && code <= 0xfaff) || (code >= 0xff01 && code <= 0xff60)
          || (code >= 0x20000 && code <= 0x3ffff);
        if (ch !== ' ') ctx.fillText(ch, padding + column * cellW, padding + y * cellH, wide ? cellW * 2 : cellW);
        column += wide ? 2 : 1;
      }
    }
    seek.value = index;
    const label = `${index + 1} / ${scene.frames.length} 帧`;
    if (time.textContent !== label) time.textContent = label;
    paintedScene = sceneIndex;
    paintedIndex = index;
    // 只保存轻量诊断值，便于验证暂停时不会继续刷新。
    canvas.dataset.renderCount = String(Number(canvas.dataset.renderCount || 0) + 1);
    canvas.dataset.renderMs = (performance.now() - before).toFixed(2);
  }

  function pause() {
    playing = false;
    if (timer !== null) cancelAnimationFrame(timer);
    timer = null;
    if (play.textContent !== '播放') play.textContent = '播放';
  }

  function step(now) {
    timer = null;
    if (!playing || document.hidden) { pause(); return; }
    // 按墙钟取最新帧，卡顿时跳过过期帧，不积压定时任务。
    index = Math.min(current().frames.length - 1, startIndex + Math.floor((now - start) / frameMs));
    draw();
    if (index === current().frames.length - 1) { pause(); return; }
    timer = requestAnimationFrame(step);
  }

  function resume(restart = false) {
    pause();
    if (restart || index === current().frames.length - 1) index = 0;
    draw();
    if (document.hidden) return;
    playing = true;
    play.textContent = '暂停';
    start = performance.now();
    startIndex = index;
    timer = requestAnimationFrame(step);
  }

  select.onchange = () => { pause(); index = 0; draw(); };
  seek.oninput = () => { pause(); index = Number(seek.value); draw(); };
  play.onclick = () => playing ? pause() : resume();
  document.getElementById('again').onclick = () => resume(true);
  document.addEventListener('visibilitychange', () => { if (document.hidden) pause(); });
  window.addEventListener('pagehide', pause);
  draw();
})();
