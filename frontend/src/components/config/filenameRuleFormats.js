export const filenameRuleFormats = [
  {
    tag: '屏蔽词',
    color: '#ef4444',
    bg: 'rgba(239,68,68,0.1)',
    desc: '直接写词，从文件名中删除',
    syntax: '关键词',
    examples: [
      { rule: '国语配音', effect: '「某剧 国语配音 S01E01.mkv」→「某剧 S01E01.mkv」' },
      { rule: '港剧', effect: '「港剧 名侦探 01.mp4」→「名侦探 01.mp4」' },
    ],
  },
  {
    tag: '替换词',
    color: '#f59e0b',
    bg: 'rgba(245,158,11,0.1)',
    desc: '把旧词替换成新词，支持正则表达式',
    syntax: '旧词 => 新词',
    examples: [
      { rule: '港剧 => ', effect: '替换为空 = 屏蔽' },
      { rule: 'OVA => SP', effect: '「OVA 01」→「SP 01」' },
      { rule: '第(\\d+)话 => E\\1', effect: '「第12话」→「E12」（正则捕获组）' },
    ],
  },
  {
    tag: '集偏移',
    color: '#3b82f6',
    bg: 'rgba(59,130,246,0.1)',
    desc: '用前后缀夹住集号数字，修正集号偏差',
    syntax: '前缀 <> 后缀 >> EP+偏移量',
    examples: [
      { rule: '第 <> 话 >> EP+0', effect: '「第12话」→ E12（无偏移）' },
      { rule: '第 <> 集 >> EP-1', effect: '「第2集」→ E01（集号减1）' },
      { rule: 'Ep <> End >> EP+12', effect: '「Ep01End」→ E13（集号加12）' },
    ],
  },
  {
    tag: '组合',
    color: '#8b5cf6',
    bg: 'rgba(139,92,246,0.1)',
    desc: '先替换再偏移，用 && 连接两段规则',
    syntax: '旧词 => 新词 && 前缀 <> 后缀 >> EP+偏移量',
    examples: [
      { rule: 'OVA => SP && SP <> . >> EP+100', effect: '「OVA 01.」→ SP，集号+100' },
    ],
  },
]
