(() => {
  const A=SA;
  A.startBattle=(kind='wild')=>{
    if(!A.guard())return;
    if(A.ui.battle)return A.tell('已经在战斗中。','提示');
    A.ui.battle={kind,round:1,phase:'player',pick:'action',index:0,left:30,lastTick:Date.now(),nextAuto:Date.now()+1500,returnStack:A.ui.stack.filter(f=>!f.modal),
      enemies:[{id:'e2',name:'野猪',level:4,hp:75,maxHp:75},{id:'e3',name:'乌力',level:3,hp:55,maxHp:55}],
      player:null,pet:null,events:[],cursor:0,records:['进入战斗。'],speed:A.state.settings.speed||1,result:null,revising:false};
    A.ui.stack=[{type:'battle',index:0}]; A.ui.notice=''; A.log('进入战斗：'+(kind==='spar'?'猎人的切磋':'野外遇敌'),'战斗'); A.emit();
  };
  A.battleBack=()=>{
    const b=A.ui.battle;if(!b)return;
    if(b.phase==='result')return A.endBattle();
    if(b.phase==='replay')return;
    if(b.pick!=='action'){ b.pick='action'; A.current().index=0;return A.emit(); }
    if(b.phase==='pet'){ b.phase='player';b.revising=true;A.current().index=0;return A.emit(); }
    A.tell('人物指令是当前最外层。可以选择动作，或打开命令栏。','提示');
  };
  A.battleChoice=(act,extra)=>{
    if(!A.guard())return;
    const b=A.ui.battle;if(!b||!['player','pet'].includes(b.phase))return;
    const needTarget=['attack','capture','magic','heal','item','skill'].includes(act);
    if(needTarget){b.pick='target';b.chosen={act,extra};A.current().index=0;A.emit();}
    else A.submitBattle({act,extra});
  };
  A.submitBattle=cmd=>{
    if(!A.guard())return;
    const b=A.ui.battle;if(!b||!['player','pet'].includes(b.phase))return;
    if(b.phase==='player'){
      b.player=cmd;b.revising=false;
      if(A.activePet()){b.phase='pet';b.pick='action';A.current().index=0;b.nextAuto=Date.now()+1500;return A.emit();}
    } else b.pet=cmd;
    A.resolveBattle();
  };
  A.battleActName=cmd=>cmd?({attack:'攻击',magic:'岩石撞击',heal:'治愈精灵',item:'使用道具',defend:'防御',capture:'捕捉',escape:'逃跑',swap:'换宠',skill:'撕咬'}[cmd.act]||cmd.act):'未选择';
  A.views.battle=()=>{
    const b=A.ui.battle;
    if(!b)return {title:'战斗已结束',items:[{label:'返回场景',run:A.world}]};
    const base={title:`战斗 · ${b.kind==='spar'?'友好切磋':'野外'}`,battle:true,subtitle:`第 ${b.round} 回合 · ${A.battlePhase()}${b.revising?'（修改人物指令）':''}`,items:[]};
    if(b.phase==='result')return {...base,title:b.result==='victory'?'战斗胜利':b.result==='escape'?'已脱离战斗':'战斗结束',text:b.summary,items:[{label:'继续冒险',run:A.endBattle}]};
    if(b.phase==='replay')return {...base,subtitle:`结算回放 ${b.cursor}/${b.events.length} · ${b.speed}×`,items:[{label:'跳过剩余回放',run:A.skipReplay},{label:`切换倍速 · 当前 ${b.speed}×`,run:()=>{b.speed=b.speed===1?2:1;A.emit();}}]};
    if(b.pick==='target'){
      const c=b.chosen,ally=c.act==='heal'||(c.act==='item'&&A.item(A.bag(c.extra)?.id)?.kind!=='material');
      const options=ally?[{id:'hero',name:A.hero().name,hp:A.state.hp,maxHp:A.state.maxHp},...A.state.pets.map(p=>({id:`p${p.slot}`,name:p.name,hp:p.hp,maxHp:p.maxHp}))]:b.enemies.filter(e=>e.hp>0&&!e.captured);
      const targets=c.act==='item'&&A.item(A.bag(c.extra)?.id)?.kind==='mana'?options.filter(t=>t.id==='hero'):options;
      return {...base,subtitle:`${A.battleActName(c)} → 选择${ally?'我方':'敌方'}目标`,items:targets.map(t=>({label:t.name,meta:`${t.id} · HP ${t.hp}/${t.maxHp}`,run:()=>A.submitBattle({...c,target:t.id})})).concat({label:'返回',run:A.battleBack})};
    }
    if(b.pick==='magic')return {...base,subtitle:'选择精灵',items:[
      {label:'岩石撞击',meta:'MP 10 · 敌方单体',disabled:A.state.mp<10?'MP 不足。':null,run:()=>A.battleChoice('magic')},
      {label:'治愈精灵',meta:'MP 6 · 我方单体',disabled:A.state.mp<6?'MP 不足。':null,run:()=>A.battleChoice('heal')},
      {label:'返回',run:A.battleBack}]};
    if(b.pick==='item')return {...base,subtitle:'选择战斗道具',items:A.state.inventory.filter(it=>it.slot>=5&&['heal','mana'].includes(A.item(it.id).kind)).map(it=>({label:A.item(it.id).name,meta:`槽位 ${it.slot} · ×${it.count}`,run:()=>A.battleChoice('item',it.slot)})).concat({label:'返回',run:A.battleBack})};
    if(b.pick==='swap')return {...base,subtitle:'选择要出战的宠物',items:A.state.pets.map(p=>({label:p.name,meta:`${p.hp}/${p.maxHp}`,disabled:p.hp===0?'宠物已倒下。':p.state==='battle'?'已经在出战。':null,run:()=>A.battleChoice('swap',p.slot)})).concat({label:'返回',run:A.battleBack})};
    if(b.pick==='skill')return {...base,subtitle:'选择宠物技能',items:[{label:'撕咬',meta:'敌方单体 · 较高伤害',run:()=>A.battleChoice('skill')},{label:'返回',run:A.battleBack}]};
    const pick=name=>{if(!A.guard())return;b.pick=name;A.current().index=0;A.emit();};
    const player=[{label:'攻击',run:()=>A.battleChoice('attack')},{label:'精灵',run:()=>pick('magic')},{label:'道具',run:()=>pick('item')},{label:'防御',run:()=>A.battleChoice('defend')},
      {label:'捕捉',disabled:b.kind==='spar'?'切磋中不能捕捉。':A.state.pets.length>=5?'宠物栏位已满。':null,run:()=>A.battleChoice('capture')},
      {label:'逃跑',disabled:b.kind==='spar'?'切磋中不能逃跑。':null,run:()=>A.battleChoice('escape')},{label:'换宠',run:()=>pick('swap')}];
    const pet=[{label:'攻击',run:()=>A.battleChoice('attack')},{label:'技能',run:()=>pick('skill')},{label:'防御',run:()=>A.battleChoice('defend')}];
    return {...base,items:b.phase==='pet'?pet:player};
  };
  A.battlePhase=()=>({player:'人物指令',pet:'宠物指令',replay:'结算回放',result:'结果'}[A.ui.battle?.phase]||'等待');
  A.resolveBattle=()=>{
    const b=A.ui.battle;if(!b)return;
    b.phase='replay';b.pick='action';b.events=[];b.cursor=0;b.lastEvent=Date.now()-1000;A.current().index=0;
    const add=(actor,c)=>{if(c)b.events.push(()=>A.applyBattleAction(actor,c));};
    add('hero',b.player||{act:'defend'});add('pet',b.pet);
    b.enemies.forEach(e=>b.events.push(()=>{
      if(e.hp<=0||e.captured||b.escaped||A.state.hp<=0)return `${e.name}未能行动。`;
      const defend=b.player?.act==='defend',damage=defend?4:12;
      A.state.hp=Math.max(0,A.state.hp-damage);return `${e.name} → ${A.hero().name}，${defend?'防御减伤，':''}受到 ${damage} 点伤害。`;
    })); A.emit();
  };
  A.applyBattleAction=(actor,c)=>{
    const b=A.ui.battle,pet=A.activePet(),name=actor==='hero'?A.hero().name:(pet?.name||'宠物');
    if(b.escaped||A.state.hp<=0||(actor==='pet'&&!pet))return `${name}未能行动。`;
    const enemy=b.enemies.find(e=>e.id===c.target),target=c.target==='hero'?A.state:A.pet(Number(String(c.target).slice(1)));
    if(c.act==='defend')return `${name}摆出了防御姿态。`;
    if(c.act==='escape'){b.escaped=true;return '你找到了空隙，成功脱离战斗。';}
    if(c.act==='swap'){
      const next=A.pet(c.extra);if(!next||!next.hp)return '换宠失败：目标无法出战。';
      A.state.pets.forEach(p=>{if(p.state==='battle')p.state='rest';});next.state='battle';return `${next.name}加入战斗。`;
    }
    if(c.act==='capture'){
      if(!enemy||enemy.hp<=0||enemy.captured)return '捕捉目标已不在场。';
      if(enemy.hp/enemy.maxHp>0.65)return `${enemy.name}挣脱了！先削弱它的体力。`;
      if(A.state.pets.length>=5)return '捕捉失败：宠物栏位已满。';
      let slot=0;while(A.pet(slot))slot++;
      A.state.pets.push({slot,name:enemy.name,species:enemy.name,hp:enemy.hp,maxHp:enemy.maxHp,level:enemy.level,state:'rest',loyalty:70});enemy.captured=true;return `捕捉成功！${enemy.name}加入了宠物栏。`;
    }
    if(c.act==='heal'||c.act==='item'){
      if(!target)return '目标已不在场，操作未执行。';
      if(c.act==='heal'){
        if(A.state.mp<6)return 'MP 不足，治愈未能施放。';A.state.mp-=6;target.hp=Math.min(target.maxHp,target.hp+50);return `${name}施放治愈，${c.target==='hero'?A.hero().name:target.name}回复 HP。`;
      }
      const it=A.bag(c.extra);if(!it)return '道具已用完，操作未执行。';
      const item=A.item(it.id);if(item.kind==='mana')A.state.mp=Math.min(A.state.maxMp,A.state.mp+item.power);else target.hp=Math.min(target.maxHp,target.hp+item.power);
      A.removeItem(it.slot);return `${name}使用${item.name}，目标恢复了。`;
    }
    if(!enemy||enemy.hp<=0||enemy.captured)return `${name}的目标已不在场，本次未行动。`;
    const damage=c.act==='magic'?40:c.act==='skill'?28:actor==='pet'?18:24;
    if(c.act==='magic'){if(A.state.mp<10)return 'MP 不足，精灵未能施放。';A.state.mp-=10;}
    enemy.hp=Math.max(0,enemy.hp-damage);return `${name} → ${A.battleActName(c)} → ${enemy.name}，造成 ${damage} 点伤害${enemy.hp===0?'，目标倒下':''}。`;
  };
  A.playNext=()=>{
    const b=A.ui.battle;if(!b||b.phase!=='replay')return;
    if(b.cursor<b.events.length){const text=b.events[b.cursor++]();b.caption=text;b.records.push(text);A.log(text,'战斗');b.lastEvent=Date.now();}
    if(b.cursor>=b.events.length)A.finishRound();A.emit();
  };
  A.skipReplay=()=>{const b=A.ui.battle;if(!b)return;while(A.ui.battle===b&&b.phase==='replay')A.playNext();};
  A.finishRound=()=>{
    const b=A.ui.battle;let result=null;
    if(b.escaped)result='escape';else if(A.state.hp===0)result='lose';else if(b.enemies.every(e=>e.hp===0||e.captured))result='victory';
    if(result){
      b.phase='result';b.result=result;b.resultAt=Date.now();A.current().index=0;
      if(result==='victory'){A.state.gold+=80;A.state.exp+=40;b.summary='获得 80 石币 · 40 经验\n角色与宠物的状态已更新。';}
      else if(result==='escape')b.summary='成功离开战斗。没有获得奖励。';
      else {A.state.hp=1;A.state.place=A.state.home;A.state.encounter=false;b.summary='你回到了回城地点，HP 为 1。\n可以找治疗师，或使用背包里的药品。';}
      A.log(b.summary.replaceAll('\n',' '),'战斗');
    } else {b.round++;b.phase='player';b.pick='action';b.player=null;b.pet=null;b.left=30;b.lastTick=Date.now();b.nextAuto=Date.now()+1500;A.current().index=0;}
  };
  A.endBattle=()=>{const back=A.ui.battle?.returnStack;A.ui.battle=null;A.ui.stack=back?.length?back:[{type:'scene',index:0}];A.ui.notice='';A.emit();};
  A.battleTick=()=>{
    const b=A.ui.battle;if(!b)return;
    const now=Date.now();
    if(['player','pet'].includes(b.phase)){
      if(now-b.lastTick>=1000){b.left=Math.max(0,b.left-Math.floor((now-b.lastTick)/1000));b.lastTick=now;A.render?.();}
      if(!b.left){b.player=b.player||{act:'defend'};b.pet=A.activePet()?{act:'defend'}:null;A.ui.stack=[{type:'battle',index:0}];A.resolveBattle();return;}
      if((A.state.auto||A.state.controller==='agent')&&now>=b.nextAuto&&A.ui.stack.length===1){
        const enemy=b.enemies.find(e=>e.hp>0&&!e.captured);
        if(A.state.controller==='agent'){
          b.player={act:'attack',target:enemy.id};b.pet=A.activePet()?{act:'attack',target:enemy.id}:null;A.resolveBattle();
        } else A.submitBattle({act:'attack',target:enemy.id});
      }
    } else if(b.phase==='replay'&&now-b.lastEvent>=800/b.speed)A.playNext();
    else if(b.phase==='result'&&A.state.auto&&now-b.resultAt>2500)A.endBattle();
  };
})();
