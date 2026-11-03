(() => {
  const A=SA;
  A.state=A.fresh();
  A.ui={screen:'login',stack:[],battle:null,connected:true,busy:false,notice:'',history:[],historyIndex:0,commandDraft:'',frame:0};
  A.handlers={}; A.views={}; A.listeners=[];
  A.current=()=>A.ui.stack.at(-1);
  A.emit=()=>{ A.ui.frame++; A.persist?.(); A.render?.(); A.listeners.forEach(fn=>fn()); };
  A.log=(text,kind='系统')=>{ A.state.messages.push({kind,text}); if(A.state.messages.length>1000)A.state.messages.shift(); };
  A.tell=(text,kind='系统')=>{ A.ui.notice=text; A.log(text,kind); A.emit(); };
  A.remember=()=>{const f=A.current();if(f&&!f.modal){A.ui.pageMemory??={};A.ui.pageMemory[f.type]={index:f.index||0,tab:f.tab||0};}};
  A.push=(type,extra={})=>{ A.remember();A.ui.stack.push({type,index:0,...(!extra.modal?A.ui.pageMemory?.[type]:{}),...extra}); A.ui.notice=''; A.emit(); };
  A.replace=(type,extra={})=>{ A.ui.stack.pop(); A.push(type,extra); };
  A.world=()=>{ A.ui.screen='world'; A.ui.stack=[{type:'scene',index:0}]; A.ui.battle=null; A.ui.notice=''; A.emit(); };
  A.back=()=>{
    if(A.ui.busy)return A.tell('操作正在提交，请稍候。');
    const f=A.current();
    if(f?.type==='command') { A.ui.commandDraft=f.values?.command||''; A.ui.stack.pop(); return A.emit(); }
    if(A.ui.battle && A.ui.stack.length===1)return A.battleBack();
    if(A.ui.stack.length>1){ A.remember();A.ui.stack.pop(); A.ui.notice=''; return A.emit(); }
    if(A.ui.screen==='world')return A.push('menu');
    if(A.ui.screen==='characters'){ A.ui.screen='login'; return A.emit(); }
    if(A.ui.screen==='create'){ return A.createBack(); }
  };
  A.confirm=(title,text,run)=>A.push('confirm',{title,text,run,modal:true,index:0});
  A.form=(title,fields,submit,extra={})=>A.push('form',{title,fields,submit,values:Object.fromEntries(fields.map(x=>[x.name,x.value??''])),modal:true,...extra});
  A.detail=(title,text)=>A.push('detail',{title,text,modal:true});
  A.connectionGuard=()=>{if(A.ui.connected)return true;A.tell('连接已断开，请从系统菜单重新连接。','错误');return false;};
  A.guard=()=>{
    if(!A.connectionGuard())return false;
    if(A.state.controller==='agent'){ A.tell('agent 正在操控。请先在系统菜单收回控制权。','提示'); return false; }
    if(A.ui.busy)return false;
    return true;
  };
  A.commit=(label,fn)=>{
    if(!A.guard())return;
    try { fn(); A.log(label); A.ui.notice=label; A.emit(); }
    catch(e){ A.tell(e.message,'错误'); }
  };
  A.place=()=>A.data.places.find(x=>x.id===A.state.place);
  A.hero=()=>A.state.characters[A.state.slot]||{name:'旅人'};
  A.item=id=>A.data.items.find(x=>x.id===id);
  A.bag=slot=>A.state.inventory.find(x=>x.slot===Number(slot));
  A.pet=slot=>A.state.pets.find(x=>x.slot===Number(slot));
  A.activePet=()=>A.state.pets.find(x=>x.state==='battle'&&x.hp>0);
  A.freeSlot=()=>{ for(let i=5;i<20;i++)if(!A.bag(i))return i; return -1; };
  A.addItem=(id,count=1)=>{
    const item=A.item(id);
    if(item.kind==='equip'){
      const free=Array.from({length:15},(_,i)=>i+5).filter(slot=>!A.bag(slot));
      if(free.length<count)return false;
      free.slice(0,count).forEach(slot=>A.state.inventory.push({slot,id,count:1}));return true;
    }
    const stack=A.state.inventory.find(x=>x.id===id&&x.slot>=5);
    if(stack){stack.count+=count;return true;}
    const slot=A.freeSlot(); if(slot<0)return false;
    A.state.inventory.push({slot,id,count}); return true;
  };
  A.removeItem=(slot,count=1)=>{
    const it=A.bag(slot); if(!it||it.count<count)return false;
    it.count-=count; if(!it.count)A.state.inventory=A.state.inventory.filter(x=>x!==it); return true;
  };
  A.moveSelection=delta=>{
    const f=A.current(); const v=A.getView(); const n=v.items?.length||0;
    if(!n)return;
    f.index=Math.max(0,Math.min(n-1,(f.index||0)+delta)); A.ui.notice=''; A.emit();
  };
  A.activate=(index)=>{
    if(A.ui.busy)return;
    const f=A.current(); const v=A.getView(); const i=index??f?.index??0;
    if(f)f.index=i;
    const row=v.items?.[i];
    if(!row)return;
    if(row.disabled)return A.tell(row.disabled,'提示');
    row.run?.();
  };
  A.getView=()=>{
    const f=A.current(); const build=A.views[f?.type];
    if(f?.modal&&build)return build(f);
    if(A.ui.screen!=='world')return A.formScreen();
    return build?build(f):{title:'未知界面',text:'按 Esc 返回。',items:[]};
  };
  A.closeModal=()=>{ while(A.ui.stack.length>1&&A.current().modal)A.ui.stack.pop(); };
  A.targetList=(run,mp=false)=>{
    A.push('targets',{title:'选择使用目标',run,mp});
  };
  A.views.targets=f=>({title:f.title,subtitle:'选择目标后，Enter 直接使用；Esc 返回。',items:[
    {label:A.hero().name,meta:`HP ${A.state.hp}/${A.state.maxHp} · MP ${A.state.mp}/${A.state.maxMp}`,run:()=>f.run('hero')},
    ...(!f.mp?A.state.pets.map(p=>({label:p.name,meta:`HP ${p.hp}/${p.maxHp}`,run:()=>f.run(p.slot)})):[])
  ]});
  A.views.confirm=f=>({title:f.title,text:f.text,subtitle:'请选择。默认取消，Esc 也可以取消。',items:[
    {label:'取消',meta:'返回，不做修改',run:A.back},{label:'确定',danger:true,run:()=>{A.ui.stack.pop();f.run();}}
  ]});
  A.views.detail=f=>({title:f.title,text:f.text,items:[{label:'返回',run:A.back}]});
  A.views.form=f=>({title:f.title,subtitle:f.subtitle,fields:f.fields,values:f.values,submit:f.submit,submitLabel:f.submitLabel||'确认',cancel:true});
  A.persist=()=>{};
})();
