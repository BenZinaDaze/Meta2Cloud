"""
Tokens —— 将标题字符串按分隔符拆为 token 列表，与 MoviePilot 保持一致。
"""
import re


class Tokens:
    def __init__(self, text: str):
        self._index = 0
        self._tokens: list[str] = []
        self._load(text)

    def _load(self, text: str):
        parts = re.split(r"\.|\ s+|\(|\)|\[|]|-|【|】|/|～|;|&|\||#|_|「|」|~", text)
        self._tokens = [p for p in parts if p]

    def cur(self):
        if self._index >= len(self._tokens):
            return None
        return self._tokens[self._index]

    def get_next(self):
        token = self.cur()
        if token:
            self._index += 1
        return token

    def peek(self):
        idx = self._index + 1
        if idx >= len(self._tokens):
            return None
        return self._tokens[idx]

    @property
    def tokens(self):
        return self._tokens
