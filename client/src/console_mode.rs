//! Windows 控制台的输入模式。
//!
//! 客户端不开鼠标，但控制台的输入模式可能被同一个窗口里先运行过的程序留成了
//! “VT 输入 + 鼠标输入”：之后每次点鼠标，控制台都把 `ESC[<0;41;25M` 之类的鼠标报告
//! 当成普通按键送进来，显示在底部输入行里。启动时清掉这两个位，退出时恢复原样；
//! 其他平台什么都不做。

pub use imp::{drop_vt_and_mouse_input, restore_input_mode};

#[cfg(windows)]
mod imp {
    use std::ffi::c_void;
    use std::sync::atomic::{AtomicU32, Ordering};

    const ENABLE_MOUSE_INPUT: u32 = 0x0010;
    const ENABLE_VIRTUAL_TERMINAL_INPUT: u32 = 0x0200;
    const STD_INPUT_HANDLE: u32 = -10i32 as u32;
    /// 没有保存过原模式。
    const NONE: u32 = u32::MAX;

    type Handle = *mut c_void;

    unsafe extern "system" {
        fn GetStdHandle(std_handle: u32) -> Handle;
        fn GetConsoleMode(console: Handle, mode: *mut u32) -> i32;
        fn SetConsoleMode(console: Handle, mode: u32) -> i32;
    }

    /// 进入前保存的原输入模式，退出时恢复。
    static SAVED_MODE: AtomicU32 = AtomicU32::new(NONE);

    /// 保存原输入模式，并清掉 VT 输入与鼠标输入位。要在 `enable_raw_mode` 之前调用，
    /// 这样保存的是进入前的模式，退出时恢复的是完整的原样。
    pub fn drop_vt_and_mouse_input() {
        if let Some(original) = get_mode() {
            SAVED_MODE.store(original, Ordering::Relaxed);
            set_mode(original & !(ENABLE_VIRTUAL_TERMINAL_INPUT | ENABLE_MOUSE_INPUT));
        }
    }

    /// 恢复 `drop_vt_and_mouse_input` 保存的原输入模式。重复调用无效。
    pub fn restore_input_mode() {
        let saved = SAVED_MODE.swap(NONE, Ordering::Relaxed);
        if saved != NONE {
            set_mode(saved);
        }
    }

    /// 标准输入的控制台句柄；不是控制台（重定向、管道）时返回 None。
    fn stdin_console() -> Option<Handle> {
        // SAFETY: 只取标准输入的句柄，不做别的。
        let handle = unsafe { GetStdHandle(STD_INPUT_HANDLE) };
        if handle.is_null() || handle as isize == -1 {
            return None;
        }
        Some(handle)
    }

    fn get_mode() -> Option<u32> {
        let handle = stdin_console()?;
        let mut mode = 0u32;
        // SAFETY: handle 是有效的控制台句柄，mode 是可写的本地变量。
        let ok = unsafe { GetConsoleMode(handle, &mut mode) };
        (ok != 0).then_some(mode)
    }

    fn set_mode(mode: u32) {
        if let Some(handle) = stdin_console() {
            // SAFETY: handle 是有效的控制台句柄；失败时保持原状，不影响启动。
            unsafe { SetConsoleMode(handle, mode) };
        }
    }
}

#[cfg(not(windows))]
mod imp {
    pub fn drop_vt_and_mouse_input() {}

    pub fn restore_input_mode() {}
}
