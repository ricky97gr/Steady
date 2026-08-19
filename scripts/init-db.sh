#!/usr/bin/env bash
# 初始化数据库（在 postgres 容器健康后执行）
set -euo pipefail

cd "$(dirname "$0")/../deploy"

# 从 .env 读取 DB 凭据
export $(grep -E '^(DB_USER|DB_PASSWORD|DB_NAME)=' .env | xargs)

docker exec -i quant-postgres psql -U "$DB_USER" -d "$DB_NAME" < postgres/init.sql

echo "✅ 数据库初始化完成: $DB_NAME"
