#!/usr/bin/env bash
set -euo pipefail

PATCH_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="${1:-$(pwd)}"

cd "$REPO_DIR"

echo "[patch] applying ly_server mysql initialization patch to $REPO_DIR"

mkdir -p deploy/mysql internal/bootstrap/sql internal/bootstrap internal/config cmd/traffic-admin scripts

cp "$PATCH_DIR/deploy/mysql/ly_server_init.sql" deploy/mysql/ly_server_init.sql
cp "$PATCH_DIR/internal/bootstrap/sql/ly_server_init.sql" internal/bootstrap/sql/ly_server_init.sql
cp "$PATCH_DIR/internal/bootstrap/lyserver_mysql.go" internal/bootstrap/lyserver_mysql.go
cp "$PATCH_DIR/internal/config/config.go" internal/config/config.go
cp "$PATCH_DIR/cmd/traffic-admin/main.go" cmd/traffic-admin/main.go
cp "$PATCH_DIR/deploy/docker-compose.yml" deploy/docker-compose.yml
cp "$PATCH_DIR/scripts/reset-demo.sh" scripts/reset-demo.sh
cp "$PATCH_DIR/scripts/init-demo.sh" scripts/init-demo.sh
cp "$PATCH_DIR/scripts/test-original-api-compat.sh" scripts/test-original-api-compat.sh
cp "$PATCH_DIR/Dockerfile" Dockerfile
chmod +x scripts/reset-demo.sh scripts/init-demo.sh scripts/test-original-api-compat.sh

python3 - <<'PY'
from pathlib import Path

# Ensure go.mod has mysql driver dependency.
p = Path('go.mod')
s = p.read_text()
need = 'github.com/go-sql-driver/mysql v1.8.1'
if need not in s:
    if 'require (' in s:
        s = s.replace('require (', 'require (\n\t' + need, 1)
    else:
        s += '\nrequire ' + need + '\n'
    p.write_text(s)

# Ensure .env.example has ly_server mysql settings.
p = Path('.env.example')
if p.exists():
    s = p.read_text()
else:
    s = ''
block = '''
# ly_server compatible MySQL database
LY_DB_BACKEND=mysql
LY_DATABASE_DSN=traffic:traffic@tcp(127.0.0.1:3306)/server?charset=utf8mb4&parseTime=true&loc=Local&multiStatements=true
'''
if 'LY_DATABASE_DSN' not in s:
    p.write_text(s.rstrip() + '\n\n' + block.lstrip())
PY

if command -v gofmt >/dev/null 2>&1; then
  gofmt -w cmd/traffic-admin/main.go internal/config/config.go internal/bootstrap/lyserver_mysql.go
fi

echo "[patch] done"
echo "[patch] next: go mod tidy && go mod vendor && docker compose -f deploy/docker-compose.yml config"
