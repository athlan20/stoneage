(() => {
  const A=SA;
  A.teleport=id=>{
    const place=A.data.places.find(p=>p.id===Number(id));
    if(!place)return A.tell('没有找到这个地点。','错误');
    if(A.ui.battle)return A.tell('战斗中不能传送。','错误');
    A.commit(`已传送到${place.name}。`,()=>{
      A.state.place=place.id; A.state.encounter=false;
      A.ui.stack=[{type:'scene',index:0}];
    });
  };
  A.equip=slot=>{
    const it=A.bag(slot); if(!it)return A.tell('这件物品已经不在原来的格子中。','错误');
    const item=A.item(it.id); if(item.kind!=='equip')return A.tell('这件物品不能装备。','错误');
    A.commit(`已装备${item.name}。`,()=>{
      const old=A.bag(item.part); if(old)old.slot=it.slot;
      it.slot=item.part; A.closeModal();
    });
  };
  A.unequip=slot=>{
    const it=A.bag(slot); if(!it)return;
    const free=A.freeSlot(); if(free<0)return A.tell('背包已满，先腾出一个格子。','错误');
    A.commit(`已卸下${A.item(it.id).name}。`,()=>{it.slot=free;A.closeModal();});
  };
  A.useItem=(slot,target)=>{
    const it=A.bag(slot); if(!it)return A.tell('物品已不存在，请重新选择。','错误');
    const item=A.item(it.id); const pet=target==='hero'?null:A.pet(target);
    if(target!=='hero'&&!pet)return A.tell('目标已不存在。','错误');
    if(!['heal','mana'].includes(item.kind))return A.tell('这件物品不能直接使用。','错误');
    if(item.kind==='mana'&&target!=='hero')return A.tell('气力药只能给角色使用。','错误');
    const current=item.kind==='mana'?A.state.mp:(pet?pet.hp:A.state.hp);
    const maximum=item.kind==='mana'?A.state.maxMp:(pet?pet.maxHp:A.state.maxHp);
    if(current>=maximum)return A.tell('目标已经恢复到满值，不会消耗物品。','提示');
    A.commit(`${pet?pet.name:A.hero().name}使用了${item.name}。`,()=>{
      const amount=Math.min(maximum,current+item.power);
      if(item.kind==='mana')A.state.mp=amount; else if(pet)pet.hp=amount; else A.state.hp=amount;
      A.removeItem(slot); A.ui.stack=A.ui.stack.filter(f=>!['targets','itemActions'].includes(f.type));
    });
  };
  A.itemActions=slot=>{
    if(!A.bag(slot))return;
    A.push('itemActions',{slot,modal:true});
  };
  A.views.itemActions=f=>{
    const it=A.bag(f.slot); if(!it)return {title:'物品已变化',text:'物品已移走或用完，请返回重新选择。',items:[{label:'返回',run:A.back}]};
    const item=A.item(it.id),rows=[];
    if(f.slot<5)rows.push({label:'卸下',run:()=>A.unequip(f.slot)});
    else if(item.kind==='equip')rows.push({label:'装备',meta:item.effect,run:()=>A.equip(f.slot)});
    else if(['heal','mana'].includes(item.kind))rows.push({label:'使用',meta:item.effect,run:()=>A.targetList(t=>A.useItem(f.slot,t),item.kind==='mana')});
    if(f.slot>=5)rows.push({label:'移动 / 交换',run:()=>A.push('moveItem',{slot:f.slot,modal:true})});
    rows.push({label:'查看说明',run:()=>A.detail(item.name,`${item.desc}\n\n${item.effect}\n数量：${it.count}\n槽位：${it.slot}`)});
    if(f.slot>=5)rows.push({label:'丢弃',danger:true,run:()=>A.quantity('丢弃数量',it.count,n=>A.confirm('丢弃物品',`要丢弃 ${item.name} × ${n} 吗？此操作不能撤回。`,()=>A.dropItem(f.slot,n)),1)});
    rows.push({label:'返回',run:A.back});
    return {title:item.name,subtitle:`${item.effect} · 持有 ${it.count}`,items:rows};
  };
  A.quantity=(title,max,run,value=1)=>A.form(title,[{name:'quantity',label:`数量（1–${max}）`,type:'number',value,min:1,max}],v=>{
    const n=Number(v.quantity); if(!Number.isInteger(n)||n<1||n>max)return A.tell(`请输入 1–${max} 的整数。`,'错误');
    A.ui.stack.pop(); run(n);
  });
  A.dropItem=(slot,n)=>{
    const it=A.bag(slot); if(!it||it.count<n)return A.tell('物品数量已变化，请重新选择。','错误');
    A.commit(`已丢弃${A.item(it.id).name} × ${n}。`,()=>{A.removeItem(slot,n);A.closeModal();});
  };
  A.views.moveItem=f=>({title:'选择目的格',subtitle:'空格可选；选中有物品的格子将交换位置。',items:Array.from({length:15},(_,i)=>{
    const slot=i+5,it=A.bag(slot);
    return {label:`${slot}  ${it?A.item(it.id).name:'空格'}`,meta:slot===f.slot?'当前位置':'',disabled:slot===f.slot?'已经在这个格子中。':null,run:()=>A.commit('物品位置已更新。',()=>{
      const source=A.bag(f.slot); if(!source)throw Error('原物品已不存在。');
      const other=A.bag(slot); if(other)other.slot=f.slot; source.slot=slot; A.closeModal();
    })};
  })});
  A.setPetState=(slot,state)=>{
    const pet=A.pet(slot); if(!pet)return A.tell('宠物已经不在该栏位。','错误');
    if(state==='battle'&&pet.hp===0)return A.tell('宠物已倒下，请先治疗。','错误');
    if(state==='standby'&&pet.state!=='standby'&&A.state.pets.filter(p=>p.state==='standby').length>=3)return A.tell('最多只能有 3 只待命宠物。','错误');
    const labels={battle:'出战',standby:'待命',rest:'休息'};
    A.commit(`${pet.name}已设为${labels[state]}。`,()=>{
      if(state==='battle')A.state.pets.forEach(p=>{if(p.state==='battle')p.state='rest';});
      pet.state=state; A.closeModal();
    });
  };
  A.views.petActions=f=>{
    const p=A.pet(f.slot); if(!p)return {title:'宠物已离开',items:[{label:'返回',run:A.back}]};
    return {title:p.name,subtitle:`${p.species} · Lv.${p.level} · 忠诚 ${p.loyalty}`,items:[
      {label:'设为出战',disabled:!p.hp?'宠物已倒下，请先治疗。':null,run:()=>A.setPetState(p.slot,'battle')},
      {label:'设为待命',run:()=>A.setPetState(p.slot,'standby')},
      {label:'休息',run:()=>A.setPetState(p.slot,'rest')},
      {label:'改名',run:()=>A.form('给宠物起名字',[{name:'name',label:'名字',value:p.name,maxLength:16}],v=>{
        const name=v.name.trim(); if(!name||Array.from(name).length>16)return A.tell('名字需要 1–16 个字符。','错误');
        A.commit(`宠物已改名为${name}。`,()=>{p.name=name;A.closeModal();});
      })},
      {label:'查看技能',run:()=>A.detail(`${p.name}的技能`,'攻击：攻击一名敌人。\n撕咬：造成较高伤害。\n防御：减轻本回合伤害。')},
      {label:'放生',danger:true,run:()=>A.confirm('放生宠物',`要放生 ${p.name} 吗？它将永久离开队伍。`,()=>A.commit(`${p.name}已回到大自然。`,()=>{A.state.pets=A.state.pets.filter(x=>x!==p);A.closeModal();}))},
      {label:'返回',run:A.back}
    ]};
  };
  A.addPoint=ability=>A.quantity(`分配${ability}点数`,A.state.skillPoints,n=>A.commit(`${ability} +${n}，剩余 ${A.state.skillPoints-n} 点。`,()=>{
    A.state.skillPoints-=n;A.state.stats[ability]+=n;
    if(ability==='体力'){A.state.maxHp+=n*4;A.state.hp+=n*4;}
    A.closeModal();
  }));
  A.views.questActions=f=>{
    const q=A.state.quests.find(x=>x.id===f.id);
    return {title:q.name,subtitle:q.state,text:q.desc,items:[
      {label:'查看攻略',run:()=>A.detail(q.name,A.data.wiki.find(w=>w.id===q.id).text)},
      {label:`直接对话 · ${A.data.npcs[q.npc].name}`,meta:A.data.places.find(p=>p.id===q.place).name,run:()=>A.visitNpc(q.npc,q.place)},
      {label:'返回',run:A.back}
    ]};
  };
})();
