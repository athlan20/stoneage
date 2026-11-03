/* 本目录只模拟交互，不连接服务器，不复刻正式游戏数值。 */
window.SA = (() => {
  const places = [
    {id:1,name:'萨姆吉尔村',island:'萨伊那斯岛',desc:'石屋之间飘来炊烟，村民正准备今天的集市。',npcs:['elder','merchant','healer','save'],wild:false,x:61,y:36},
    {id:2,name:'玛丽娜丝渔村',island:'萨伊那斯岛',desc:'海风吹过木制码头，远处传来渔民的歌声。',npcs:['fisher','merchant','healer'],wild:false,x:27,y:47},
    {id:3,name:'加加村',island:'加鲁卡岛',desc:'高大的椰树围着村落，猎人们在村口交换见闻。',npcs:['hunter','merchant','healer'],wild:false,x:72,y:70},
    {id:4,name:'卡鲁它那村',island:'加鲁卡岛',desc:'村长的石屋旁，有一位等待暗号的旅人。',npcs:['traveler','merchant','healer'],wild:false,x:49,y:79},
    {id:5,name:'萨姆吉尔近郊',island:'萨伊那斯岛',desc:'草丛窸窣作响，乌力和野猪正在不远处觅食。',npcs:['hunter'],wild:true,x:67,y:28},
    {id:6,name:'加加村近郊',island:'加鲁卡岛',desc:'这里是捕捉宠物与练习战斗的好地方。',npcs:['hunter'],wild:true,x:78,y:62}
  ];
  const items = [
    {id:'hat',name:'皮帽',kind:'equip',part:0,price:60,effect:'防御 +2',desc:'柔软的兽皮帽。适合刚开始冒险的旅人。'},
    {id:'axe',name:'小斧头',kind:'equip',part:2,price:120,effect:'攻击 +9 · 防御 -3 · 敏捷 -3',desc:'打磨过的石斧。握柄上缠着结实的藤条。'},
    {id:'armor',name:'皮甲',kind:'equip',part:1,price:180,effect:'防御 +6',desc:'用厚实兽皮制成，能抵挡一些伤害。'},
    {id:'herb',name:'回复药',kind:'heal',price:20,power:60,effect:'回复 60 HP',desc:'带着青草香气的药。可对角色或宠物使用。'},
    {id:'mana',name:'气力药',kind:'mana',price:35,power:25,effect:'回复 25 MP',desc:'恢复精神的果汁，只能给角色使用。'},
    {id:'stone',name:'漂亮的石头',kind:'material',price:8,effect:'任务物品',desc:'溪水把它磨得很光滑。村长似乎在寻找这种石头。'},
    {id:'ring',name:'骨制手环',kind:'equip',part:3,price:90,effect:'攻击 +2',desc:'用小块兽骨串成的手环。'}
  ];
  const npcs = {
    elder:{name:'村长',type:'任务',text:'年轻的旅人，村子的储备不太够了。你愿意替我带来一颗漂亮的石头吗？'},
    merchant:{name:'特产品贩卖员',type:'商店',text:'欢迎光临！看看今天带来的货物吧。'},
    healer:{name:'治疗师',type:'治疗',text:'在外面受伤了吗？我可以为你和宠物恢复体力与气力，费用是 30 石币。'},
    save:{name:'储存点',type:'存档',text:'这里能记住你回家的方向。要把这里设为回城地点吗？'},
    fisher:{name:'老渔夫',type:'对话',text:'潮水一天两涨两落。听说卡鲁它那有个旅人，在寻找知道「海风」暗号的人。'},
    hunter:{name:'猎人阿布',type:'战斗',text:'想练练身手？我可以陪你来一场友好的较量，也能送你去附近的练级点。'},
    traveler:{name:'神秘旅人',type:'暗号',text:'你知道我们的暗号吗？说对了，我会送你一份小礼物。'}
  };
  const wiki = [
    {id:'wuli',category:'宠物',name:'乌力',text:'温顺而结实的初期伙伴。适合与新旅人一起成长。\n\n技能：攻击、防御、撕咬。\n获得：野外捕捉。\n提示：出战宠物会与你一起进入战斗。'},
    {id:'boar',category:'宠物',name:'野猪',text:'生活在近郊草地的生物，动作敏捷。\n\n捕捉前先降低其体力，成功率会更高。'},
    {id:'herb',category:'物品',name:'回复药',text:'恢复角色或宠物的 HP。\n\n背包中：选择物品 → 使用 → 选择目标。\n战斗中：道具 → 回复药 → 选择目标。'},
    {id:'first',category:'任务',name:'村长的委托',text:'1. 在萨姆吉尔村与村长交谈。\n2. 准备一颗漂亮的石头。\n3. 再次对话并交付。\n\n奖励：100 石币，50 经验。\n当前示例背包中已放入任务所需物品。'},
    {id:'secret',category:'任务',name:'海风的暗号',text:'老渔夫知道旅人的秘密。\n\n前往卡鲁它那村，与神秘旅人交谈，输入「海风」。\n首次完成可获得回复药。'},
    {id:'travel',category:'地图',name:'村庄与传送',text:'本游戏无需手动行走。\n\n通过地点列表传送，或在对话中前往目的地。\n世界地图帮助你了解各村的位置。'},
    {id:'battle',category:'玩法',name:'战斗与控制权',text:'每回合先下人物指令，再下宠物指令。\n\n方向键选择，Enter 确认，Esc 返回。\nagent 操控时可以浏览，收回控制权后才能操作。\n自动战斗与 agent 操控是两种不同功能。'}
  ];
  function fresh() {
    return {version:1,account:'traveler',characters:[{name:'小明',body:'辫子男孩',color:'绿色',home:1},null],slot:0,place:1,home:1,
      gold:680,hp:125,maxHp:180,mp:45,maxMp:80,level:12,exp:35,skillPoints:5,
      stats:{体力:15,腕力:12,耐久力:10,速度:11},elements:{地:8,水:2,火:0,风:0},
      inventory:[{slot:0,id:'hat',count:1},{slot:5,id:'axe',count:1},{slot:7,id:'herb',count:5},{slot:8,id:'mana',count:2},{slot:10,id:'stone',count:1}],
      pets:[{slot:0,name:'乌力',species:'乌力',hp:65,maxHp:100,level:8,state:'battle',loyalty:92},{slot:2,name:'小红',species:'红暴龙',hp:110,maxHp:110,level:10,state:'standby',loyalty:85}],
      quests:[{id:'first',name:'村长的委托',state:'未接取',desc:'村长正在寻找一颗漂亮的石头。',npc:'elder',place:1},{id:'secret',name:'海风的暗号',state:'进行中',desc:'找到神秘旅人，告诉他暗号。',npc:'traveler',place:4}],
      tokens:[],controller:'player',agentOnline:true,auto:false,encounter:false,settings:{unicode:true,sound:false,speed:1},
      messages:[{kind:'系统',text:'欢迎来到石器时代。用方向键与菜单开始冒险。'}]};
  }
  return {data:{places,items,npcs,wiki},fresh};
})();
