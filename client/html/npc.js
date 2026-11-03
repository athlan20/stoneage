(() => {
  const A=SA;
  A.visitNpc=(id,place)=>{
    if(!A.guard())return;
    if(A.ui.battle)return A.tell('战斗中不能对话。','错误');
    if(place&&A.state.place!==Number(place)){
      return A.confirm('前往对话',`${A.data.npcs[id].name}在${A.data.places.find(p=>p.id===place).name}。传送过去并开始对话吗？`,()=>{
        A.teleport(place); A.push('npc',{id,modal:true});
      });
    }
    A.push('npc',{id,modal:true});
  };
  A.views.npc=f=>{
    const npc=A.data.npcs[f.id],items=[];
    if(f.id==='merchant')items.push({label:'买东西',run:()=>A.push('shop',{mode:'buy'})},{label:'卖东西',run:()=>A.push('shop',{mode:'sell'})});
    if(f.id==='healer')items.push({label:'全员恢复',meta:'30 石币',disabled:A.state.gold<30?'石币不足，需要 30 石币。':null,run:()=>A.commit('角色与宠物已完全恢复。',()=>{A.state.gold-=30;A.state.hp=A.state.maxHp;A.state.mp=A.state.maxMp;A.state.pets.forEach(p=>p.hp=p.maxHp);A.closeModal();})});
    if(f.id==='save')items.push({label:'设为回城地点',run:()=>A.commit('回城地点已更新。',()=>{A.state.home=A.state.place;A.closeModal();})});
    if(f.id==='hunter')items.push({label:'开始切磋',meta:'模拟一场完整战斗',run:()=>A.startBattle('spar')},{label:'前往近郊',run:()=>A.teleport(A.state.place===3?6:5)});
    if(f.id==='elder'){
      const q=A.state.quests.find(q=>q.id==='first');
      if(q.state==='未接取')items.push({label:'接受委托',run:()=>A.commit('已接受任务：村长的委托。',()=>{q.state='进行中';A.closeModal();})});
      else if(q.state==='进行中')items.push({label:'交付漂亮的石头',disabled:!A.state.inventory.some(x=>x.id==='stone'&&x.slot>=5)?'背包里还没有漂亮的石头。':null,run:()=>A.commit('任务完成！获得 100 石币与 50 经验。',()=>{const it=A.state.inventory.find(x=>x.id==='stone'&&x.slot>=5);if(!it)throw Error('任务物品已不在背包。');A.removeItem(it.slot);q.state='已完成';A.state.gold+=100;A.state.exp+=50;A.closeModal();})});
      else items.push({label:'聊聊村子',run:()=>A.detail('村长','谢谢你帮了村子。带上宠物，去看看更远的地方吧。')});
    }
    if(f.id==='traveler')items.push({label:'输入暗号',run:()=>A.form('告诉旅人暗号',[{name:'text',label:'暗号',placeholder:'线索在老渔夫那里'}],v=>{
      if(v.text.trim()!=='海风')return A.tell('旅人摇了摇头：“暗号不对，再想想。”','错误');
      const q=A.state.quests.find(q=>q.id==='secret');
      if(q.state==='已完成')return A.tell('你已经领取过这份礼物了。','提示');
      if(!A.guard())return;
      if(!A.addItem('herb',3))return A.tell('背包已满，请先腾出格子。','错误');
      A.commit('暗号正确！任务完成，获得回复药 ×3。',()=>{q.state='已完成';A.closeModal();});
    })});
    if(f.id==='fisher')items.push({label:'询问暗号',run:()=>A.detail('老渔夫的线索','告诉他：海风。\n\n也可以从任务菜单选择“直接对话”，自动传送过去。')});
    items.push({label:'离开',run:A.back});
    return {title:npc.name,subtitle:npc.type,text:npc.text,items};
  };
  A.views.shop=f=>{
    const buying=f.mode==='buy';
    const stock=buying?['herb','mana','axe','armor','ring'].map(id=>({id})):A.state.inventory.filter(it=>it.slot>=5);
    return {title:buying?'买东西':'卖东西',subtitle:`持有 ${A.state.gold} 石币 · 选择商品后输入数量`,items:stock.map(it=>{
      const item=A.item(it.id),price=buying?item.price:Math.max(1,Math.floor(item.price/2));
      return {label:item.name,meta:`${price} 石币${buying?' / 件':` / 件 · 持有 ${it.count}`}`,detail:`${item.desc}\n\n${item.effect}`,run:()=>{
        const max=buying?Math.min(99,Math.floor(A.state.gold/price)):it.count;
        if(!max)return A.tell('石币不足。','错误');
        A.quantity(buying?'购买数量':'出售数量',max,n=>A.confirm(buying?'确认购买':'确认出售',`${buying?'购买':'出售'} ${item.name} ×${n}\n${buying?'需要支付':'可以获得'} ${price*n} 石币。`,()=>{
          A.commit(`${buying?'已购买':'已出售'}${item.name} ×${n}。`,()=>{
            if(buying){if(A.state.gold<price*n)throw Error('石币不足。');if(!A.addItem(it.id,n))throw Error('背包已满。');A.state.gold-=price*n;}
            else {if(!A.removeItem(it.slot,n))throw Error('物品数量已变化。');A.state.gold+=price*n;}
            while(A.current()?.type!=='shop'&&A.ui.stack.length>1)A.ui.stack.pop();
          });
        }));
      }};
    }).concat({label:'返回',run:A.back}),empty:buying?'':'背包中没有可以出售的物品。'};
  };
})();
