"""原子表 itematom.txt 的读取，合成相关的生成脚本共用（STAGE-205）。

原版 item_gen.c 的 ITEM_initAtom：每行 名字,魔法标志,编号（GBK）。原子的下标就是行序，
ITEM_getAtomIndexByName 返回的也是它；物品和宠物的原子名都靠它找下标。
“编号”列原版没有读取，这里照原样保留。
"""

from pathlib import Path


def load_atoms(path: Path) -> list[tuple[str, int, int]]:
    """返回 [(名字, 魔法标志, 编号)]，列表下标即原子下标。"""
    atoms = []
    for lineno, raw in enumerate(path.read_bytes().splitlines(), 1):
        line = raw.rstrip(b"\r")
        if lineno == 1:
            line = line.removeprefix(b"\xef\xbb\xbf")
        text = line.decode("gbk")
        if not text.strip() or text.lstrip().startswith("#"):
            continue
        fields = text.split(",")
        if len(fields) < 3:
            raise SystemExit(f"{path}:{lineno}: 列数不够（{len(fields)}），应为 名字,魔法标志,编号")
        name = fields[0].strip()
        if not name:
            raise SystemExit(f"{path}:{lineno}: 原子名为空")
        if any(name == a[0] for a in atoms):
            raise SystemExit(f"{path}:{lineno}: 原子名 {name} 重复")
        try:
            magic, num = int(fields[1].strip()), int(fields[2].strip())
        except ValueError as e:
            raise SystemExit(f"{path}:{lineno}: 数字列解析失败: {e}")
        atoms.append((name, magic, num))
    return atoms
