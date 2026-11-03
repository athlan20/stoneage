(() => {
  const A=SA;
  A.escape=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
  const e=A.escape;
  const art={
    welcome:'       /\\      /\\\n      /  \\____/  \\\n     /   STONE    \\\n    /_____AGE______\\\n        /  \\      \n   ____/____\\____  \n  /   A D V E N T  \\\n /__________________\\',
    characters:'        .-~~~~-.\n       /  o  o  \\\n      |    __    |\n       \\  \\_/  /\n        `-.__.-´\n       /|  /\\  |\\\n      /_|_/__\\_|_\\\n        /  /\\  \\\n       /__/  \\__\\',
    hero:'       .---.\n      / o o \\\n      |  ^  |\n       \\_-_/\n      /|===|\\\n     /_|___|_\\\n       /   \\\n      /_/ \\_\\',
    pet:'       .--.      .--.\n      /    \\____/    \\\n     |   o        o   |\n      \\     __      /\n       `-._ \\__/ _.-´\n        /  `----´  \\\n       /  /    \\  \\\n      (_ /      \\ _)',
    scene:'             _      _\n           _/ \\____/ \\_\n         _/            \\_\n        /     /\\  /\\     \\\n       /_____/__\\/__\\_____\\\n       |  []  |    |  []  |\n       |______|_[]_|______|\n    ~~     🌿        🌿     ~~\n      ~~~~~~~~~~~~~~~~~~~~~',
    map:'     .---.\n   ./     `---.\n  /  北岛      \\\n  \\      ●     /\n   `--.   .--´\n       `--´\n                .----.\n              ./  南岛 `-.\n             /    ●      /\n             `--.    .--´\n                 `--´'
  };
  function gauge(value,max,kind='hp'){
    return `<span class="gauge ${kind}" aria-label="${kind.toUpperCase()} ${value}/${max}"><i style="width:${Math.max(0,Math.min(100,value/max*100))}%"></i></span>`;
  }
  function form(view,frame){
    frame.values??={};
    const fields=view.fields.map(field=>{
      if(frame.values[field.name]===undefined)frame.values[field.name]=field.value??'';
      const value=frame.values[field.name],id='field-'+field.name;
      let input;
      const attr=`id="${e(id)}" aria-labelledby="${e(id)}-label" name="${e(field.name)}" data-field="${e(field.name)}"`;
      if(field.type==='select')input=`<select ${attr}>${field.options.map(o=>{const val=typeof o==='object'?o.value:o,label=typeof o==='object'?o.label:o;return `<option value="${e(val)}" ${String(value)===String(val)?'selected':''}>${e(label)}</option>`;}).join('')}</select>`;
      else if(field.type==='textarea')input=`<textarea ${attr} maxlength="${field.maxLength||200}" rows="3">${e(value)}</textarea>`;
      else input=`<input ${attr} type="${e(field.type||'text')}" value="${e(value)}" placeholder="${e(field.placeholder||'')}" ${field.min!==undefined?`min="${field.min}"`:''} ${field.max!==undefined?`max="${field.max}"`:''} ${field.maxLength?`maxlength="${field.maxLength}"`:''} autocomplete="${field.type==='password'?'new-password':'off'}">`;
      return `<label class="field ${field.type==='number'?'numeric-field':''}" for="${e(id)}"><span id="${e(id)}-label">${e(field.label)}</span>${input}</label>`;
    }).join('');
    return `<form id="active-form" novalidate><div class="form-fields">${fields}</div><div class="form-buttons"><button type="submit" class="primary">${e(view.submitLabel||'确认')} <span>↵</span></button>${view.cancel?'<button type="button" data-action="back">返回 <span>Esc</span></button>':''}</div></form>${view.secondary?`<div class="secondary-actions">${view.secondary.map((x,i)=>`<button data-secondary="${i}">${e(x.label)}</button>`).join('')}</div>`:''}`;
  }
  function rows(view,frame){
    const items=view.items||[];frame.index=Math.min(Math.max(frame.index||0,0),Math.max(0,items.length-1));
    return `<div class="view-list" role="listbox" aria-label="${e(view.title)}">${items.map((row,i)=>`<button type="button" role="option" aria-selected="${i===frame.index}" ${row.disabled?'aria-disabled="true"':''} data-index="${i}" class="menu-row ${i===frame.index?'selected':''} ${row.disabled?'unavailable':''} ${row.danger?'danger-row':''}"><span class="pointer">${i===frame.index?(A.state.settings.unicode?'▶':'>'):''}</span><span class="row-name">${e(row.label)}</span><span class="row-meta">${e(row.meta||'')}</span><span class="row-arrow">${i===frame.index?'↵':''}</span></button>`).join('')}${!items.length&&view.empty?`<div class="empty-state">${e(view.empty)}</div>`:''}</div>`;
  }
  function preview(view,frame){
    const row=view.items?.[frame.index||0];
    const text=row?.disabled?row.disabled:row?.detail;
    let image=view.art;
    if(view.scene)image='scene';
    const showArt=image&&art[image];
    const pet=row?.pet;
    return `<aside class="preview-pane"><div class="panel-caption">${view.scene?'当前场景':view.art==='map'?'世界方位 · 示意':'详情预览'}</div>${showArt?`<pre class="ascii-art ${view.art==='map'?'map-art':''}" aria-hidden="true">${e(art[image])}</pre>`:''}${pet?`<div class="pet-portrait">${e(pet.species)}</div>${gauge(pet.hp,pet.maxHp)}`:''}${text?`<div class="preview-copy">${e(text)}</div>`:view.text?`<div class="preview-copy">${e(view.text)}</div>`:'<div class="preview-copy muted">选择一项，Enter 打开。<br>Esc 随时返回上一层。</div>'}${view.scene?`<div class="location-stamp">⌖ ${e(A.place().island)}<br><strong>${e(A.place().name)}</strong></div>`:''}</aside>`;
  }
  function battleField(view){
    const b=A.ui.battle;if(!b)return '';
    const p=A.activePet(),selected=view.items?.[A.current().index||0]?.label;
    const unit=(name,hp,max,extra,enemy=false)=>`<div class="unit-card ${enemy?'enemy':'ally'} ${hp<=0?'fallen':''} ${selected===name?'targeted':''}"><div class="unit-name">${e(name)} <small>${e(extra)}</small></div>${gauge(hp,max)}<div class="unit-numbers">${hp}/${max}${hp<=0?' · 倒下':''}</div></div>`;
    return `<div class="battle-field"><div class="field-label">敌方 / 前排</div><div class="unit-line">${b.enemies.map(en=>unit(en.name,en.captured?0:en.hp,en.maxHp,en.captured?'已捕获':en.id,true)).join('')}</div><div class="battle-caption" aria-live="polite">${e(b.caption||'与伙伴并肩作战。')}</div><div class="unit-line">${unit(A.hero().name,A.state.hp,A.state.maxHp,'你')}${p?unit(p.name,p.hp,p.maxHp,'出战宠物'):''}</div><div class="field-label">我方 / ${A.state.controller==='agent'?'agent 正在出招':'等待你的指令'}</div></div>`;
  }
  function body(view,frame,isModal=false){
    const tabs=view.tabs?`<nav class="page-tabs" aria-label="分类">${view.tabs.map((t,i)=>`<button data-tab="${i}" class="${i===view.tab?'active':''}">${e(t)}</button>`).join('')}<span>← → 切换</span></nav>`:'';
    const steps=view.steps?`<div class="steps">${view.steps.map((s,i)=>`<span class="${i===view.activeStep?'active':''}">${i+1} ${e(s)}</span>`).join('')}</div>`:'';
    const field=view.battle?battleField(view):'';
    const text=view.text&&!view.scene?`<div class="prose ${view.messages?'message-history':''}" tabindex="0">${e(view.text)}</div>`:'';
    const content=view.fields?`${text}${form(view,frame)}`:view.battle?`${field}${view.text?text:''}<div class="battle-actions">${rows(view,frame)}</div>`:`<div class="content-grid ${view.art||view.items?.some(x=>x.detail)?'with-preview':''}"><div class="main-content">${text}${rows(view,frame)}</div>${!isModal&&(view.art||view.items?.some(x=>x.detail))?preview(view,frame):''}</div>`;
    const extra=view.extraActions?`<div class="extra-actions">${view.extraActions.map((x,i)=>`<button data-extra="${i}" class="${x.danger?'danger-text':''}">${e(x.label)}</button>`).join('')}</div>`:'';
    return `<div class="view ${view.battle?'battle-view':''} ${view.fields?'form-view':''}"><div class="view-heading"><div><span class="view-kicker">${view.battle?'BATTLE':A.ui.screen==='world'?'ADVENTURE':'YOUR JOURNEY'}</span><h2>${e(view.title)}</h2><p>${e(view.subtitle||'')}</p></div>${view.battle&&A.ui.battle?.phase!=='result'?`<div class="countdown ${A.ui.battle.left<=8?'urgent':''}">${A.ui.battle.left}<small>SEC</small></div>`:''}</div>${steps}${tabs}${content}${extra}</div>`;
  }
  A.render=()=>{
    if(typeof document==='undefined')return;
    const root=document.getElementById('game');if(!root)return;
    const active=document.activeElement,activeField=active?.dataset?.field,selection=activeField&&active.type!=='number'?active.selectionStart:null;
    const formButton=active?.closest('#active-form')&&active.tagName==='BUTTON'?active.type:null;
    const oldList=root.querySelector('.modal .view-list')||root.querySelector('.view-list');
    const oldScroll=oldList?.scrollTop||0,oldProse=root.querySelector('.prose'),proseScroll=oldProse?.scrollTop||0;
    const f=A.current()||{type:'login',index:0,values:{}};const view=A.getView();
    const sameForm=A.ui.renderedFormFrame===f&&A.ui.renderedFormTitle===view.title;
    const isWorld=A.ui.screen==='world',modal=!!f.modal;
    let under='';
    if(modal){const base=[...A.ui.stack].reverse().find(x=>!x.modal);if(base&&A.views[base.type])under=body(A.views[base.type](base),base);}
    const crumbs=A.ui.stack.filter(x=>!x.modal).map(x=>({scene:'场景',menu:'游戏菜单',bag:'背包',pets:'宠物',character:'角色',quests:'任务',travel:'地图',wiki:'百科',npcs:'直接对话',system:'系统',shop:'商店',battle:'战斗',messages:'消息',tokens:'令牌',settings:'设置',targets:'选择目标',characterBridge:'角色管理'}[x.type]||'管理'));
    const hp=isWorld?`<div class="mini-stats"><span>${e(A.hero().name)} <small>Lv.${A.state.level}</small></span><span class="hp-label">HP ${A.state.hp}/${A.state.maxHp}</span><span class="mp-label">MP ${A.state.mp}/${A.state.maxMp}</span><span class="gold-label">◈ ${A.state.gold}</span></div>`:'<span class="opening-label">石器时代 · 你的下一段冒险</span>';
    const observer=A.state.controller==='agent'&&isWorld;
    const badge=isWorld?`<div class="state-flags">${!A.ui.connected?'<span class="error-flag">连接已断开</span>':''}${observer?'<button data-action="take" class="observer-flag">agent 操控中 · 收回控制权</button>':''}${A.state.auto?'<span class="auto-flag">自动战斗</span>':''}</div>`:'';
    const msg=A.state.messages.slice(-2).map(m=>`<div class="log-line"><span class="log-kind kind-${m.kind==='错误'?'error':m.kind==='战斗'?'battle':m.kind==='agent'?'agent':'system'}">${e(m.kind)}</span><span>${e(m.text)}</span></div>`).join('');
    let hint=view.fields?'Tab 切字段 · Enter 提交 · Esc 返回':A.ui.battle&&f.type==='battle'?(A.ui.battle.phase==='replay'?'Enter 跳过 · Space 跳过 · f 倍速':'↑↓ 选择 · Enter 确认 · Esc 返回 · / 命令'):'↑↓ 选择 · Enter 确认 · '+(f.type==='scene'?'Esc 游戏菜单':'Esc 返回')+' · / 命令';
    if(view.tabs)hint='←→ 分类 · '+hint;
    if(f.type==='command')hint='↑↓ 历史 · Tab 补全 · Enter 执行 · Esc 收起';
    if(!isWorld&&!modal)hint=A.ui.screen==='login'?'Tab 切字段与按钮 · Enter 登录':view.fields?'Tab 切字段 · Enter 确认 · Esc 上一步':'↑↓ 选择 · Enter 确认 · Esc 返回登录';
    root.innerHTML=`<div class="game-top">${hp}${badge}</div><div class="breadcrumb">${isWorld?e(crumbs.join(' / ')):'WELCOME TO STONE AGE'}<span>${isWorld?'LOCAL WORLD':'LOCAL DEMO'}</span></div><div class="stage ${modal?'has-modal':''}">${modal?`<div class="underlay" inert>${under}</div><div class="modal-shade"></div><section class="modal" role="dialog" aria-modal="true" aria-label="${e(view.title)}">${body(view,f,true)}</section>`:body(view,f)}</div><div class="feedback" role="status" aria-live="polite">${A.ui.notice?`<span class="feedback-marker">!</span> ${e(A.ui.notice)}`:''}</div>${isWorld&&f.type!=='messages'?`<div class="recent-log">${msg}</div>`:''}<div class="game-hint">${e(hint)}</div>`;
    const list=root.querySelector('.modal .view-list')||root.querySelector('.view-list');
    if(list){
      list.scrollTop=oldScroll;const selected=list.querySelector('.selected');
      if(selected){const row=selected.getBoundingClientRect(),box=list.getBoundingClientRect();if(row.top<box.top)list.scrollTop-=box.top-row.top;else if(row.bottom>box.bottom)list.scrollTop+=row.bottom-box.bottom;}
    }
    const prose=root.querySelector('.prose');if(prose)prose.scrollTop=view.messages&&A.ui.lastType!=='messages'?prose.scrollHeight:proseScroll;
    if(view.fields){
      const input=root.querySelector(`[data-field="${CSS.escape(sameForm&&activeField||view.fields[0].name)}"]`);
      if(formButton&&sameForm){root.querySelector(`#active-form button[type="${formButton}"]`)?.focus({preventScroll:true});}
      else if(input){input.focus({preventScroll:true});if(sameForm&&selection!==null&&['text','password','search'].includes(input.type))input.setSelectionRange(selection,selection);}
    }else root.focus({preventScroll:true});
    A.ui.renderedFormFrame=view.fields?f:null;A.ui.renderedFormTitle=view.title;
    document.getElementById('focus-title').textContent=view.title;
    document.getElementById('focus-note').textContent=view.fields?'当前在输入表单。Enter 提交，Esc 返回。':observer?'agent 操控中。可以浏览，点击状态条可收回控制权。':view.battle?'方向键选动作或目标，确认后才出招。':f.type==='scene'?'选择 NPC 直接对话，或打开地图传送。Esc 打开游戏菜单。':'当前高亮项接收操作。Enter 打开，Esc 退回上一层。';
    A.ui.lastType=f.type;
  };
})();
