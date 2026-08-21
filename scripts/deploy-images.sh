#!/usr/bin/env bash
# ===== VM 上发布镜像包（只运行，不编译）=====
# 用法：./scripts/deploy-images.sh <镜像包.tar.gz>
#   tar 包由 scripts/build-images.sh 在开发机产出，scp 过来即可。
# 流程：docker load（带入 :latest 与 g<SHA> 两个标签）→ run-only compose up -d。
#   compose 检测到 :latest 的镜像 ID 变了 → 自动重建对应容器；数据在命名卷里，不受影响。
# 回滚：拿上一个 tar 包再执行一次本脚本（load 会覆盖 :latest 指回旧镜像 ID）。
set -euo pipefail

if [ $# -lt 1 ]; then
  echo "用法: $0 <镜像包.tar.gz>"
  echo "例:   $0 ~/steady-20260821-a1b2c3d.tar.gz"
  exit 1
fi
TARBALL="$1"
[ -f "$TARBALL" ] || { echo "找不到文件: $TARBALL"; exit 1; }

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT/deploy"

echo "==> 1/3 加载镜像包 $(basename "$TARBALL")"
gunzip -c "$TARBALL" | docker load

echo "==> 2/3 应用发布（run-only compose，VM 永不编译）"
docker compose -f docker-compose.run.yml up -d

echo "==> 3/3 记录发布日志"
RELEASES=RELEASES.md
touch "$RELEASES"
IMGS=$(docker compose -f docker-compose.run.yml ps --format '{{.Service}}={{.Image}}' | tr '\n' ' ')
echo "- $(date '+%Y-%m-%d %H:%M') | 镜像包: $(basename "$TARBALL") | $IMGS" >> "$RELEASES"

echo ""
echo "✅ 部署完成，当前版本已记入 deploy/RELEASES.md"
echo "   查看：docker compose -f docker-compose.run.yml ps"
