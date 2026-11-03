// STAGE-167：调度回归，防止静止或隐藏的验收页重新进入无限刷新。
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

function setup() {
  const listeners = {}, pending = new Map();
  let now = 0, nextId = 0, strokes = 0;
  const nodes = Object.fromEntries(['frames','scene','seek','screen','play','time','again'].map(id => [id, {
    value: '0', dataset: {}, textContent: '', add() {},
    getContext() { return {fillRect(){}, fillText(){strokes++;}}; },
    set innerHTML(_) { throw Error('播放期间不得重建整屏 DOM'); }
  }]));
  const frame = [[0,0,'#ddd',' O '],[0,1,'#ddd','/|\\']];
  nodes.frames.textContent = JSON.stringify([{name:'一',width:80,height:24,frames:[frame,frame]},
    {name:'二',width:80,height:24,frames:[frame,frame]}]);
  const document = {hidden:false,getElementById:id=>nodes[id],addEventListener:(name,fn)=>listeners[name]=fn};
  const sandbox = {document,performance:{now:()=>now},Option:class{},
    window:{addEventListener:(name,fn)=>listeners[name]=fn},
    requestAnimationFrame:fn=>{pending.set(++nextId,fn);return nextId;},
    cancelAnimationFrame:id=>pending.delete(id)};
  vm.runInNewContext(fs.readFileSync(path.join(__dirname,'../battle_preview.js'),'utf8'),sandbox);
  return {nodes,document,listeners,pending,strokes:()=>strokes,
    step(ms){now=ms;const callbacks=[...pending.values()];pending.clear();callbacks.forEach(fn=>fn(ms));}};
}

test('初始暂停且没有定时器；绘制只使用固定 Canvas',()=>{
  const s=setup();
  assert.equal(s.pending.size,0);
  assert.equal(s.nodes.screen.dataset.renderCount,'1');
  assert.ok(s.strokes()>0);
});

test('同一帧不重画，播放一遍后停止且不无限循环',()=>{
  const s=setup();s.nodes.play.onclick();
  assert.equal(s.pending.size,1);
  s.step(10);assert.equal(s.nodes.screen.dataset.renderCount,'1');
  s.step(40);assert.equal(s.pending.size,0);
  assert.equal(s.nodes.play.textContent,'播放');
  assert.equal(s.nodes.screen.dataset.renderCount,'2');
});

test('拖进度、切换场景和后台隐藏都取消后续刷新',()=>{
  const s=setup();s.nodes.play.onclick();s.nodes.seek.value='1';s.nodes.seek.oninput();
  assert.equal(s.pending.size,0);
  s.nodes.again.onclick();s.nodes.scene.value='1';s.nodes.scene.onchange();
  assert.equal(s.pending.size,0);
  s.nodes.play.onclick();s.document.hidden=true;s.listeners.visibilitychange();
  assert.equal(s.pending.size,0);
  const count=s.nodes.screen.dataset.renderCount;s.step(10000);
  assert.equal(s.nodes.screen.dataset.renderCount,count);
  s.document.hidden=false;s.listeners.visibilitychange();assert.equal(s.pending.size,0);
});

test('跳过积压帧且离开页面时释放调度',()=>{
  const s=setup();s.nodes.play.onclick();s.step(2000);
  assert.equal(s.pending.size,0);
  assert.equal(s.nodes.seek.value,1);
  s.nodes.again.onclick();s.listeners.pagehide();assert.equal(s.pending.size,0);
});
