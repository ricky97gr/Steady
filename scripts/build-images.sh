#!/usr/bin/env bash
# ===== 本地构建镜像并打包（在开发机执行，VM 不编译）=====
# 产物：deploy/images/steady-<日期>-<git短SHA>.tar.gz
#   - 4 个业务镜像各带两个标签：steady/<svc>:latest 和 steady/<svc>:g<SHA>
#   - postgres / nginx 来自 Docker Hub，不打包，VM 直接拉
# 发布：把 tar 包 scp 到 VM，然后执行 scripts/deploy-images.sh <包名>
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT/deploy"

SHA=$(git -C "$REPO_ROOT" rev-parse --short HEAD)
DATE=$(date +%Y%m%d)
OUT_DIR="$REPO_ROOT/deploy/images"
mkdir -p "$OUT_DIR"

echo "==> 1/3 构建镜像（compose 显式 image: steady/<svc>:latest）"
docker compose -f docker-compose.yml build

echo "==> 2/3 打版本标签 g$SHA（镜像 ↔ 精确代码提交，可追溯）"
for svc in collector quant-engine backend frontend; do
  docker tag "steady/$svc:latest" "steady/$svc:g$SHA"
done

TARBALL="$OUT_DIR/steady-$DATE-$SHA.tar.gz"
echo "==> 3/3 打包 -> $TARBALL"
docker save \
  steady/collector:g$SHA steady/collector:latest \
  steady/quant-engine:g$SHA steady/quant-engine:latest \
  steady/backend:g$SHA steady/backend:latest \
  steady/frontend:g$SHA steady/frontend:latest \
  | gzip > "$TARBALL"

# 本地只留最近 3 份，避免积压吃磁盘
ls -1t "$OUT_DIR"/steady-*.tar.gz 2>/dev/null | tail -n +4 | xargs -r rm -f

echo ""
echo "✅ 完成：$TARBALL（$(du -h "$TARBALL" | cut -f1)）"
echo ""
echo "下一步（传到 VM）："
echo "  scp $TARBALL <user>@<vm-ip>:~"
echo "  # VM 上执行："
echo "  cd Steady && ./scripts/deploy-images.sh ~/$(basename "$TARBALL")"
