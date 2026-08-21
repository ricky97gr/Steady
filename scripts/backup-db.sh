#!/usr/bin/env bash
# 备份数据库到 deploy/backup/ 目录（按日期命名，保留最近 30 份）
set -euo pipefail

cd "$(dirname "$0")/../deploy"

export $(grep -E '^(DB_USER|DB_NAME)=' .env | xargs)

BACKUP_DIR="backup"
mkdir -p "$BACKUP_DIR"

STAMP=$(date +%Y%m%d_%H%M%S)
# gzip 压缩：全库 4GB 时，不压缩每份 4GB、留 30 份会吃满磁盘；压缩后 ~0.5-1GB/份
docker exec quant-postgres pg_dump -U "$DB_USER" "$DB_NAME" | gzip > "$BACKUP_DIR/${DB_NAME}_${STAMP}.sql.gz"

# 清理 30 天前的备份
find "$BACKUP_DIR" -name "${DB_NAME}_*.sql.gz" -mtime +30 -delete

echo "✅ 备份完成: $BACKUP_DIR/${DB_NAME}_${STAMP}.sql"
