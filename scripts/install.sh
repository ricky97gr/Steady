#!/usr/bin/env bash
# ===== Steady 安装/升级脚本（VM 唯一入口，幂等）=====
# 与 config.tar.gz、steady-images*.tar.gz 放在同一发布目录里运行：./install.sh
# 自动完成：解压配置包 → 生成 .env（首次）→ 加载镜像 → compose up -d → 记录版本
# 升级：把新版三件套目录拷进来再跑一次；.env（数据库密码）已存在则保留。
# 生产 = master 分支产物（版本号在目录名里）。
set -euo pipefail

APP_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$APP_DIR"

command -v docker >/dev/null 2>&1 || { echo "❌ 未安装 docker"; exit 1; }
docker compose version >/dev/null 2>&1 || { echo "❌ 未安装 docker compose 插件"; exit 1; }

echo "==> 1/4 解压配置包"
[ -f config.tar.gz ] || { echo "❌ 缺少 config.tar.gz（须与 install.sh 同目录）"; exit 1; }
tar -xzf config.tar.gz
echo "   ✔ compose / nginx.conf / config.yaml / init.sql 已就位"

echo "==> 2/4 数据库凭据"
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

echo "==> 3/4 加载镜像（取最新一份 steady-images*.tar.gz）"
IMAGES=$(ls -1t steady-images*.tar.gz 2>/dev/null | head -1)
[ -n "$IMAGES" ] || { echo "❌ 缺少 steady-images*.tar.gz（须与 install.sh 同目录）"; exit 1; }
echo "   -> $IMAGES"
gunzip -c "$IMAGES" | docker load

echo "==> 4/4 启动服务（run-only compose，VM 不编译）"
docker compose -f docker-compose.run.yml up -d

mkdir -p logs
{ echo "- $(date '+%Y-%m-%d %H:%M') | 发布: $(basename "$APP_DIR") | 镜像包: $IMAGES"; } >> RELEASES.md

echo ""
echo "✔ 部署完成。当前版本：$(basename "$APP_DIR")"
echo "   RELEASES.md 已记录；容器状态："
docker compose -f docker-compose.run.yml ps
echo ""
echo "查看日志：docker compose -f docker-compose.run.yml logs -f"
