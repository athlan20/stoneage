"""任务脚本里的 NOW=n 条件（STAGE-219，对照 gmsv/npc/npc_exchangeman.c 的 NPC_EventFreeIfCheck）。

原版 NPC_EventFreeIfCheck 逐个比较标记名，只认 LV、ITEM、ENDEV、NOWEV、SP、TIME、IMAGE、reITEM 等，NOW 不在其中，
所以 NOW=n 恒不成立（返回 FALSE）。EVENT 的候选组里 & 是与，一项为假整组不成立，含 NOW=n 的组因此永远不成立。
这里把这样的组整组丢掉，其余组按原顺序保留。

丢掉的组要在解析 EVENT 之前做：DelItem:EVDEL 和 DelPet:EVDEL 按“命中的第几组”取道具、宠物，原版的序号是原文里的位置，
这里按丢掉之后的序号，两边（条件和 EVDEL 的分组）用同一份 EVENT，序号才对得上（见 npcevent.py 的 convert_block）。
"""

import re

# 只认标记名恰为 NOW、比较符 =、值是整数的项。NOW!= 等别的写法不在这里处理，仍按不支持。
NOW_RE = re.compile(r"^NOW=\d+$")


def drop_now_groups(value: str) -> tuple[str, list[str]]:
    """EVENT 的值：返回 (丢掉含 NOW=n 的组之后的值，被丢掉的组原文)。剩下的组按原顺序，逗号连接。
    全部组都被丢掉时返回空串（这个块在原版里永远选不中，由调用方省略）。"""
    kept, dropped = [], []
    for alt in value.strip().split(","):
        if any(NOW_RE.match(term.strip()) for term in alt.split("&")):
            dropped.append(alt.strip())
        else:
            kept.append(alt.strip())
    return ",".join(kept), dropped
