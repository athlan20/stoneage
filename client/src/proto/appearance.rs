//! 外形、颜色、出生地的显示名，取自 `docs/login.md`（原表在旧服务端 `char_data.c` 的 `MetamoList`）。

pub const BODY_COUNT: u8 = 12;
pub const COLOR_COUNT: u8 = 4;
pub const FACE_COUNT: u8 = 5;

pub const BODY_NAMES: [&str; BODY_COUNT as usize] = [
    "小矮子", "赛亚人", "辫子男孩", "酷哥", "熊皮男", "大个", "小矮妹", "熊皮妹", "帽子妹",
    "短发夹妹", "手套女", "辣妹",
];

/// 颜色序号在不同外形上对应不同的颜色名。
pub const COLOR_NAMES: [[&str; COLOR_COUNT as usize]; BODY_COUNT as usize] = [
    ["红", "绿", "黄", "灰"],
    ["灰", "红", "绿", "黄"],
    ["灰", "黄", "绿", "红"],
    ["红", "绿", "黄", "灰"],
    ["灰", "绿", "黄", "红"],
    ["红", "灰", "黄", "绿"],
    ["绿", "黄", "灰", "红"],
    ["绿", "红", "黄", "灰"],
    ["黄", "红", "绿", "灰"],
    ["灰", "黄", "红", "绿"],
    ["红", "灰", "绿", "黄"],
    ["黄", "绿", "红", "灰"],
];

pub fn body_name(body: u8) -> &'static str {
    BODY_NAMES.get(body as usize).copied().unwrap_or("?")
}

pub fn color_name(body: u8, color: u8) -> &'static str {
    COLOR_NAMES
        .get(body as usize)
        .and_then(|row| row.get(color as usize))
        .copied()
        .unwrap_or("?")
}

/// 服务端保存的身体图像编号：`100000 + 外形×20 + 颜色×5`。
pub fn body_image(body: u8, color: u8) -> u32 {
    100_000 + body as u32 * 20 + color as u32 * 5
}

pub struct Hometown {
    pub id: u8,
    pub name: &'static str,
    /// 村庄在世界地图上的位置（世界格坐标，见 `docs/map.md`）。
    pub world: (f64, f64),
}

pub const HOMETOWNS: [Hometown; 4] = [
    Hometown { id: 0, name: "萨姆吉尔", world: (852.0, 685.0) },
    Hometown { id: 1, name: "玛丽娜丝", world: (255.0, 861.0) },
    Hometown { id: 2, name: "加加村", world: (1257.0, 1071.0) },
    Hometown { id: 3, name: "卡鲁它那", world: (969.0, 1394.0) },
];
