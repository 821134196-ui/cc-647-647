#!/usr/bin/env bash
# 一条命令启动游泳计时复核系统（后端 Go+Gin + 前端 Vue+Vite）
# 用法:
#   ./start.sh          开发模式：后端 :8080，前端 :5173（已代理 /api）
#   ./start.sh --prod   生产模式：构建前端后由 Go 单端口托管，访问 http://localhost:8080
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND="$ROOT/backend"
FRONTEND="$ROOT/frontend"

# -------- 定位 Go --------
find_go() {
  if command -v go >/dev/null 2>&1; then
    command -v go
    return
  fi
  for cand in /usr/local/go/bin/go "$HOME/go-sdk/bin/go" "$HOME/go/bin/go" /opt/go/bin/go; do
    if [ -x "$cand" ]; then echo "$cand"; return; fi
  done
  echo ""
}
GO_BIN="$(find_go)"
if [ -z "$GO_BIN" ]; then
  echo "未找到 Go。请安装 Go 1.23+，或解压到 \$HOME/go-sdk。" >&2
  exit 1
fi
export GOTOOLCHAIN=local
echo "使用 Go: $("$GO_BIN" version)"

# -------- 依赖与构建 --------
echo ">> 检查后端依赖..."
(cd "$BACKEND" && "$GO_BIN" mod download)

echo ">> 检查前端依赖..."
if [ ! -d "$FRONTEND/node_modules" ]; then
  (cd "$FRONTEND" && npm install)
fi

MODE="${1:-dev}"

if [ "$MODE" = "--prod" ]; then
  echo ">> 构建前端..."
  (cd "$FRONTEND" && npm run build)
  echo ">> 启动单端口服务: http://localhost:8080"
  cd "$BACKEND"
  export SWIM_FRONTEND_DIR="$FRONTEND/dist"
  exec "$GO_BIN" run .
fi

# -------- 开发模式：同时拉起前后端 --------
cleanup() {
  echo ""
  echo ">> 正在停止前后端..."
  [ -n "${BACK_PID:-}" ] && kill "$BACK_PID" 2>/dev/null || true
  [ -n "${FRONT_PID:-}" ] && kill "$FRONT_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

echo ">> 启动后端 (Gin :8080)..."
( cd "$BACKEND" && "$GO_BIN" run . ) &
BACK_PID=$!

echo ">> 启动前端 (Vite :5173)..."
( cd "$FRONTEND" && npm run dev -- --host ) &
FRONT_PID=$!

echo ""
echo "============================================================"
echo " 系统已启动："
echo "   前端页面 : http://localhost:5173"
echo "   后端接口 : http://localhost:8080/api"
echo " 演示账号（密码均为 123456）："
echo "   chief 总裁判 / judge 裁判 / clerk 录入员 / device 电子计时台"
echo " 按 Ctrl+C 停止全部服务"
echo "============================================================"

wait -n "$BACK_PID" "$FRONT_PID"
