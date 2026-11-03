"""冒烟测试的终端模拟器（STAGE-5，见 tools/smoke-pty.py 和 docs/smoke.md）：把客户端输出的终端控制序列
还原成屏幕文字，宽字符（中文）占两列。只实现客户端用到的那一小部分。"""
import codecs
import unicodedata

ROWS, COLS = 40, 120


class Screen:
    """只实现客户端用到的那一小部分终端控制序列：光标定位、清屏、清行、移动光标和样式（忽略）。"""

    def __init__(self):
        self.grid = [[" "] * COLS for _ in range(ROWS)]
        self.row = self.col = 0
        self.state = "text"
        self.params = ""
        self.dec = codecs.getincrementaldecoder("utf-8")("replace")

    def feed(self, data: bytes):
        for ch in self.dec.decode(data):
            self.char(ch)

    def char(self, ch):
        if self.state == "esc":
            self.state = "csi" if ch == "[" else "text"
            self.params = ""
            return
        if self.state == "csi":
            if ch.isdigit() or ch in ";?<=>":
                self.params += ch
            else:
                self.csi(ch)
                self.state = "text"
            return
        if ch == "\x1b":
            self.state = "esc"
        elif ch == "\r":
            self.col = 0
        elif ch == "\n":
            self.row = min(self.row + 1, ROWS - 1)
        elif ch >= " ":
            self.put(ch)

    def put(self, ch):
        w = 2 if unicodedata.east_asian_width(ch) in ("W", "F") else 1
        if unicodedata.combining(ch):
            return
        if self.col + w > COLS:
            return
        row, col = self.grid[self.row], self.col

        def is_wide(c):
            return c != "" and unicodedata.east_asian_width(c) in ("W", "F")

        # 覆盖宽字符的一半时，留下的另一半要清成空格（真实终端就是这样显示的）。
        if row[col] == "" and col > 0 and row[col - 1] != "":
            row[col - 1] = " "           # 覆盖的是宽字符的右半，左半变空格
        if col + 1 < COLS and is_wide(row[col]) and row[col + 1] == "" and w == 1:
            row[col + 1] = " "           # 覆盖的是宽字符的左半，右半变空格
        if w == 2 and col + 2 < COLS and is_wide(row[col + 1]) and row[col + 2] == "":
            row[col + 2] = " "           # 宽字符盖住了另一个宽字符的左半，那个字符的右半变空格
        row[col] = ch
        if w == 2:
            row[col + 1] = ""
        self.col += w

    def csi(self, final):
        nums = [int(p) if p else 0 for p in self.params.lstrip("?").split(";")] if self.params else []
        n = nums[0] if nums and nums[0] else 1
        if final in "Hf":
            self.row = min(max((nums[0] if nums and nums[0] else 1) - 1, 0), ROWS - 1)
            self.col = min(max((nums[1] if len(nums) > 1 and nums[1] else 1) - 1, 0), COLS - 1)
        elif final == "J":
            mode = nums[0] if nums else 0
            if mode == 2 or mode == 3:
                self.grid = [[" "] * COLS for _ in range(ROWS)]
            elif mode == 0:
                for c in range(self.col, COLS):
                    self.grid[self.row][c] = " "
                for r in range(self.row + 1, ROWS):
                    self.grid[r] = [" "] * COLS
        elif final == "K":
            for c in range(self.col, COLS):
                self.grid[self.row][c] = " "
        elif final == "A":
            self.row = max(self.row - n, 0)
        elif final == "B":
            self.row = min(self.row + n, ROWS - 1)
        elif final == "C":
            self.col = min(self.col + n, COLS - 1)
        elif final == "D":
            self.col = max(self.col - n, 0)
        elif final == "G":
            self.col = min(max(n - 1, 0), COLS - 1)
        # m（样式）、h/l（模式）等其他序列不影响文字内容，忽略

    def text(self):
        return "\n".join("".join(row).rstrip() for row in self.grid)
