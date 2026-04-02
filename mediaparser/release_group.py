"""
ReleaseGroupsMatcher —— 识别字幕组/发布组。
来源：MoviePilot app/core/meta/releasegroup.py
改动：移除数据库依赖，改为通过构造函数传入自定义组列表。
"""
import regex as re
from typing import List, Optional


class ReleaseGroupsMatcher:
    """识别制作组、字幕组"""

    # 内置组（正则片段）
    RELEASE_GROUPS: dict = {
        "0ff": ["FF(?:(?:A|WE)B|CD|E(?:DU|B)|TV)"],
        "audiences": ["Audies", "AD(?:Audio|E(?:book|)|Music|Web)"],
        "beitai": ["BeiTai"],
        "btschool": ["Bts(?:CHOOL|HD|PAD|TV)", "Zone"],
        "carpt": ["CarPT"],
        "chdbits": ["CHD(?:Bits|PAD|(?:|HK)TV|WEB|)", "StBOX", "OneHD", "Lee", "xiaopie"],
        "eastgame": ["(?:(?:iNT|(?:HALFC|Mini(?:S|H|FH)D))-|)TLF"],
        "gainbound": ["(?:DG|GBWE)B"],
        "hares": ["Hares(?:(?:M|T)V|Web|)"],
        "hdarea": ["HDA(?:pad|rea|TV)", "EPiC"],
        "hdchina": ["HDC(?:hina|TV|)", "k9611", "tudou", "iHD"],
        "hddolby": ["D(?:ream|BTV)", "(?:HD|QHstudI)o"],
        "hdfans": ["beAst(?:TV|)"],
        "hdhome": ["HDH(?:ome|Pad|TV|WEB|)"],
        "hdpt": ["HDPT(?:Web|)"],
        "hdsky": ["HDS(?:ky|TV|Pad|WEB|)", "AQLJ"],
        "hdzone": ["HDZ(?:one|)"],
        "hhanclub": ["HHWEB"],
        "htpt": ["HTPT"],
        "keepfrds": ["FRDS", "Yumi", "cXcY"],
        "lemonhd": ["L(?:eague(?:(?:C|H)D|(?:M|T)V|NF|WEB)|HD)", "i18n", "CiNT"],
        "mteam": ["MTeam(?:TV|)", "MPAD", "MWeb"],
        "ourbits": ["Our(?:Bits|TV)", "FLTTH", "Ao", "PbK", "MGs", "iLove(?:HD|TV)"],
        "panda": ["Panda", "AilMWeb"],
        "piggo": ["PiGo(?:NF|(?:H|WE)B)"],
        "pterclub": ["PTer(?:DIY|Game|(?:M|T)V|WEB|)"],
        "pthome": ["PTH(?:Audio|eBook|music|ome|tv|WEB|)"],
        "ptsbao": ["PTsbao", "OPS", "F(?:Fans(?:AIeNcE|BD|D(?:VD|IY)|TV|WEB)|HDMv)", "SGXT"],
        "putao": ["PuTao"],
        "springsunday": ["CMCT(?:V|)"],
        "sharkpt": ["Shark(?:WEB|DIY|TV|MV|)"],
        "tjupt": ["TJUPT"],
        "totheglory": ["TTG", "WiKi", "NGB", "DoA", "(?:ARi|ExRE)N"],
        "others": [
            "B(?:MDru|eyondHD|TN)", "C(?:fandora|trlhd|MRG)", "DON", "EVO", "FLUX",
            "HONE(?:yG|)", "N(?:oGroup|T(?:b|G))", "PandaMoon", "SMURF",
            "T(?:EPES|aengoo|rollHD )",
        ],
        "anime": [
            "ANi", "HYSUB", "KTXP", "LoliHouse", "MCE", "Nekomoe kissaten",
            "SweetSub", "MingY", "(?:Lilith|NC)-Raws",
            "织梦字幕组", "枫叶字幕组", "猎户手抄部", "喵萌奶茶屋", "漫猫字幕社",
            "霜庭云花Sub", "北宇治字幕组", "氢气烤肉架", "云歌字幕组", "萌樱字幕组",
            "极影字幕社", "悠哈璃羽字幕社", "❀拨雪寻春❀",
            "沸羊羊(?:制作|字幕组)", "(?:桜|樱)都字幕组",
        ],
        "forge": ["FROG(?:E|Web|)"],
        "ubits": ["UB(?:its|WEB|TV)"],
        "sakurato": ["Sakurato"],
    }

    def __init__(self, custom_groups: Optional[List[str]] = None):
        """
        :param custom_groups: 额外的字幕组正则片段列表
        """
        all_groups: List[str] = []
        for site_groups in self.RELEASE_GROUPS.values():
            all_groups.extend(site_groups)
        if custom_groups:
            all_groups.extend(g for g in custom_groups if g)
        self._pattern_str = "|".join(all_groups)

    def match(self, title: str) -> str:
        """
        :param title: 资源标题或文件名
        :return: 匹配到的字幕组，多个用 @ 分隔
        """
        if not title or not self._pattern_str:
            return ""
        title = f"{title} "
        groups_re = re.compile(
            r"(?<=[-@\[￡【&])(?:(?:%s))(?=$|[@.\s\]\[】&])" % self._pattern_str, re.I
        )
        unique: list[str] = []
        for item in re.findall(groups_re, title):
            s = item[0] if isinstance(item, tuple) else item
            if s not in unique:
                unique.append(s)
        return "@".join(unique)
