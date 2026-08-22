#!/usr/bin/env bash
# ===== Steady 安装/升级脚本（VM 唯一入口，幂等）=====
# 与 config.tar.gz、steady-images*.tar.gz 放在同一发布目录里运行：./install.sh
# 自动完成：检测既有部署 → 解压配置包 → 复用/生成 .env → 加载镜像 → compose up -d → 记录版本
# 升级：把新版三件套目录拷进来再跑一次。自动复用运行中 postgres 的「compose 项目名 + 数据库密码」，
#       数据卷/网络/容器名全继承，绝不新建空数据卷或改密码（防止升级"丢数据"）。
# 生产 = master 分支产物（版本号在目录名里）。
set -euo pipefail

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$APP_DIR"

command -v docker >/dev/null 2>&1 || { echo "❌ 未安装 docker"; exit 1; }
docker compose version >/dev/null 2>&1 || { echo "❌ 未安装 docker compose 插件"; exit 1; }

# ---- 0. 检测既有部署（升级）：复用运行中 postgres 的项目名与 .env ----
# 升级若不复用，compose 项目名=新目录名 → 新建空数据卷 + 新密码，数据"看起来丢了"。
UPGRADE=0
EXISTING_PROJECT=""
OLD_DIR=""
if docker ps -aq --filter "name=quant-postgres" | grep -q .; then
  UPGRADE=1
  EXISTING_PROJECT=$(docker inspect quant-postgres --format '{{ index .Config.Labels "com.docker.compose.project" }}' 2>/dev/null || true)
  OLD_DIR=$(docker inspect quant-postgres --format '{{ index .Config.Labels "com.docker.compose.project.working_dir" }}' 2>/dev/null || true)
  [ -n "$EXISTING_PROJECT" ] || { echo "❌ 检测到运行中 quant-postgres 但取不到项目名，中止（避免误新建数据卷）"; exit 1; }
  echo "==> 0/5 检测到既有部署：$EXISTING_PROJECT（升级模式，复用数据卷）"
else
  echo "==> 0/5 未检测到既有部署：全新安装"
fi

# 项目名：升级复用既有项目；全新安装固定为 steady（此后升级始终继承同一数据卷）
PROJECT="steady"
if [ "$UPGRADE" = 1 ]; then
  PROJECT="$EXISTING_PROJECT"
fi

echo "==> 1/5 解压配置包"
[ -f config.tar.gz ] || { echo "❌ 缺少 config.tar.gz（须与 install.sh 同目录）"; exit 1; }
tar -xzf config.tar.gz
echo "   ✔ compose / nginx.conf / config.yaml / init.sql 已就位"

echo "==> 2/5 数据库凭据"
if [ "$UPGRADE" = 1 ]; then
  # 升级：复用既有数据库密码。优先整份拷旧目录 .env（保留用户可能自加的键）；
  # 旧目录已删则从运行中 postgres 环境变量恢复。
  if [ -n "$OLD_DIR" ] && [ -f "$OLD_DIR/.env" ]; then
    cp "$OLD_DIR/.env" .env
    echo "   ✔ 复用 $OLD_DIR/.env（数据库密码不变）"
  else
    DB_PASSWORD=$(docker inspect quant-postgres --format '{{ range .Config.Env }}{{ println . }}{{ end }}' | sed -n 's/^POSTGRES_PASSWORD=//p' | tr -d '\r\n')
    [ -n "$DB_PASSWORD" ] || { echo "❌ 无法读取运行中 postgres 的密码，拒绝覆盖 .env，请手动处理"; exit 1; }
    cp .env.example .env
    sed -i "s/^DB_PASSWORD=.*/DB_PASSWORD=$DB_PASSWORD/" .env
    echo "   ✔ 已从运行中 postgres 恢复 .env（数据库密码不变）"
  fi
  chmod 600 .env
else
  # 全新安装：仅当无 .env 时生成随机强密码
  if [ ! -f .env ]; then
    cp .env.example .env
    PW=$(openssl rand -hex 24)
    sed -i "s/change_me_strong_password/$PW/" .env
    chmod 600 .env
    echo "   ✔ 已生成 .env，数据库密码：$PW"
    echo "     请立即抄录（升级/重装不会重置，丢了就只能改库或删卷重建）"
  else
    echo "   ✔ .env 已存在，保留（数据库密码不变）"
  fi
fi

echo "==> 3/5 加载镜像（取最新一份 steady-images*.tar.gz）"
IMAGES=$(ls -1t steady-images*.tar.gz 2>/dev/null | head -1)
[ -n "$IMAGES" ] || { echo "❌ 缺少 steady-images*.tar.gz（须与 install.sh 同目录）"; exit 1; }
echo "   -> $IMAGES"
gunzip -c "$IMAGES" | docker load

echo "==> 4/5 启动服务（run-only compose，VM 不编译）"
if [ "$UPGRADE" = 1 ]; then
  echo "   -> 复用既有项目 $PROJECT（数据卷/网络/容器名继承，只重建有变化的容器）"
else
  echo "   -> 全新部署，项目名固定为 steady（后续升级始终继承同一数据卷）"
fi
docker compose -p "$PROJECT" -f docker-compose.run.yml up -d

mkdir -p logs
{ echo "- $(date '+%Y-%m-%d %H:%M') | 发布: $(basename "$APP_DIR") | 镜像包: $IMAGES"; } >> RELEASES.md

echo ""
echo "✔ 部署完成。当前版本：$(basename "$APP_DIR")"
echo "   RELEASES.md 已记录；容器状态："
docker compose -p "$PROJECT" -f docker-compose.run.yml ps
echo ""
echo "查看日志：docker compose -p $PROJECT -f docker-compose.run.yml logs -f"
