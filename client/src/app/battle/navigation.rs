//! STAGE-138：只恢复可浏览层，不恢复旧交易或目标。
use crossterm::event::{KeyCode,KeyEvent};
use crate::app::{App,Layer,WorldState};

impl WorldState {
    /// 战斗画面是不是占着整块界面：有战斗，且玩家没有在观察模式下把菜单盖上去（STAGE-182）。
    pub fn battle_front(&self) -> bool {
        self.battle.is_some() && !self.battle_veil
    }

    /// 观察模式的战斗里按 Esc：打开游戏菜单查看背包、宠物等页面，战斗留在后台。
    pub(crate) fn open_battle_veil(&mut self) {
        self.battle_veil = true;
        self.nav.reset();
        self.nav.push(Layer::Menu);
        self.input.clear();
    }

    /// 收起菜单回到战斗画面（菜单退回场景、`take` 收回控制权、Tab 打开战斗控制面板时）。
    pub(crate) fn close_battle_veil(&mut self) {
        if !self.battle_veil {
            return;
        }
        self.battle_veil = false;
        self.nav.reset();
        self.input.clear();
        self.bag_step = None;
        self.pet_step = None;
        self.char_step = None;
        self.party_step = None;
        self.scene_step = None;
        self.browse.step = None;
        self.browse.quest_visit = None;
        self.confirm = None;
        self.npc_window = None;
    }

    pub(crate) fn prepare_battle_navigation(&mut self) {
        if self.battle_veil {
            return; // 玩家正停在菜单页里，下一场战斗开始不把人拉回战斗画面（STAGE-182）
        }
        while self.nav.top().is_modal(){self.nav.pop();}
        self.battle_return=Some(self.nav.clone());
        self.input.clear();
        self.bag_step=None;self.pet_step=None;self.char_step=None;self.scene_step=None;
        self.browse.step=None;self.browse.quest_visit=None;self.confirm=None;self.npc_window=None;
    }
}

impl App {
    /// 菜单退回场景，收起盖在战斗上的菜单（STAGE-182）。战斗已经结束时（STAGE-284）直接收掉战斗，
    /// 不弹结果窗口，也不把人拉回战斗前的页面。
    pub(crate) fn leave_battle_veil(&mut self) {
        let Some(w) = self.world.as_mut() else {
            return;
        };
        w.close_battle_veil();
        if w.battle.as_ref().is_some_and(|b| b.result.is_some()) {
            w.battle_return = None;
            self.close_battle();
        }
    }

    pub(crate) fn battle_bar_enter(&mut self)->bool {
        if self.world.as_ref().is_none_or(|w|w.input.is_empty()){return true;}
        if self.world.as_ref().is_some_and(|w|!w.online && super::command::parse(w.input.text().trim()).is_some()){
            self.log.error("战斗快照已离线，请从系统菜单重新连接");return true;
        }
        let before=self.log.all().iter().filter(|m|m.kind==crate::app::Kind::Error).count();
        if !self.battle_enter(){return false;}
        if self.log.all().iter().filter(|m|m.kind==crate::app::Kind::Error).count()==before {
            if let Some(w)=self.world.as_mut(){w.nav.close_command();}
        }
        true
    }

    pub(crate) fn battle_open_bar(&mut self,key:KeyEvent)->bool {
        if key.code==KeyCode::Char('/') {
            if let Some(w)=self.world.as_mut(){w.nav.push(Layer::Command);}
            true
        } else {false}
    }
}
