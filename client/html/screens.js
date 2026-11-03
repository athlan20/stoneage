(() => {
  const A=SA;
  const open=type=>()=>A.push(type);
  const status=p=>({battle:'出战',standby:'待命',rest:'休息'}[p.state]);
  A.views.scene=()=>({title:A.place().name,subtitle:A.place().island+' · 通过传送与对话探索',scene:true,art:'scene',text:A.place().desc,items:[
    ...A.place().npcs.map(id=>({label:A.data.npcs[id].name,meta:A.data.npcs[id].type,detail:A.data.npcs[id].text,run:()=>A.visitNpc(id)})),
    {label:'传送到其他地点',meta:'选择目的地',detail:'不需要步行。选择一个地点即可传送。',run:open('travel')},
    {label:'查找并直接对话',meta:'跨地图也可以找到 NPC',run:open('npcs')},
    ...(A.place().wild?[{label:'开始遇敌',meta:'进入一场野外战斗',run:()=>A.startBattle()}]:[]),
    {label:'打开游戏菜单',meta:'Esc',run:open('menu')}
  ]});
  A.views.menu=()=>({title:'游戏菜单',subtitle:'你的冒险工具箱 · Esc 回到场景',menu:true,items:[
    {label:'角色',meta:'状态 / 能力加点 / 精灵',run:open('character')},
    {label:'宠物',meta:'出战 / 待命 / 休息 / 改名',run:open('pets')},
    {label:'背包',meta:'物品 / 装备 / 使用 / 整理',run:open('bag')},
    {label:'任务',meta:'进度 / 攻略 / 直接对话',run:open('quests')},
    {label:'地图与传送',meta:'各村与近郊',run:open('travel')},
    {label:'社交',meta:'未开放',disabled:'社交功能尚未实现。本原型不模拟组队与玩家交易。'},
    {label:'消息',meta:'查看本次冒险记录',run:open('messages')},
    {label:'百科',meta:'分类 / 搜索 / 资料',run:open('wiki')},
    {label:'系统',meta:'控制权 / 自动战斗 / 设置',run:open('system')}
  ]});
  A.views.travel=()=>({title:'地图与传送',subtitle:'选择目的地，Enter 传送 · 无手动移动',art:'map',items:A.data.places.map(p=>({
    label:p.name,meta:p.id===A.state.place?'当前位置':p.wild?'近郊 · 可遇敌':p.island,detail:p.desc,
    disabled:p.id===A.state.place?'你已经在这里。':null,run:()=>A.teleport(p.id)
  })).concat({label:'回到回城地点',meta:A.data.places.find(p=>p.id===A.state.home).name,run:()=>A.teleport(A.state.home)})});
  A.views.npcs=f=>({title:'直接对话',subtitle:'选择 NPC；跨地图时先确认传送。',tabs:['全部','本地图'],tab:f.tab||0,onTab:tab=>{f.tab=tab;f.index=0;A.emit();},items:Object.entries(A.data.npcs).filter(([id])=>!f.tab||A.place().npcs.includes(id)).map(([id,npc])=>{
    const place=A.place().npcs.includes(id)?A.place():A.data.places.find(p=>p.npcs.includes(id));
    return {label:npc.name,meta:`${npc.type} · ${place.name}`,detail:npc.text,run:()=>A.visitNpc(id,place.id)};
  })});
  A.views.bag=f=>{
    const equip=f.tab===1;
    const slots=equip?[0,1,2,3,4]:Array.from({length:15},(_,i)=>i+5);
    const held=slots.map(A.bag).filter(Boolean);
    const parts=['头部','身体','手部','饰品一','饰品二'];
    return {title:'背包',subtitle:`持有 ${A.state.gold} 石币 · 背包 ${A.state.inventory.filter(i=>i.slot>=5).length}/15`,tabs:['物品','装备'],tab:equip?1:0,onTab:tab=>{f.tab=tab;f.index=0;A.emit();},
      items:held.map(it=>{const item=A.item(it.id);return {label:item.name,meta:equip?parts[it.slot]:`×${it.count} · 槽位 ${it.slot}`,detail:`${item.effect}\n\n${item.desc}\n\nEnter 打开动作菜单。`,run:()=>A.itemActions(it.slot)};}),
      empty:equip?'还没有穿上装备。按 ← 返回物品栏，选择小斧头试试。':'背包是空的。可以找商人购买物品。'};
  };
  A.views.pets=()=>({title:'宠物',subtitle:`伙伴 ${A.state.pets.length}/5 · 选择一只查看和管理`,art:'pet',items:A.state.pets.map(p=>({
    label:p.name,meta:`Lv.${p.level} · ${status(p)} · ${p.hp}/${p.maxHp}`,pet:p,detail:`种族：${p.species}\n等级：${p.level}\n体力：${p.hp}/${p.maxHp}\n忠诚：${p.loyalty}\n状态：${status(p)}\n\n技能：攻击、撕咬、防御`,run:()=>A.push('petActions',{slot:p.slot,modal:true})
  })),empty:'暂时没有宠物。可以在近郊战斗中捕捉。'});
  A.views.character=f=>{
    const tab=f.tab||0;
    const basics=`${A.hero().name} · Lv.${A.state.level}\n\nHP ${A.state.hp}/${A.state.maxHp}\nMP ${A.state.mp}/${A.state.maxMp}\n经验 ${A.state.exp}\n石币 ${A.state.gold}\n\n地 ${A.state.elements.地} · 水 ${A.state.elements.水} · 火 ${A.state.elements.火} · 风 ${A.state.elements.风}`;
    const abilities=Object.entries(A.state.stats).map(([name,value])=>({label:name,meta:`${value} 点`,detail:`尚有 ${A.state.skillPoints} 点可以分配。\n\nEnter 调整这项能力的加点数量。`,disabled:A.state.skillPoints===0?'没有可分配的能力点。':null,run:()=>A.addPoint(name)}));
    const magic=[{label:'岩石撞击',meta:'MP 10 · 敌方单体',detail:'地属性魔法，对一个敌人造成伤害。\n\n在战斗的“精灵”菜单中使用。',run:()=>A.detail('岩石撞击','对一个敌人造成伤害。消耗 MP 10。\n战斗中选择：精灵 → 岩石撞击 → 目标。')},{label:'治愈精灵',meta:'MP 6 · 我方单体',detail:'在战斗中恢复一名队友的 HP。',run:()=>A.detail('治愈精灵','战斗中恢复 50 HP。消耗 MP 6。')}];
    return {title:'角色',subtitle:`${A.hero().name} · 未分配能力点 ${A.state.skillPoints}`,tabs:['状态','能力','精灵'],tab,onTab:tab=>{f.tab=tab;f.index=0;A.emit();},text:tab===0?basics:null,art:tab===0?'hero':null,items:tab===0?[{label:'分配能力点',run:()=>{f.tab=1;f.index=0;A.emit();}}]:tab===1?abilities:magic};
  };
  A.views.quests=f=>{
    const tab=f.tab||0,filter=['全部','进行中','已完成'][tab];
    const list=A.state.quests.filter(q=>!tab||q.state===filter);
    return {title:'任务',subtitle:'选择任务可查看攻略或直接与相关 NPC 对话',tabs:['全部','进行中','已完成'],tab,onTab:tab=>{f.tab=tab;f.index=0;A.emit();},items:list.map(q=>({label:q.name,meta:q.state,detail:q.desc,run:()=>A.push('questActions',{id:q.id,modal:true})})),empty:'这个分类下还没有任务。'};
  };
  A.views.wiki=f=>{
    const cats=['宠物','物品','任务','地图','玩法'];
    if(f.entry){const entry=A.data.wiki.find(w=>w.id===f.entry);return {title:entry.name,subtitle:`百科 / ${entry.category}`,text:entry.text,items:[{label:'返回词条列表',run:A.back}]};}
    if(f.category||f.query!==undefined){
      const list=A.data.wiki.filter(w=>f.category?w.category===f.category:(w.name+w.text).includes(f.query));
      return {title:f.category?`百科 / ${f.category}`:`搜索「${f.query}」`,subtitle:`${list.length} 条资料`,items:list.map(w=>({label:w.name,meta:w.category,detail:w.text,run:()=>A.push('wiki',{entry:w.id})})),empty:'没有匹配的资料，Esc 返回后重新搜索。'};
    }
    return {title:'石器百科',subtitle:'资料随原型提供，不需要联网',items:[{label:'搜索资料',meta:'按名称或内容查找',run:()=>A.form('搜索百科',[{name:'query',label:'关键词',placeholder:'例如：乌力、任务、传送'}],v=>{
      if(!v.query.trim())return A.tell('请输入关键词。','错误');A.ui.stack.pop();A.push('wiki',{query:v.query.trim()});
    })},...cats.map(category=>({label:category,meta:`${A.data.wiki.filter(w=>w.category===category).length} 条`,run:()=>A.push('wiki',{category})}))]};
  };
  A.views.messages=f=>({title:'消息记录',subtitle:`共 ${A.state.messages.length} 条 · 最新消息在下方`,messages:true,items:[],text:A.state.messages.map(m=>`[${m.kind}] ${m.text}`).join('\n')});
  A.views.system=()=>({title:'系统',subtitle:`${A.ui.connected?'本地模拟连接正常':'连接已断开'} · 操控方：${A.state.controller==='agent'?'agent':'你'}`,items:[
    {label:A.state.controller==='agent'?'收回控制权':'交给 agent 操控',meta:A.state.agentOnline?'agent 在线':'agent 离线',disabled:A.state.controller!=='agent'&&!A.state.agentOnline?'agent 不在线，请先连接模拟 agent。':null,run:()=>{
      if(!A.connectionGuard())return;A.state.controller=A.state.controller==='agent'?'player':'agent';A.tell(A.state.controller==='player'?'你已收回控制权。':'agent 已接管，你可以继续浏览菜单。');
    }},
    {label:A.state.agentOnline?'断开 agent':'连接模拟 agent',run:()=>{if(!A.connectionGuard())return;A.state.agentOnline=!A.state.agentOnline;if(!A.state.agentOnline)A.state.controller='player';A.tell(A.state.agentOnline?'agent 已上线。':'agent 已离线，控制权回到玩家。');}},
    {label:'给 agent 发消息',run:()=>A.form('与 agent 私聊',[{name:'text',label:'消息',type:'textarea',maxLength:200}],v=>{
      if(!A.connectionGuard())return;if(!A.state.agentOnline)return A.tell('agent 不在线。','错误');if(!v.text.trim())return A.tell('请输入消息。','错误');A.log(`你：${v.text}`,'agent');A.log('agent：收到。我会留意你的指示。','agent');A.ui.stack.pop();A.tell('消息已发送给模拟 agent。');
    })},
    {label:'agent 令牌',meta:`${A.state.tokens.length}/5`,run:open('tokens')},
    {label:`自动战斗：${A.state.auto?'开':'关'}`,meta:'客户端自动出招，可手动接管',run:()=>A.commit('自动战斗设置已更新。',()=>{A.state.auto=!A.state.auto;})},
    {label:`连续遇敌：${A.state.encounter?'开':'关'}`,meta:A.place().wild?'当前地点可以遇敌':'当前地点无野外敌人',run:()=>{
      if(!A.state.encounter&&!A.place().wild)return A.tell('这里没有敌人，请先传送到近郊。','错误');
      A.commit('遇敌开关已更新。',()=>{A.state.encounter=!A.state.encounter;});
      if(A.state.encounter&&!A.ui.battle)A.startBattle();
    }},
    {label:'显示与回放设置',run:open('settings')},
    {label:A.ui.connected?'模拟断线':'重新连接',meta:'体验异常与恢复',run:()=>{A.ui.connected=!A.ui.connected;A.tell(A.ui.connected?'连接已恢复，当前进度保留。':'连接已断开。可以继续浏览，操作暂不可用。',A.ui.connected?'系统':'错误');}},
    {label:'操作帮助',run:()=>A.detail('操作帮助','方向键：移动当前选择\nEnter：进入或确认\nEsc：退回上一层；场景根界面打开菜单\n左右键：在物品/装备等分类间切换\nPgUp/PgDn：列表或长内容翻页\n/：打开命令栏\n\n游戏不需要手动行走。传送和直接对话就能到达目的地。\n战斗中：Space 跳过回放，f 切换倍速。')},
    {label:'返回角色选择',run:()=>A.confirm('离开世界','返回角色选择？本地模拟进度会保留。',A.leaveWorld)},
    {label:'退出游戏',run:()=>A.confirm('退出游戏','结束本次体验并返回登录界面？',()=>{A.state.auto=false;A.state.encounter=false;A.openLogin();})}
  ]});
  A.views.settings=()=>({title:'显示与回放',subtitle:'设置立即生效',items:[
    {label:`图形字符：${A.state.settings.unicode?'Unicode':'ASCII'}`,run:()=>{A.state.settings.unicode=!A.state.settings.unicode;A.emit();}},
    {label:`默认回放速度：${A.state.settings.speed}×`,run:()=>{A.state.settings.speed=A.state.settings.speed===1?2:1;A.emit();}},
    {label:'返回',run:A.back}
  ]});
  A.views.tokens=()=>({title:'agent 令牌',subtitle:'假令牌，仅供交互体验，不可用于连接真实游戏',items:[
    {label:'生成新令牌',disabled:A.state.tokens.length>=5?'最多保留 5 个令牌。':null,run:()=>A.form('生成令牌',[{name:'label',label:'备注',value:'我的助手',maxLength:32},{name:'days',label:'有效天数',type:'number',value:7,min:1,max:365}],v=>{
      if(!A.connectionGuard())return;const days=Number(v.days);if(!Number.isInteger(days)||days<1||days>365)return A.tell('有效期为 1–365 天。','错误');
      const id='demo_'+Date.now().toString(36);A.state.tokens.push({id,label:v.label||'未命名',days});A.ui.stack.pop();A.detail('新令牌 · 仅本次显示',`${id}_NOT_A_REAL_TOKEN\n\n备注：${v.label}\n有效期：${days} 天\n这是假令牌，无实际授权能力。`);
    })},
    ...A.state.tokens.map(t=>({label:t.label,meta:`${t.id} · ${t.days} 天`,run:()=>A.confirm('吊销令牌',`吊销「${t.label}」？`,()=>{if(!A.connectionGuard())return;A.state.tokens=A.state.tokens.filter(x=>x!==t);A.tell('令牌已吊销。');})})),
    {label:'返回',run:A.back}
  ]});
})();
