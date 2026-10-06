#!/usr/bin/env bash
# 一条本地命令启动游泳计时复核系统（后端 Go/Gin + 前端 Vue + SQLite）。
# 用法：./run.sh            生产模式（前端构建后由 Go 同端口托管，访问 http://localhost:8080）
#       ./run.sh dev        开发模式（Vite 热更新 http://localhost:5173，API 代理到 8080）
#       ./run.sh reset      清空本地数据库后以生产模式启动
set -euo pipefail
cd "$(dirname "$0")"

MODE="${1:-serve}"

# 1) 定位 Go 工具链：优先 PATH，其次 ~/sdk/go
if ! command -v go >/dev/null 2>&1; then
  if [ -x "$HOME/sdk/go/bin/go" ]; then
    export PATH="$HOME/sdk/go/bin:$PATH"
  else
    echo "未找到 Go，请先安装 Go 1.22+（或解压到 ~/sdk/go）" >&2
    exit 1
  fi
fi
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GO111MODULE=on

mkdir -p backend/data

reset_db() {
  rm -f backend/data/timing.db
  echo "[run] 已清空本地数据库"
}

start_backend() {
  echo "[run] 启动后端  http://localhost:8080"
  ( cd backend && go run ./cmd/server -addr :8080 -db data/timing.db )
}

if [ "$MODE" = "reset" ]; then
  reset_db
  MODE="serve"
fi

if [ "$MODE" = "dev" ]; then
  # 开发模式：后端 + Vite dev server
  if ! command -v node >/dev/null 2>&1; then
    echo "未找到 Node.js，开发模式需要 Node 18+；生产模式可直接运行 ./run.sh" >&2
    exit 1
  fi
  [ -d web/node_modules ] || ( echo "[run] 安装前端依赖…" && cd web && npm install --registry=https://registry.npmmirror.com )
  start_backend &
  BACK_PID=$!
  trap 'kill $BACK_PID 2>/dev/null || true' EXIT
  echo "[run] 启动前端  http://localhost:5173"
  ( cd web && npm run dev )
else
  # 生产模式：构建前端，由 Go 在 :8080 同端口托管 web/dist
  if ! command -v node >/dev/null 2>&1; then
    echo "未找到 Node.js；将尝试使用已构建的 web/dist（若不存在请先在有 Node 的环境构建）" >&2
  else
    if [ ! -d web/node_modules ]; then
      echo "[run] 安装前端依赖…"
      ( cd web && npm install --registry=https://registry.npmmirror.com )
    fi
    echo "[run] 构建前端…"
    ( cd web && npm run build )
  fi
  start_backend
fi
