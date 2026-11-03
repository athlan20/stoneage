(() => {
  const A=SA;
  const profileKeys=['place','home','gold','hp','maxHp','mp','maxMp','level','exp','skillPoints','stats','elements','inventory','pets','quests'];
  A.saveProfile=()=>{
    if(!A.ui.characterActive)return;
    A.state.profiles??={};
    A.state.profiles[A.hero().name]=Object.fromEntries(profileKeys.map(k=>[k,structuredClone(A.state[k])]));
  };
  A.enterCharacter=slot=>{
    const character=A.state.characters[slot];if(!character)return;
    A.saveProfile();A.state.slot=slot;
    const profile=A.state.profiles?.[character.name];
    if(profile)profileKeys.forEach(k=>A.state[k]=structuredClone(profile[k]));
    else if(slot!==0||character.created){
      const fresh=A.fresh();profileKeys.forEach(k=>A.state[k]=structuredClone(fresh[k]));
      A.state.level=1;A.state.place=character.home;A.state.home=character.home;
      A.state.stats=character.stats||{体力:5,腕力:5,耐久力:5,速度:5};
      A.state.elements=character.elements||{地:5,水:5,火:0,风:0};
    }
    A.ui.characterActive=true;A.log(`进入游戏：${character.name}。`);A.world();
  };
  A.leaveWorld=()=>{
    if(A.ui.battle)return A.tell('战斗中不能离开世界。','错误');
    A.saveProfile();A.ui.characterActive=false;A.state.encounter=false;A.state.auto=false;
    A.ui.screen='characters';A.ui.stack=[{type:'characters',index:A.state.slot}];A.emit();
  };
  A.openLogin=()=>{A.saveProfile();A.ui.characterActive=false;A.ui.battle=null;A.ui.screen='login';A.ui.stack=[{type:'login',index:0,values:{account:A.state.account,password:'demo123'}}];A.ui.notice='';A.emit();};
  A.openRegister=()=>{A.ui.screen='register';A.ui.stack=[{type:'register',index:0,values:{account:'',password:'',confirm:''}}];A.ui.notice='';A.emit();};
  A.createCharacter=slot=>{
    A.ui.create={slot,step:0,body:'辫子男孩',color:'绿色',name:'',体力:5,腕力:5,耐久力:5,速度:5,地:5,水:5,火:0,风:0,home:'1'};
    A.ui.screen='create';A.ui.stack=[{type:'create',index:0,values:{}}];A.emit();
  };
  A.createBack=()=>{
    if(A.ui.create.step>0){A.ui.create.step--;A.current().values={};return A.emit();}
    A.ui.screen='characters';A.ui.stack=[{type:'characters',index:A.ui.create.slot}];A.emit();
  };
  A.submitCreate=v=>{
    const c=A.ui.create;
    Object.assign(c,v);
    if(c.step===1){
      if(!c.name.trim()||Array.from(c.name.trim()).length>16)return A.tell('请输入 1–16 个字符的角色名。','错误');
      if(A.state.characters.some(x=>x?.name===c.name.trim()))return A.tell('本地已有同名角色。','错误');
      const sum=names=>names.reduce((n,k)=>n+Number(c[k]),0);
      if(['体力','腕力','耐久力','速度','地','水','火','风'].some(k=>!Number.isInteger(Number(c[k]))||Number(c[k])<0))return A.tell('点数必须是非负整数。','错误');
      if(sum(['体力','腕力','耐久力','速度'])!==20)return A.tell('请将 20 点能力点分配完毕。','错误');
      if(sum(['地','水','火','风'])!==10)return A.tell('请将 10 点属性点分配完毕。','错误');
      if(Number(c.地)&&Number(c.火)||Number(c.水)&&Number(c.风))return A.tell('地与火互斥，水与风互斥。','错误');
    }
    if(c.step<2){c.step++;A.current().values={};A.ui.notice='';return A.emit();}
    A.state.characters[c.slot]={name:c.name.trim(),body:c.body,color:c.color,home:Number(c.home),created:true,
      stats:Object.fromEntries(['体力','腕力','耐久力','速度'].map(k=>[k,Number(c[k])])),elements:Object.fromEntries(['地','水','火','风'].map(k=>[k,Number(c[k])]))};
    A.ui.screen='characters';A.ui.stack=[{type:'characters',index:c.slot}];A.tell('角色已创建。选择角色后进入游戏。');
  };
  A.deleteCharacter=slot=>{
    const c=A.state.characters[slot];if(!c)return;
    A.ui.screen='world';A.ui.stack=[{type:'characterBridge',index:0}];
    A.form('删除角色',[{name:'name',label:`请输入完整角色名「${c.name}」`,maxLength:16}],v=>{
      if(v.name!==c.name)return A.tell('角色名不匹配，未删除。','错误');
      A.state.characters[slot]=null;if(A.state.profiles)delete A.state.profiles[c.name];
      A.ui.screen='characters';A.ui.stack=[{type:'characters',index:slot}];A.tell('角色已删除。');
    },{cancelRun:()=>{A.ui.screen='characters';A.ui.stack=[{type:'characters',index:slot}];A.emit();},submitLabel:'永久删除'});
    A.emit();
  };
  A.views.characterBridge=()=>({title:'角色管理',text:'输入角色名以完成删除。',items:[]});
  A.views.deleteList=()=>({title:'选择要删除的角色',subtitle:'下一步需要输入完整名字确认。',items:A.state.characters.flatMap((c,slot)=>c?[{label:c.name,danger:true,run:()=>A.deleteCharacter(slot)}]:[]).concat({label:'取消',run:A.back})});
  A.formScreen=()=>{
    const f=A.current();
    if(A.ui.screen==='login')return {title:'欢迎回来，旅人。',subtitle:'登录 · 本地示例账号已填好，可直接进入',art:'welcome',fields:[
      {name:'account',label:'账号',value:A.state.account||'traveler',maxLength:20},{name:'password',label:'密码',type:'password',value:'demo123',minLength:6,maxLength:40}],
      submitLabel:'登录游戏',submit:v=>{
        if(!/^[a-zA-Z0-9_]{3,20}$/.test(v.account))return A.tell('账号请输入 3–20 位字母、数字或下划线。','错误');
        if(v.password.length<6)return A.tell('模拟密码至少 6 位。','错误');
        A.state.account=v.account;A.ui.screen='characters';A.ui.stack=[{type:'characters',index:0}];A.ui.notice='';A.emit();
      },secondary:[{label:'注册账号',run:A.openRegister},{label:'直接体验示例角色',run:()=>{const slot=A.state.characters.findIndex(Boolean);if(slot<0){A.ui.screen='characters';A.ui.stack=[{type:'characters',index:0}];return A.tell('尚无角色，选择空栏位创建旅人。');}A.enterCharacter(slot);}}],text:'任意合规账号与 6 位以上密码均可体验；此处不进行真实认证。'};
    if(A.ui.screen==='register')return {title:'结识一位新旅人',subtitle:'注册 · 仅验证表单交互，不创建真实账号',fields:[
      {name:'account',label:'账号',maxLength:20},{name:'password',label:'密码',type:'password',minLength:6},{name:'confirm',label:'确认密码',type:'password',minLength:6}],submitLabel:'注册',cancel:true,submit:v=>{
        if(!/^[a-zA-Z0-9_]{3,20}$/.test(v.account))return A.tell('账号请输入 3–20 位字母、数字或下划线。','错误');
        if(v.password.length<6||v.password!==v.confirm)return A.tell('密码至少 6 位，且两次输入必须一致。','错误');
        A.state.account=v.account;A.openLogin();A.tell('注册流程成功。账号已回填，请登录。');
      }};
    if(A.ui.screen==='characters')return {title:'选择你的旅人',subtitle:'选择角色进入游戏，选择空栏位创建角色',art:'characters',items:A.state.characters.map((c,i)=>({
      label:c?c.name:'创建新角色',meta:c?`${c.body} · ${c.color} · ${A.data.places.find(p=>p.id===c.home)?.name}`:`空栏位 ${i+1}`,
      detail:c?'Enter 进入冒险。\n选择列表中的“删除角色”可体验名字确认。':'创建流程：外形 → 名字与属性 → 出生地。',run:()=>c?A.enterCharacter(i):A.createCharacter(i)
    })).concat({label:'删除角色',meta:'需要完整名字确认',danger:true,run:()=>{
      A.ui.screen='world';A.ui.stack=[{type:'characterBridge',index:0}];A.push('deleteList',{modal:true,cancelRun:()=>{A.ui.screen='characters';A.ui.stack=[{type:'characters',index:0}];A.emit();}});
    }},{label:'返回登录',run:A.openLogin})};
    const c=A.ui.create;
    const steps=['选择外形','名字与属性','选择出生地'];
    if(c.step===0)return {title:'创造你的旅人',subtitle:'1 / 3 · 选择外形',steps,activeStep:0,fields:[
      {name:'body',label:'外形',type:'select',options:['小矮子','赛亚人','辫子男孩','酷哥','熊皮男','大个','小矮妹','熊皮妹','帽子妹','短发夹妹','手套女','辣妹'],value:c.body},
      {name:'color',label:'配色',type:'select',options:['红色','绿色','黄色','灰色'],value:c.color}],submitLabel:'下一步',cancel:true,submit:A.submitCreate};
    if(c.step===1)return {title:'名字与属性',subtitle:'2 / 3 · 能力总计 20 点；属性总计 10 点，地火、水风互斥',steps,activeStep:1,fields:[
      {name:'name',label:'角色名字',value:c.name,maxLength:16},...['体力','腕力','耐久力','速度','地','水','火','风'].map(k=>({name:k,label:k,type:'number',value:c[k],min:0,max:['地','水','火','风'].includes(k)?10:20}))],submitLabel:'下一步',cancel:true,submit:A.submitCreate};
    return {title:'选择出生地',subtitle:'3 / 3 · 确认后创建角色',steps,activeStep:2,fields:[{name:'home',label:'出生地',type:'select',value:c.home,options:A.data.places.filter(p=>!p.wild).map(p=>({value:p.id,label:p.name}))}],text:`${c.name} · ${c.body} · ${c.color}`,submitLabel:'创建角色',cancel:true,submit:A.submitCreate};
  };
})();
