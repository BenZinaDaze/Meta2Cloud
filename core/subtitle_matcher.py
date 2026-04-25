"""
subtitle_matcher.py —— 字幕文件匹配器

功能：
  1. 从字幕文件名提取语言标签
  2. 将字幕与视频文件进行匹配
  3. 生成字幕目标文件名
"""
from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path
from typing import List, Optional, Tuple

from storage.base import CloudFile


@dataclass
class SubtitleMatch:
    """字幕匹配结果"""
    subtitle_file: CloudFile          # 原字幕文件
    language_tags: List[str]          # 语言标签列表 ['chs', 'forced']
    target_name: str                  # 目标字幕文件名
    confidence: float                 # 匹配置信度 0.0-1.0


class SubtitleMatcher:
    """字幕匹配器"""

    # 集数匹配模式（用于判断是否为单集字幕）
    EPISODE_PATTERN = re.compile(
        r'[Ss](\d{1,2})[Ee](\d{1,3})|'   # S01E01
        r'[Ee](\d{1,3})|'                 # E01
        r'第(\d{1,3})[集话]',             # 第01集、第01话
        re.IGNORECASE
    )

    # 语言代码映射（标准化为 ISO-639-1 + 地区代码）
    LANGUAGE_CODE_MAP = {
        # 简体中文 -> zh-CN
        'chs': 'zh-CN', 'sc': 'zh-CN', 'simplified': 'zh-CN', 'zh-cn': 'zh-CN',
        '简体': 'zh-CN', '简中': 'zh-CN', 'gb': 'zh-CN',
        # 繁体中文 -> zh-TW
        'cht': 'zh-TW', 'tc': 'zh-TW', 'traditional': 'zh-TW', 'zh-tw': 'zh-TW',
        '繁体': 'zh-TW', '繁中': 'zh-TW', 'big5': 'zh-TW',
        # 中文通用 -> zh
        'zh': 'zh', 'chinese': 'zh', '中文': 'zh', '中字': 'zh', '中英': 'zh',
        # 英文
        'en': 'en', 'eng': 'en', 'english': 'en', '英': 'en', '英字': 'en',
        # 日文
        'ja': 'ja', 'jp': 'ja', 'japanese': 'ja', '日': 'ja', '日字': 'ja',
        # 韩文
        'ko': 'ko', 'kr': 'ko', 'korean': 'ko', '韩': 'ko', '韩字': 'ko',
    }

    # 匹配末尾语言标签的正则模式
    # 只从文件名末尾开始匹配，避免误匹配标题中的语言词
    SUFFIX_LANG_PATTERN = re.compile(
        r'[\._-]('
        r'chs|cht|sc|tc|gb|big5|simplified|traditional|'
        r'zh-cn|zh-tw|zh|chinese|简体|繁体|简中|繁中|中文|中字|中英|'
        r'en|eng|english|英|英字|'
        r'ja|jp|japanese|日|日字|'
        r'ko|kr|korean|韩|韩字|'
        r'default|forced|sdh|hi|commentary'
        r')$',
        re.IGNORECASE
    )

    # 匹配括号形式语言标签的正则模式（可出现在任意位置）
    BRACKET_LANG_PATTERN = re.compile(
        r'[\[\【]('
        r'chs|cht|sc|tc|gb|big5|simplified|traditional|'
        r'zh-cn|zh-tw|zh|chinese|简体|繁体|简中|繁中|中文|中字|中英|'
        r'en|eng|english|英|英字|'
        r'ja|jp|japanese|日|日字|'
        r'ko|kr|korean|韩|韩字|'
        r'default|forced|sdh|hi|commentary'
        r')[\]\】]',
        re.IGNORECASE
    )

    def parse_subtitle_tags(self, subtitle_stem: str) -> Tuple[str, List[str]]:
        """
        解析字幕文件名，提取语言标签

        Args:
            subtitle_stem: 字幕文件名（不含扩展名）

        Returns:
            (清理后的视频名, 语言标签列表)

        Example:
            "Movie.2024.chs.forced" -> ("Movie.2024", ["zh-CN", "forced"])
            "Movie.2024[chs]" -> ("Movie.2024", ["zh-CN"])
            "The.English.Game.S01E01.eng" -> ("The.English.Game.S01E01", ["en"])
        """
        tags: List[str] = []
        clean_name = subtitle_stem

        # 先处理括号形式的语言标签
        while True:
            match = self.BRACKET_LANG_PATTERN.search(clean_name)
            if not match:
                break
            tag = match.group(1).lower()
            normalized = self.LANGUAGE_CODE_MAP.get(tag, tag)
            if normalized not in tags:
                tags.append(normalized)
            # 移除匹配到的标签
            clean_name = clean_name[:match.start()] + clean_name[match.end():]

        # 从末尾开始处理后缀形式的语言标签（收集后反转以保持原始顺序）
        suffix_tags: List[str] = []
        while True:
            match = self.SUFFIX_LANG_PATTERN.search(clean_name)
            if not match:
                break
            tag = match.group(1).lower()
            normalized = self.LANGUAGE_CODE_MAP.get(tag, tag)
            if normalized not in suffix_tags:
                suffix_tags.append(normalized)
            # 移除匹配到的标签
            clean_name = clean_name[:match.start()]

        # 反转后缀标签顺序，并添加到 tags 前面
        suffix_tags.reverse()
        tags = suffix_tags + tags

        # 清理多余的分隔符和空括号
        clean_name = re.sub(r'[\._-]+', '.', clean_name).strip('.')
        clean_name = re.sub(r'\[\s*\]', '', clean_name)
        clean_name = re.sub(r'【\s*】', '', clean_name)
        clean_name = clean_name.strip('.')

        return clean_name, tags

    def match_subtitle_to_video(
        self,
        subtitle: CloudFile,
        video: CloudFile,
    ) -> Optional[SubtitleMatch]:
        """
        将字幕匹配到视频

        匹配策略：
        1. 精确匹配：字幕去掉标签后与视频stem完全相同
        2. 字幕名是视频名的前缀：字幕省略了视频的发行标签
        3. 视频名是字幕名的前缀
        4. 包含匹配
        """
        subtitle_stem = Path(subtitle.name).stem
        video_stem = Path(video.name).stem
        video_stem_guess, language_tags = self.parse_subtitle_tags(subtitle_stem)

        # 精确匹配
        if video_stem.lower() == video_stem_guess.lower():
            target_name = self.build_target_name(video.name, subtitle.extension, language_tags)
            return SubtitleMatch(
                subtitle_file=subtitle,
                language_tags=language_tags,
                target_name=target_name,
                confidence=1.0
            )

        # 字幕名是视频名的前缀（字幕省略了发行标签）
        # 例如：video=Movie.2024.1080p.WEB-DL.mkv, subtitle=Movie.2024.chs.srt
        # 这种匹配非常可靠，给予较高的基础置信度
        # 但需要排除季级字幕匹配到单集视频的情况
        if video_stem.lower().startswith(video_stem_guess.lower() + '.') or \
           video_stem.lower().startswith(video_stem_guess.lower() + '_') or \
           video_stem.lower().startswith(video_stem_guess.lower() + '-'):
            # 检查是否为季级字幕匹配单集视频的情况
            # 如果视频有集数但字幕没有集数，则不匹配
            video_has_episode = bool(self.EPISODE_PATTERN.search(video_stem))
            subtitle_has_episode = bool(self.EPISODE_PATTERN.search(video_stem_guess))
            if video_has_episode and not subtitle_has_episode:
                # 季级字幕不应匹配到单集视频
                pass
            else:
                # 前缀匹配非常可靠，基础置信度 0.7，加上长度比例的加成
                confidence = 0.7 + 0.3 * (len(video_stem_guess) / len(video_stem))
                target_name = self.build_target_name(video.name, subtitle.extension, language_tags)
                return SubtitleMatch(
                    subtitle_file=subtitle,
                    language_tags=language_tags,
                    target_name=target_name,
                    confidence=confidence
                )

        # 视频名是字幕名的前缀（如 Video.mkv -> Video.chs.srt）
        if subtitle_stem.lower().startswith(video_stem.lower() + '.') or \
           subtitle_stem.lower().startswith(video_stem.lower() + '_') or \
           subtitle_stem.lower().startswith(video_stem.lower() + '-'):
            confidence = len(video_stem) / len(subtitle_stem)
            target_name = self.build_target_name(video.name, subtitle.extension, language_tags)
            return SubtitleMatch(
                subtitle_file=subtitle,
                language_tags=language_tags,
                target_name=target_name,
                confidence=confidence
            )

        # 包含匹配：视频名包含在清理后的字幕名中
        # 但需要确保是边界匹配，避免 S01E01 匹配到 S01E010
        if video_stem.lower() in video_stem_guess.lower():
            # 检查匹配位置是否在边界上
            idx = video_stem_guess.lower().find(video_stem.lower())
            before_ok = idx == 0 or video_stem_guess[idx - 1] in '._-'
            after_ok = idx + len(video_stem) == len(video_stem_guess) or video_stem_guess[idx + len(video_stem)] in '._-'
            if before_ok and after_ok:
                confidence = len(video_stem) / len(video_stem_guess)
                if confidence >= 0.5:
                    target_name = self.build_target_name(video.name, subtitle.extension, language_tags)
                    return SubtitleMatch(
                        subtitle_file=subtitle,
                        language_tags=language_tags,
                        target_name=target_name,
                        confidence=confidence
                    )

        return None

    def find_subtitles_for_video(
        self,
        video: CloudFile,
        all_subtitles: List[CloudFile],
    ) -> List[SubtitleMatch]:
        """
        找出某个视频的所有匹配字幕

        注意：
        - 会对目标文件名去重，保留置信度最高的匹配
        """
        # 先收集所有匹配
        all_matches: List[SubtitleMatch] = []
        for subtitle in all_subtitles:
            match = self.match_subtitle_to_video(subtitle, video)
            if match and match.confidence >= 0.5:
                all_matches.append(match)

        # 按目标名分组，保留每组中置信度最高的
        best_by_target: dict[str, SubtitleMatch] = {}
        for match in all_matches:
            existing = best_by_target.get(match.target_name)
            if existing is None or match.confidence > existing.confidence:
                best_by_target[match.target_name] = match

        matches = list(best_by_target.values())

        # 按语言优先级排序
        priority = {'zh-CN': 0, 'zh-TW': 1, 'zh': 2, 'en': 3, 'ja': 4, 'ko': 5}
        matches.sort(
            key=lambda m: priority.get(m.language_tags[0] if m.language_tags else '', 99)
        )

        return matches

    def build_target_name(
        self,
        video_name: str,
        subtitle_ext: str,
        language_tags: List[str],
    ) -> str:
        """
        构建字幕目标文件名

        规则：视频名 + 语言标签 + 字幕扩展名
        """
        video_stem = Path(video_name).stem

        if language_tags:
            # 过滤掉非语言标签用于命名（forced, sdh 等保留）
            lang_part = '.'.join(t for t in language_tags if t != 'default')
            if lang_part:
                return f"{video_stem}.{lang_part}{subtitle_ext}"

        return f"{video_stem}{subtitle_ext}"
