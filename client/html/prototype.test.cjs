const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function setup() {
  const context = {structuredClone, Date, console};
  context.window = context;
  vm.createContext(context);
  for (const file of ['data','engine','actions','npc','battle','commands','forms','screens']) {
    vm.runInContext(fs.readFileSync(path.join(__dirname, file+'.js'),'utf8'),context,{filename:file+'.js'});
  }
  const A = context.SA;
  A.ui.screen='world';A.ui.stack=[{type:'scene',index:0}];
  A.choose = label => {
    const index=A.getView().items.findIndex(x=>x.label===label);
    assert.notEqual(index,-1,'找不到选项：'+label);A.activate(index);
  };
  A.submit = values => A.current().submit(values);
  return A;
}

test('场景只有 NPC、传送、直接对话等入口，没有手动移动',()=>{
  const A=setup();assert.ok(A.getView().items.some(x=>x.label==='传送到其他地点'));
  assert.ok(!A.getView().items.some(x=>/行走|向北/.test(x.label)));
});
test('Esc 在场景打开菜单，再按返回场景',()=>{
  const A=setup();A.back();assert.equal(A.current().type,'menu');A.back();assert.equal(A.current().type,'scene');
});
test('列表选择不改变游戏数据',()=>{
  const A=setup(),before=JSON.stringify(A.state);A.moveSelection(1);assert.equal(JSON.stringify(A.state),before);
});
test('选择物品打开动作，选择装备后直接装备且不打开命令',()=>{
  const A=setup();A.push('bag');A.choose('小斧头');assert.equal(A.current().type,'itemActions');
  A.choose('装备');assert.equal(A.bag(2).id,'axe');assert.equal(A.current().type,'bag');
});
test('更换装备时旧装备回到原物品格',()=>{
  const A=setup();A.addItem('axe');A.equip(5);const next=A.state.inventory.find(x=>x.id==='axe'&&x.slot>=5);
  const slot=next.slot;A.equip(slot);assert.equal(A.bag(slot).id,'axe');assert.equal(A.bag(2).id,'axe');
});
test('一次购买多件装备时分别占格，背包不足则原子失败',()=>{
  const A=setup();assert.ok(A.addItem('armor',2));
  assert.equal(A.state.inventory.filter(x=>x.id==='armor').length,2);
  const before=JSON.stringify(A.state.inventory);assert.equal(A.addItem('armor',20),false);assert.equal(JSON.stringify(A.state.inventory),before);
});
test('回复药按目标回血并消耗一件',()=>{
  const A=setup();A.useItem(7,'hero');assert.equal(A.state.hp,180);assert.equal(A.bag(7).count,4);
});
test('满血不消耗回复药，观察模式也不消耗',()=>{
  const A=setup();A.state.hp=A.state.maxHp;A.useItem(7,'hero');assert.equal(A.bag(7).count,5);
  A.state.hp=100;A.state.controller='agent';A.useItem(7,'hero');assert.equal(A.state.hp,100);assert.equal(A.bag(7).count,5);
});
test('物品移动目的格可选空格，并可以交换物品',()=>{
  const A=setup();A.push('moveItem',{slot:5,modal:true});A.activate(1);assert.equal(A.bag(6).id,'axe');
  A.command('item move 6 7');assert.equal(A.bag(6).id,'herb');assert.equal(A.bag(7).id,'axe');
});
test('丢弃默认取消，确认后才扣除指定数量',()=>{
  const A=setup();A.push('bag');A.itemActions(7);A.choose('丢弃');A.submit({quantity:'2'});
  assert.equal(A.current().type,'confirm');assert.equal(A.current().index,0);assert.equal(A.bag(7).count,5);
  A.activate();assert.equal(A.bag(7).count,5);
  A.choose('丢弃');A.submit({quantity:'2'});A.activate(1);assert.equal(A.bag(7).count,3);
});
test('数量表单拒绝负数、小数与超限，不关闭表单',()=>{
  const A=setup();let result=0;A.quantity('数量',5,n=>result=n);
  for(const v of ['0','-1','1.5','6']){A.submit({quantity:v});assert.equal(result,0);assert.equal(A.current().type,'form');}
});
test('宠物出战切换保证只有一只出战',()=>{
  const A=setup();A.setPetState(2,'battle');assert.equal(A.activePet().slot,2);assert.equal(A.pet(0).state,'rest');
});
test('倒下宠物不能出战；取消放生保留宠物',()=>{
  const A=setup();A.pet(2).hp=0;A.setPetState(2,'battle');assert.equal(A.activePet().slot,0);
  A.push('petActions',{slot:2,modal:true});A.choose('放生');A.activate(0);assert.ok(A.pet(2));
});
test('跨地图直接对话先确认传送，取消不换图',()=>{
  const A=setup();A.visitNpc('traveler',4);assert.equal(A.state.place,1);A.activate(0);assert.equal(A.state.place,1);
  A.visitNpc('traveler',4);A.activate(1);assert.equal(A.state.place,4);assert.equal(A.current().id,'traveler');
});
test('任务接受与交付奖励只发放一次',()=>{
  const A=setup();A.visitNpc('elder');A.choose('接受委托');assert.equal(A.state.quests[0].state,'进行中');
  A.visitNpc('elder');A.choose('交付漂亮的石头');assert.equal(A.state.gold,780);assert.equal(A.state.quests[0].state,'已完成');
  A.visitNpc('elder');assert.ok(!A.getView().items.some(x=>x.label==='交付漂亮的石头'));
});
test('暗号错误不发奖，正确暗号完成任务且不能重复领奖',()=>{
  const A=setup();A.state.place=4;A.visitNpc('traveler');A.choose('输入暗号');A.submit({text:'错误'});assert.equal(A.bag(7).count,5);
  A.submit({text:'海风'});assert.equal(A.bag(7).count,8);assert.equal(A.state.quests[1].state,'已完成');
  A.visitNpc('traveler');A.choose('输入暗号');A.submit({text:'海风'});assert.equal(A.bag(7).count,8);
});
test('商店买入扣款入包，卖出扣物加款',()=>{
  const A=setup();A.visitNpc('merchant');A.choose('买东西');A.choose('回复药');A.submit({quantity:'2'});assert.equal(A.state.gold,680);A.activate(1);
  assert.equal(A.state.gold,640);assert.equal(A.bag(7).count,7);
  A.back();A.choose('卖东西');A.choose('回复药');A.submit({quantity:'3'});A.activate(1);
  assert.equal(A.state.gold,670);assert.equal(A.bag(7).count,4);
});
test('商店提交时再次检查石币，不足不会发货',()=>{
  const A=setup();A.push('shop',{mode:'buy'});A.choose('回复药');A.submit({quantity:'2'});A.state.gold=1;A.activate(1);
  assert.equal(A.bag(7).count,5);assert.equal(A.state.gold,1);
});
test('断线允许浏览，禁止传送；恢复后可以操作',()=>{
  const A=setup();A.ui.connected=false;A.push('bag');assert.equal(A.getView().title,'背包');A.teleport(2);assert.equal(A.state.place,1);
  A.ui.connected=true;A.teleport(2);assert.equal(A.state.place,2);
});
test('观察模式拦截菜单与命令操作，take 后恢复',()=>{
  const A=setup();A.state.controller='agent';A.equip(5);A.command('item equip 5');assert.equal(A.bag(5).id,'axe');
  A.command('take');A.command('item equip 5');assert.equal(A.bag(2).id,'axe');
});
test('百科分类、搜索与详情逐层返回',()=>{
  const A=setup();A.push('wiki');A.choose('宠物');A.choose('乌力');assert.equal(A.getView().title,'乌力');
  A.back();assert.equal(A.getView().title,'百科 / 宠物');A.back();A.choose('搜索资料');A.submit({query:'传送'});assert.ok(A.getView().items.length);
});
test('建角三步完成，校验能力与属性并支持返回',()=>{
  const A=setup();A.createCharacter(1);A.formScreen().submit({body:'酷哥',color:'绿色'});assert.equal(A.ui.create.step,1);
  const values={name:'新旅人',体力:'5',腕力:'5',耐久力:'5',速度:'5',地:'5',水:'5',火:'0',风:'0'};
  A.formScreen().submit({...values,火:'1'});assert.equal(A.ui.create.step,1);
  A.formScreen().submit(values);assert.equal(A.ui.create.step,2);A.formScreen().submit({home:'3'});assert.equal(A.ui.screen,'characters');assert.equal(A.state.characters[1].name,'新旅人');
});
test('不同角色的物品、位置与能力互相隔离',()=>{
  const A=setup();A.enterCharacter(0);A.state.gold=123;
  A.state.characters[1]={name:'另一个旅人',created:true,home:3};A.enterCharacter(1);A.state.gold=456;
  A.enterCharacter(0);assert.equal(A.state.gold,123);A.enterCharacter(1);assert.equal(A.state.gold,456);
});
test('删除角色要求完整名字匹配',()=>{
  const A=setup();A.deleteCharacter(0);A.submit({name:'小'});assert.ok(A.state.characters[0]);A.submit({name:'小明'});assert.equal(A.state.characters[0],null);
});
test('战斗动作先选目标，确认才提交人物指令',()=>{
  const A=setup();A.startBattle();A.choose('攻击');assert.equal(A.ui.battle.pick,'target');assert.equal(A.ui.battle.player,null);
  A.activate(0);assert.equal(A.ui.battle.phase,'pet');assert.equal(A.ui.battle.player.target,'e2');
});
test('宠物阶段可以返回修改人物指令，已提交指令保留',()=>{
  const A=setup();A.startBattle();A.battleChoice('defend');const cmd=A.ui.battle.player;A.back();
  assert.equal(A.ui.battle.phase,'player');assert.equal(A.ui.battle.player,cmd);assert.equal(A.ui.battle.revising,true);
});
test('战斗回放改变双方 HP，下一回合恢复人物阶段',()=>{
  const A=setup();A.startBattle();A.submitBattle({act:'attack',target:'e2'});A.submitBattle({act:'attack',target:'e2'});
  A.skipReplay();assert.equal(A.ui.battle.enemies[0].hp,33);assert.equal(A.state.hp,101);assert.equal(A.ui.battle.round,2);assert.equal(A.ui.battle.phase,'player');
});
test('捕捉先削弱目标，成功后宠物加入栏位',()=>{
  const A=setup();A.startBattle();A.ui.battle.enemies[0].hp=20;A.submitBattle({act:'capture',target:'e2'});A.submitBattle({act:'defend'});A.skipReplay();
  assert.equal(A.state.pets.length,3);assert.equal(A.ui.battle.enemies[0].captured,true);
});
test('胜利奖励仅发一次，结束回到战前页面',()=>{
  const A=setup();A.push('quests',{index:1});A.startBattle();A.ui.battle.enemies.forEach(e=>e.hp=1);
  A.submitBattle({act:'attack',target:'e2'});A.submitBattle({act:'attack',target:'e3'});A.skipReplay();assert.equal(A.ui.battle.result,'victory');assert.equal(A.state.gold,760);
  A.skipReplay();assert.equal(A.state.gold,760);A.endBattle();assert.equal(A.current().type,'quests');assert.equal(A.current().index,1);
});
test('超时使用已提交的人物指令，并为宠物补防御',()=>{
  const A=setup();A.startBattle();A.submitBattle({act:'attack',target:'e2'});A.ui.battle.lastTick=Date.now()-31000;A.battleTick();
  assert.equal(A.ui.battle.phase,'replay');assert.equal(A.ui.battle.player.act,'attack');assert.equal(A.ui.battle.pet.act,'defend');
});
test('战斗命令与菜单共享状态，观察方不能出招',()=>{
  const A=setup();A.startBattle();A.state.controller='agent';A.command('attack e2');assert.equal(A.ui.battle.player,null);
  A.command('take');A.command('attack e2');assert.equal(A.ui.battle.player.target,'e2');assert.equal(A.ui.battle.phase,'pet');
});
test('auto off 会立即更新状态并发出反馈',()=>{
  const A=setup();A.state.auto=true;A.command('auto off');assert.equal(A.state.auto,false);assert.match(A.ui.notice,/off/);
});
test('离开再打开页面时记住选择与分类',()=>{
  const A=setup();A.push('bag');A.current().tab=1;A.current().index=2;A.back();A.push('bag');assert.equal(A.current().tab,1);assert.equal(A.current().index,2);
});
test('不足 MP 的战斗命令不提交，与菜单禁用规则一致',()=>{
  const A=setup();A.startBattle();A.state.mp=0;A.command('magic 0 e2');assert.equal(A.ui.battle.player,null);assert.match(A.ui.notice,/MP/);
});
test('气力药不能给宠物使用，命令不会绕过目标限制',()=>{
  const A=setup();A.useItem(8,0);assert.equal(A.bag(8).count,2);A.startBattle();A.command('item 8 p0');assert.equal(A.ui.battle.player,null);assert.equal(A.bag(8).count,2);
});
test('令牌生成、有效期校验与吊销确认',()=>{
  const A=setup();A.push('tokens');A.choose('生成新令牌');A.submit({label:'测试助手',days:'0'});assert.equal(A.state.tokens.length,0);
  A.submit({label:'测试助手',days:'7'});assert.equal(A.state.tokens.length,1);A.back();A.choose('测试助手');A.activate(0);assert.equal(A.state.tokens.length,1);
  A.choose('测试助手');A.activate(1);assert.equal(A.state.tokens.length,0);
});
test('非世界屏幕也能显示确认模态层',()=>{
  const A=setup();A.ui.screen='login';A.ui.stack=[{type:'login',index:0}];A.confirm('退出游戏','确认退出？',()=>{});assert.equal(A.getView().title,'退出游戏');A.back();assert.equal(A.ui.screen,'login');
});
