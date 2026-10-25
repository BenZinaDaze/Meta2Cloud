#!/bin/sh
# plex-sse-capture.sh —— 抓取 Plex SSE 实时通知原始报文（每行带到达时间戳），
# 用于对照真实事件形态排查问题（勿凭猜想改 SSE 相关代码，先抓包）。
# 用法: plex-sse-capture.sh <输出文件> [最大秒数，默认 1800]
# 凭证读取 config/config.yaml（plex.base_url）与 config/plex-auth.json（token）。
set -eu
OUT=${1:?用法: $0 <输出文件> [最大秒数]}
MAXT=${2:-1800}
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
BASE=$(python3 - "$ROOT/config/config.yaml" <<'PY'
import re, sys
t = open(sys.argv[1]).read()
m = re.search(r'^plex:\n(?:[ \t]+.*\n)*?[ \t]+base_url:\s*(\S+)', t, re.M)
print(m.group(1) if m else '')
PY
)
TOKEN=$(python3 - "$ROOT/config/plex-auth.json" <<'PY'
import json, sys
print(json.load(open(sys.argv[1])).get('auth_token', ''))
PY
)
[ -n "$BASE" ] || { echo "config.yaml 未找到 plex.base_url" >&2; exit 1; }
[ -n "$TOKEN" ] || { echo "plex-auth.json 无 token（未完成 PIN 配对）" >&2; exit 1; }
echo "# capture start $(date '+%F %T') base=$BASE max=${MAXT}s" >> "$OUT"
# -N 关缓冲逐行吐；到达时间精确到毫秒，便于与 app log 的分钟级时间对照
curl -sN --max-time "$MAXT" \
  -H "Accept: text/event-stream" \
  -H "X-Plex-Token: $TOKEN" \
  "$BASE/:/eventsource/notifications?filters=playing" \
| perl -MTime::HiRes=time -ne '
  BEGIN { $| = 1 }  # 逐行落盘：中途 kill 不丢已到达事件
  my $t = time; my @l = localtime($t);
  printf "%02d:%02d:%02d.%03d %s", $l[2], $l[1], $l[0], int(1000 * ($t - int($t))), $_;
' >> "$OUT"
echo "# capture end   $(date '+%F %T')" >> "$OUT"
