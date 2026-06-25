#!/bin/bash
set -e
BACKUP_DIR="/var/www/myfibase/backups"
DATE=$(date +%Y-%m-%d_%H%M)
BACKUP_FILE="$BACKUP_DIR/myfibase_$DATE.tar.gz"
mkdir -p "$BACKUP_DIR"
docker exec myfibase_postgres pg_dump -U myfibase myfibase > /tmp/myfibase_db.sql
tar -czf "$BACKUP_FILE" \
  --exclude='*.log' \
  --exclude='node_modules' \
  --exclude='.next' \
  --exclude='backups' \
  --exclude='.git' \
  -C /var/www/myfibase . \
  -C /tmp myfibase_db.sql
rm -f /tmp/myfibase_db.sql
ls -t "$BACKUP_DIR"/myfibase_*.tar.gz | tail -n +8 | xargs -r rm --
SIZE=$(du -sh "$BACKUP_FILE" | cut -f1)
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Backup created: $BACKUP_FILE ($SIZE)"
