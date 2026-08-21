#!/usr/bin/env bash
# ===== 构建发布产物（本地执行，必须在 master 分支）=====
# 生产只发 master：本脚本校验当前分支，非 master 直接拒绝。
# 产出 deploy/release/steady-<日期>-<短SHA>/ 内含三件套：
#   install.sh              安装脚本（VM 唯一入口）
#   config.tar.gz           配置包（compose / nginx.conf / init.sql / config.yaml / .env.example / backup 脚本）
#   steady-images.tar.gz    业务镜像包（collector / quant-engine / backend / frontend）
# 传到 VM：scp -r 该目录 → cd 进去 → ./install.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ---- 0. 分支校验：生产只发 master ----
BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$BRANCH" != "master" ]; then
  echo "❌ 生产发布必须在 master 分支（当前: $BRANCH）。"
  echo "   开发在 dev 分支；发布时：git checkout master && git merge dev"
  exit 1
fi
SHA=$(git rev-parse --short HEAD)
DATE=$(date +%Y%m%d)
VER="$DATE-$SHA"
RELDIR="$REPO_ROOT/deploy/release/steady-$VER"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$RELDIR"

echo "==> 1/4 构建镜像（master $SHA）"
cd "$REPO_ROOT/deploy"
docker compose -f docker-compose.yml build
for svc in collector quant-engine backend frontend; do
  docker tag "steady/$svc:latest" "steady/$svc:g$SHA"
done

echo "==> 2/4 打包镜像 -> steady-images.tar.gz"
docker save \
  steady/collector:g$SHA steady/collector:latest \
  steady/quant-engine:g$SHA steady/quant-engine:latest \
  steady/backend:g$SHA steady/backend:latest \
  steady/frontend:g$SHA steady/frontend:latest \
  | gzip > "$RELDIR/steady-images.tar.gz"

echo "==> 3/4 组装配置包 -> config.tar.gz"
# 目录结构对齐 run compose 的相对路径：解压到同一目录即可被容器挂载
mkdir -p "$WORK/nginx" "$WORK/postgres" "$WORK/configs" "$WORK/scripts"
cp "$REPO_ROOT/deploy/docker-compose.run.yml" "$WORK/"
cp "$REPO_ROOT/deploy/.env.example" "$WORK/"
cp "$REPO_ROOT/deploy/nginx/nginx.conf" "$WORK/nginx/"
cp "$REPO_ROOT/deploy/postgres/init.sql" "$WORK/postgres/"
cp "$REPO_ROOT/backend/configs/config.yaml" "$WORK/configs/"
cp "$REPO_ROOT/scripts/backup-db.sh" "$WORK/scripts/"
chmod +x "$WORK/scripts/backup-db.sh"
tar -czf "$RELDIR/config.tar.gz" -C "$WORK" .

echo "==> 4/4 安装脚本"
cp "$REPO_ROOT/scripts/install.sh" "$RELDIR/install.sh"
chmod +x "$RELDIR/install.sh"

echo ""
echo "✅ 发布产物：$RELDIR"
du -h "$RELDIR"/* | sed 's/^/   /'
echo ""
echo "传到 VM（整目录）："
echo "  scp -r $RELDIR <user>@<vm-ip>:~/"
echo "VM 上执行："
echo "  cd ~/steady-$VER && ./install.sh"
