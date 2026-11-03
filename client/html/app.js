(() => {
  const A=SA,storeKey='stoneage-cli-ui-prototype-v1';
  try{
    const saved=JSON.parse(localStorage.getItem(storeKey)||'null');
    if(saved?.version===1&&Array.isArray(saved.characters)&&saved.characters.length===2&&Array.isArray(saved.inventory)&&Array.isArray(saved.pets)&&A.data.places.some(p=>p.id===saved.place)){
      A.state={...A.fresh(),...saved};
    }
  }catch{document.getElementById('save-note').textContent='本浏览器限制本地存储，进度只在当前页面保留。';}
  A.persist=()=>{
    try{A.saveProfile();localStorage.setItem(storeKey,JSON.stringify(A.state));}
    catch{document.getElementById('save-note').textContent='本地存储不可用，进度仅保留到关闭页面。';}
  };
  const oldBack=A.back;
  A.back=()=>{
    const f=A.current();
    if(f?.cancelRun)return f.cancelRun();
    if(A.ui.screen==='register'&&A.ui.stack.length===1)return A.openLogin();
    oldBack();
  };
  A.views.confirm=(f)=>({title:f.title,text:f.text,subtitle:'默认取消 · Enter 确认当前选项 · Esc 返回',items:[
    {label:'取消',meta:'不做修改',run:A.back},{label:'确定',danger:true,run:()=>{
      A.ui.stack.pop();f.run();
    }}
  ]});
  A.submitForm=()=>{
    const f=A.current(),view=A.getView();if(!view.submit)return;
    const form=document.getElementById('active-form');
    if(form)for(const [key,value]of new FormData(form).entries())f.values[key]=value;
    view.submit({...f.values});
  };
  A.completeCommand=input=>{
    const candidates=['help','status','page 地图','page 角色','page 宠物','page 背包','page 任务','page 消息','page 系统','page 百科','warp 萨姆吉尔村','warp 玛丽娜丝渔村','warp 加加村','warp 卡鲁它那村','warp 萨姆吉尔近郊','item equip','item unequip','item use','item move','item drop','pet status','pet battle','pet standby','pet rest','pet rename','pet drop','quest','wiki find','auto on','auto off','encounter on','encounter off','take','release','kick-agent','ag','token new','token list','revoke','leave','quit','attack','magic','defend','escape','capture','swap','skill'];
    const matches=candidates.filter(c=>c.startsWith(input.value));
    if(matches.length===1){input.value=matches[0]+' ';A.current().values.command=input.value;}
    else if(matches.length)A.tell('候选：'+matches.join(' · '),'提示');
  };
  A.key=(key,event={})=>{
    const target=document.activeElement,textInput=target?.matches('input,textarea,select'),f=A.current();
    if(event.isComposing)return;
    if(event.ctrlKey&&key.toLowerCase()==='c'){
      if(textInput&&target.selectionStart!==target.selectionEnd)return;
      event.preventDefault?.();return A.confirm('退出游戏','退出体验并返回登录界面？',A.openLogin);
    }
    if(event.repeat&&['Enter','Escape'].includes(key)){event.preventDefault?.();return;}
    if(key==='Enter'&&target?.tagName==='BUTTON'){
      if(target.matches('[data-secondary],[data-extra],[data-action],[data-tab],[data-key]'))return;
      if(target.dataset.index!==undefined){event.preventDefault?.();return A.activate(Number(target.dataset.index));}
    }
    if(key==='Tab'&&A.getView().fields&&f?.type!=='command'){
      const fields=[...document.querySelectorAll('#active-form input,#active-form select,#active-form textarea,#active-form button,.secondary-actions button')];
      if(fields.length){event.preventDefault?.();const index=fields.indexOf(target);fields[(index+(event.shiftKey?-1:1)+fields.length)%fields.length].focus();return;}
    }
    if(textInput){
      if(key==='Escape'){event.preventDefault?.();return A.back();}
      if(f?.type==='command'){
        if(key==='Tab'){event.preventDefault?.();A.completeCommand(target);return;}
        if(['ArrowUp','ArrowDown'].includes(key)){
          event.preventDefault?.();const h=A.ui.history;A.ui.historyIndex=Math.max(0,Math.min(h.length,A.ui.historyIndex+(key==='ArrowUp'?-1:1)));
          target.value=h[A.ui.historyIndex]||'';f.values.command=target.value;return;
        }
      }
      if(key==='Enter'&&target.tagName!=='TEXTAREA'){event.preventDefault?.();A.submitForm();}
      if(A.ui.battle)A.ui.battle.nextAuto=Date.now()+3000;
      return;
    }
    if(key==='Escape'){event.preventDefault?.();return A.back();}
    if(key==='/'&&A.ui.screen==='world'){event.preventDefault?.();return A.openCommand();}
    const b=A.ui.battle;
    if(b&&f?.type==='battle'){
      if(key===' '&&b.phase==='replay'){event.preventDefault?.();return A.skipReplay();}
      if(key==='f'&&b.phase==='replay'){b.speed=b.speed===1?2:1;return A.emit();}
      if(key===' '&&b.phase==='result'){event.preventDefault?.();return A.endBattle();}
      b.nextAuto=Date.now()+3000;
    }
    if(A.getView().fields){
      if(key==='Enter'&&target?.closest('#active-form'))return;
      if(key==='Enter'){event.preventDefault?.();return A.submitForm();}
      return;
    }
    const view=A.getView();
    if(key==='Enter'){event.preventDefault?.();return A.activate();}
    if(key==='ArrowUp'||key==='ArrowDown'){
      event.preventDefault?.();
      if(view.messages||f.type==='detail'||f.type==='wiki'&&f.entry)return (document.querySelector('.modal .prose')||document.querySelector('.prose'))?.scrollBy({top:key==='ArrowUp'?-32:32});
      const columns=view.battle?getComputedStyle(document.querySelector('.battle-actions .view-list')).gridTemplateColumns.split(' ').length:1;
      return A.moveSelection((key==='ArrowUp'?-1:1)*columns);
    }
    if(['ArrowLeft','ArrowRight'].includes(key)){
      event.preventDefault?.();
      if(view.tabs)return view.onTab(Math.max(0,Math.min(view.tabs.length-1,(view.tab||0)+(key==='ArrowRight'?1:-1))));
      if(view.battle)return A.moveSelection(key==='ArrowRight'?1:-1);
    }
    if(['PageUp','PageDown','Home','End'].includes(key)){
      event.preventDefault?.();const prose=document.querySelector('.modal .prose')||document.querySelector('.prose');
      if(prose){prose.scrollTop=key==='Home'?0:key==='End'?prose.scrollHeight:prose.scrollTop+(key==='PageUp'?-180:180);return;}
      if(key==='Home'){f.index=0;return A.emit();}
      if(key==='End'){f.index=Math.max(0,(view.items?.length||1)-1);return A.emit();}
      return A.moveSelection(key==='PageUp'?-5:5);
    }
  };
  document.addEventListener('keydown',event=>{
    if(event.target.closest('.guide')&&!event.target.closest('#game')&&!['Escape','/'].includes(event.key))return;
    A.key(event.key,event);
  });
  document.addEventListener('input',event=>{
    if(event.target.dataset.field){A.current().values??={};A.current().values[event.target.dataset.field]=event.target.value;}
  });
  document.addEventListener('change',event=>{
    if(event.target.dataset.field){A.current().values??={};A.current().values[event.target.dataset.field]=event.target.value;}
  });
  document.addEventListener('submit',event=>{if(event.target.id==='active-form'){event.preventDefault();A.submitForm();}});
  document.addEventListener('click',event=>{
    const key=event.target.closest('[data-key]');if(key){
      const input=document.querySelector('.modal [data-field]')||document.querySelector('#active-form [data-field]');
      if(input&&A.getView().fields&&key.dataset.key==='Enter')return A.submitForm();
      if(key.dataset.key==='Escape')return A.back();
      document.getElementById('game').focus();return A.key(key.dataset.key);
    }
    const row=event.target.closest('[data-index]');if(row){A.activate(Number(row.dataset.index));return;}
    const tab=event.target.closest('[data-tab]');if(tab){A.getView().onTab?.(Number(tab.dataset.tab));return;}
    const action=event.target.closest('[data-action]');if(action){
      if(action.dataset.action==='back')A.back();
      if(action.dataset.action==='take'&&A.connectionGuard()){A.state.controller='player';A.tell('你已收回控制权。');}return;
    }
    const secondary=event.target.closest('[data-secondary]');if(secondary){A.getView().secondary[Number(secondary.dataset.secondary)].run();return;}
    const extra=event.target.closest('[data-extra]');if(extra){A.getView().extraActions[Number(extra.dataset.extra)].run();return;}
  });
  A.ensureWorld=()=>{
    A.ui.connected=true;A.state.controller='player';
    if(!A.state.characters.some(Boolean))A.state.characters[0]=A.fresh().characters[0];
    if(A.ui.characterActive)A.world();else A.enterCharacter(A.state.characters.findIndex(Boolean));
  };
  const scenarios=[
    ['01','开始冒险',()=>{A.ensureWorld();}],
    ['02','登录、选角与建角',A.openLogin],
    ['03','背包与装备',()=>{A.ensureWorld();A.push('menu');A.push('bag');}],
    ['04','商店与数量输入',()=>{A.ensureWorld();A.state.place=1;A.visitNpc('merchant');}],
    ['05','任务与直接对话',()=>{A.ensureWorld();A.push('menu');A.push('quests');}],
    ['06','回合制战斗',()=>{A.ensureWorld();A.state.place=5;A.state.hp=Math.max(A.state.hp,100);A.startBattle();}],
    ['07','agent 观察模式',()=>{A.ensureWorld();A.state.agentOnline=true;A.state.controller='agent';A.push('bag');}],
    ['08','断线与重新连接',()=>{A.ensureWorld();A.ui.connected=false;A.push('system');}]
  ];
  document.getElementById('scenarios').innerHTML=scenarios.map(([no,title],i)=>`<button data-scenario="${i}"><span>${no}</span>${title}<b>↗</b></button>`).join('');
  document.getElementById('scenarios').addEventListener('click',event=>{const button=event.target.closest('[data-scenario]');if(button)scenarios[Number(button.dataset.scenario)][2]();});
  document.getElementById('reset').addEventListener('click',()=>{
    const reset=()=>{A.state=A.fresh();A.ui.characterActive=false;A.ui.history=[];A.ui.pageMemory={};A.ui.commandDraft='';A.ui.connected=true;A.openLogin();A.tell('示例数据已重置。');};
    if(A.ui.screen!=='world'){A.ui.screen='world';A.ui.stack=[{type:'scene',index:0}];}
    A.confirm('重置体验','将清除本原型的所有模拟进度，恢复初始示例数据。',reset);
  });
  document.querySelector('.brand').addEventListener('click',event=>{event.preventDefault();A.openLogin();});
  A.openLogin();
  setInterval(()=>{
    A.battleTick();
    if(!A.ui.battle&&A.state.encounter&&A.ui.screen==='world'&&A.current()?.type==='scene'&&A.ui.connected&&A.state.controller==='player'){
      A.ui.encounterDue??=Date.now()+1800;
      if(Date.now()>A.ui.encounterDue){A.ui.encounterDue=null;A.startBattle();}
    }else A.ui.encounterDue=null;
  },150);
})();
