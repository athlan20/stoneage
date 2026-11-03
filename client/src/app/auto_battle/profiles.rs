//! 方案的新建、删除、保存（STAGE-260 起从 `edit.rs` 拆出，控制 500 行上限）：编辑器按键调用这里，
//! 保存会写回方案文件并让托管读到新设置。

use super::AutoProfile;
use crate::app::App;

impl App {
    pub(super) fn profile_count(&self) -> usize {
        self.auto_store.as_ref().map_or(1, |s| s.profiles.profiles.len())
    }

    pub(super) fn add_profile(&mut self, name: &str) {
        if let Some(s) = self.auto_store.as_mut() {
            let mut p = AutoProfile::default();
            p.name = name.to_string();
            s.profiles.profiles.push(p);
            s.profiles.active = s.profiles.profiles.len() - 1;
        }
        self.apply_playback();
    }

    /// 删除当前方案，返回给玩家看的说明。
    pub(super) fn delete_profile(&mut self) -> String {
        let Some(s) = self.auto_store.as_mut() else {
            return "方案没加载，删不了".into();
        };
        if s.profiles.profiles.len() <= 1 {
            return "至少保留一个方案".into();
        }
        let i = s.profiles.active.min(s.profiles.profiles.len() - 1);
        let removed = s.profiles.profiles.remove(i);
        s.profiles.active = i.min(s.profiles.profiles.len() - 1);
        self.log.system(format!("已删除自动战斗方案：{}", removed.name));
        self.apply_playback();
        format!("已删除方案「{}」", removed.name)
    }

    /// 保存并应用：写回方案文件、让托管的下一回合就用新设置，并同步播放偏好。
    /// 草稿是调用方传进来的（按键处理时编辑器已经从 `WorldState` 里 take 出来了）。
    pub(super) fn save_auto_settings(&mut self, draft: AutoProfile) -> Result<(), String> {
        let mut err = None;
        if let Some(s) = self.auto_store.as_mut() {
            if s.profiles.profiles.is_empty() {
                s.profiles.profiles.push(draft.clone());
                s.profiles.active = 0;
            }
            let i = s.profiles.active.min(s.profiles.profiles.len() - 1);
            s.profiles.profiles[i] = draft;
            if let Err(e) = s.save() {
                err = Some(e);
            }
        }
        self.apply_playback();
        match err {
            None => {
                let line = format!("自动战斗设置已保存并应用：{}", self.auto_profile().summary());
                self.log.system(line);
                Ok(())
            }
            Some(e) => {
                self.log.error(format!("自动战斗设置没能保存：{e}"));
                Err(e)
            }
        }
    }

}
