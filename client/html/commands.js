(() => {
  const A=SA;
  const help='page 地图|角色|宠物|背包|任务|消息|系统|百科\nwarp 地点名或编号\nnpc 名字 / talk NPC名\nitem equip|unequip|use|move|drop 槽位 [目标/数量]\npet status|battle|standby|rest|rename|drop 编号 [名字]\naddpoint 体力|腕力|耐久力|速度 [数量]\nquest [doing|done|all|任务名]\nwiki [find 关键词]\nencounter on|off / auto on|off\ntake / release / kick-agent / ag 消息\ntoken new [备注] [天数] / token list / revoke 编号\nleave / quit / status / help\n\n战斗：attack e2、magic 0 e2、magic 1 m8、item 7 m8、defend、capture e2、escape、swap 2、skill 1 e2\n\n菜单和命令使用同一份模拟状态。';
  A.openCommand=()=>{
    if(A.ui.screen!=='world'||A.current()?.type==='command')return;
    A.push('command',{modal:true,values:{command:A.ui.commandDraft||''}});
  };
  A.views.command=f=>({title:'命令栏',subtitle:'Enter 执行 · Esc 收起并保留草稿 · ↑↓ 历史 · Tab 补全',fields:[{name:'command',label:'>',value:f.values?.command||'',placeholder:'输入 help 查看支持的命令',autocomplete:'off'}],values:f.values,submitLabel:'执行命令',cancel:true,submit:v=>{
    const command=v.command.trim();if(!command)return;
    A.ui.history.push(command);if(A.ui.history.length>100)A.ui.history.shift();A.ui.historyIndex=A.ui.history.length;
    A.ui.stack.pop();A.ui.commandDraft='';A.command(command);
  }});
  const positive=(x,fallback=1)=>{const n=Number(x??fallback);return Number.isInteger(n)&&n>0?n:null;};
  const pageMap={地图:'travel',角色:'character',宠物:'pets',背包:'bag',任务:'quests',消息:'messages',系统:'system',百科:'wiki',社交:'social'};
  const numericPages=['travel','character','pets','bag','quests','social','messages','system','wiki'];
  A.command=line=>{
    const [raw,...args]=line.split(/\s+/),name=raw.toLowerCase(),rest=args.join(' ');
    A.log(line,'命令');A.ui.notice='';
    try{
      if(name==='help')return A.detail('命令帮助',help);
      if(A.ui.battle&&A.battleCommand(name,args))return;
      if(['take','release','kick-agent'].includes(name)){
        if(!A.connectionGuard())return;
        if(name==='release'&&!A.state.agentOnline)return A.tell('agent 未在线。','错误');
        A.state.controller=name==='release'?'agent':'player';
        if(name==='kick-agent')A.state.agentOnline=false;
        return A.tell(name==='release'?'已交给 agent。':'已收回控制权。');
      }
      if(name==='ag'){if(!A.connectionGuard())return;if(!A.state.agentOnline)return A.tell('agent 不在线。','错误');if(!rest)return A.tell('请输入消息内容。','错误');A.log('你：'+rest,'agent');A.log('agent：收到。','agent');return A.tell('消息已发送。');}
      if(name==='status')return A.detail('当前状态',`${A.hero().name} · Lv.${A.state.level}\nHP ${A.state.hp}/${A.state.maxHp}\nMP ${A.state.mp}/${A.state.maxMp}\n石币 ${A.state.gold}\n地点 ${A.place().name}`);
      if(['page','页'].includes(name)){
        if(A.ui.battle)return A.tell('战斗中不能切页。','错误');
        const type=pageMap[rest]||numericPages[Number(rest)-1];
        if(!type)return A.tell('用 page 页名 或 page 1–9。','错误');
        if(type==='social')return A.tell('社交功能尚未实现。','提示');
        return A.push(type);
      }
      if(['auto','auto-battle','托管'].includes(name)){
        if(!rest||rest==='status')return A.tell('自动战斗：'+(A.state.auto?'开':'关'));
        if(!['on','off'].includes(rest))return A.tell('用 auto on 或 auto off。','错误');
        return A.commit('自动战斗：'+rest,()=>A.state.auto=rest==='on');
      }
      if(name==='battle')return A.ui.battle?A.tell('当前：'+A.battlePhase()):A.tell('当前不在战斗中。');
      if(name==='quit'||name==='exit')return A.confirm('退出游戏','退出并回到登录界面？',A.openLogin);
      if(A.ui.battle)return A.tell('这个命令不能在战斗中使用。','错误');
      if(name==='leave'||name==='logout')return A.leaveWorld();
      if(name==='warp'||name==='传送'){
        const p=A.data.places.find(p=>p.id===Number(rest)||p.name===rest);return p?A.teleport(p.id):A.tell('没有这个地点。','错误');
      }
      if(name==='talk'||name==='npc'||name==='对话'){
        const entry=Object.entries(A.data.npcs).find(([id,n])=>id===rest||n.name===rest);
        if(!entry)return A.tell('没有找到 NPC，可从“直接对话”菜单查看。','错误');
        const place=A.place().npcs.includes(entry[0])?A.place():A.data.places.find(p=>p.npcs.includes(entry[0]));return A.visitNpc(entry[0],place.id);
      }
      if(name==='choose'){
        const f=A.current();if(f.type!=='npc')return A.tell('请先和 NPC 对话。','错误');
        const n=positive(args[0]);if(!n||n>A.getView().items.length)return A.tell('没有这个选项。','错误');return A.activate(n-1);
      }
      if(name==='reply'){
        if(A.current().type!=='npc'||A.current().id!=='traveler')return A.tell('当前没有等待文字回复的对话。','错误');
        A.activate(0);const form=A.current();if(form.type==='form')form.submit({text:rest});return;
      }
      if(name==='item')return A.itemCommand(args);
      if(name==='pet')return A.petCommand(args);
      if(name==='addpoint'){
        const key=args[0]==='耐久'?'耐久力':args[0],n=positive(args[1]);
        if(!(key in A.state.stats)||!n||n>A.state.skillPoints)return A.tell('能力名或点数无效，或剩余点数不足。','错误');
        return A.commit(`${key} +${n}。`,()=>{A.state.stats[key]+=n;A.state.skillPoints-=n;if(key==='体力'){A.state.maxHp+=n*4;A.state.hp+=n*4;}});
      }
      if(name==='quest'){
        const q=A.state.quests.find(q=>q.name===rest||q.id===rest);
        return q?A.push('questActions',{id:q.id,modal:true}):A.push('quests',{tab:rest==='doing'?1:rest==='done'?2:0});
      }
      if(name==='wiki'||name==='百科')return args[0]==='find'?A.push('wiki',{query:args.slice(1).join(' ')}):A.push('wiki');
      if(name==='encounter'||name==='遇敌'){
        if(!['on','off'].includes(rest))return A.tell('用 encounter on 或 encounter off。','错误');
        if(rest==='on'&&!A.place().wild)return A.tell('这里没有野外敌人。','错误');
        if(!A.guard())return;A.state.encounter=rest==='on';return A.state.encounter?A.startBattle():A.tell('连续遇敌已关闭。');
      }
      if(name==='token'){
        if(args[0]!=='new')return A.push('tokens');
        A.push('tokens');A.activate(0);
        const last=args.at(-1),days=/^\d+$/.test(last)?Number(last):7,label=args.slice(1,/^\d+$/.test(last)?-1:undefined).join(' ')||'我的助手';
        return A.current().submit?.({label,days});
      }
      if(name==='revoke'){
        const token=A.state.tokens[Number(args[0])-1];if(!token)return A.tell('没有这个令牌编号。','错误');
        return A.confirm('吊销令牌',`吊销「${token.label}」？`,()=>{if(!A.connectionGuard())return;A.state.tokens=A.state.tokens.filter(t=>t!==token);A.tell('令牌已吊销。');});
      }
      A.tell('未知命令。输入 help 查看支持的命令。','错误');
    }catch(e){A.tell(e.message,'错误');}
  };
  A.itemCommand=args=>{
    const [act,raw,to,yes]=args,slot=Number(raw),it=A.bag(slot);
    if(!it)return A.tell('该槽位没有物品。','错误');
    if(act==='equip')return A.equip(slot);
    if(act==='unequip')return slot<5?A.unequip(slot):A.tell('请选择已装备的物品。','错误');
    if(act==='use')return to?A.useItem(slot,to==='self'||to==='hero'?'hero':Number(to)):A.targetList(t=>A.useItem(slot,t),A.item(it.id).kind==='mana');
    if(act==='drop'){
      if(slot<5)return A.tell('先卸下装备，再丢弃。','错误');
      const n=to==='yes'?it.count:positive(to,it.count);if(!n||n>it.count)return A.tell('数量无效。','错误');
      return to==='yes'||yes==='yes'?A.dropItem(slot,n):A.confirm('丢弃物品',`丢弃 ${A.item(it.id).name} ×${n}？`,()=>A.dropItem(slot,n));
    }
    if(act==='move'){
      const dest=Number(to);if(slot<5||!Number.isInteger(dest)||dest<5||dest>19)return A.tell('物品移动的格子范围为 5–19。','错误');
      return A.commit('物品位置已更新。',()=>{const other=A.bag(dest);if(other)other.slot=slot;it.slot=dest;});
    }
    A.tell('用 item use / equip / unequip / move / drop。','错误');
  };
  A.petCommand=args=>{
    const [act,slot,...tail]=args,p=A.pet(slot);if(!p)return A.tell('该栏位没有宠物。','错误');
    if(['battle','standby','rest'].includes(act))return A.setPetState(p.slot,act);
    if(act==='status')return A.push('petActions',{slot:p.slot,modal:true});
    if(act==='rename'){
      const name=tail.join(' ')||p.species;if(Array.from(name).length>16)return A.tell('名字最多 16 字。','错误');
      return A.commit('宠物已改名。',()=>p.name=name);
    }
    if(act==='drop'){
      const drop=()=>A.commit(`${p.name}已被放生。`,()=>A.state.pets=A.state.pets.filter(x=>x!==p));
      return tail[0]==='yes'?drop():A.confirm('放生宠物',`放生 ${p.name}？此操作不能撤回。`,drop);
    }
    A.tell('用 pet status / battle / standby / rest / rename / drop。','错误');
  };
  A.battleCommand=(name,args)=>{
    const aliases={a:'attack',m:'magic',i:'item',g:'defend',e:'escape',c:'capture',p:'swap',s:'skill'},act=aliases[name]||name;
    if(!['attack','magic','item','defend','escape','capture','swap','skill'].includes(act))return false;
    if(!A.guard())return true;
    const b=A.ui.battle;if(!['player','pet'].includes(b.phase)){A.tell('当前不能出招。','错误');return true;}
    if(b.phase==='pet'&&!['attack','skill','defend'].includes(act)){A.tell('宠物只能攻击、技能或防御。','错误');return true;}
    let command={act,target:args[0]};
    if(act==='magic'){command={act:Number(args[0])===1?'heal':'magic',target:args[1]};if(!['0','1'].includes(args[0])){A.tell('精灵编号为 0 或 1。','错误');return true;}}
    if(act==='item'||act==='swap'){command={act,extra:Number(args[0]),target:args[1]};if(act==='item'&&!['heal','mana'].includes(A.item(A.bag(command.extra)?.id)?.kind)){A.tell('这个道具不可用于战斗。','错误');return true;}}
    if(act==='skill')command.target=args[1];
    if(command.target==='m8'||command.target==='self')command.target='hero';
    if(command.act==='magic'&&A.state.mp<10||command.act==='heal'&&A.state.mp<6){A.tell('MP 不足，指令未提交。','错误');return true;}
    if(command.act==='item'&&A.item(A.bag(command.extra)?.id)?.kind==='mana'&&command.target!=='hero'){A.tell('气力药只能对角色使用。','错误');return true;}
    if(['attack','magic','skill','capture'].includes(command.act)&&!b.enemies.some(e=>e.id===command.target&&e.hp>0&&!e.captured)){A.tell('请选择在场敌人，例如 e2 或 e3。','错误');return true;}
    if(['heal','item'].includes(command.act)&&command.target!=='hero'&&!A.pet(Number(String(command.target).slice(1)))){A.tell('请选择己方目标：m8 或 p0。','错误');return true;}
    if((act==='capture'||act==='escape')&&b.kind==='spar'){A.tell('切磋中不能捕捉或逃跑。','错误');return true;}
    if(act==='capture'&&A.state.pets.length>=5){A.tell('宠物栏位已满。','错误');return true;}
    if(act==='swap'&&(!A.pet(command.extra)||!A.pet(command.extra).hp||A.pet(command.extra).state==='battle')){A.tell('宠物无法出战或已在场。','错误');return true;}
    A.ui.stack=[{type:'battle',index:0}];A.submitBattle(command);return true;
  };
})();
