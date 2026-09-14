#!/bin/bash

echo ""
echo "🚀 启动：前端 + Go后端"
echo ""

# 检查依赖
echo "🔍 检查环境依赖..."

if ! command -v node &> /dev/null; then
    echo "❌ Node.js 未安装! 请安装Node.js 18+: https://nodejs.org/"
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "❌ Go 未安装! 请安装Go 1.22+: https://golang.org/dl/"
    exit 1
fi

echo "✅ Node.js: $(node --version)"
echo "✅ Go: $(go version | cut -d' ' -f3)"
echo ""

# 启动Go后端（后台）
echo "🎯 启动Go后端..."
cd server
if [ -z "$GOPATH" ]; then
    export PATH=$PATH:$(go env GOPATH)/bin
fi
air --build.cmd "echo" --build.full_bin "CGO_ENABLED=1 go run main.go" &
BACKEND_PID=$!
cd ..
echo ""

# 检查服务状态
echo "🔍 检查服务状态..."

# 检查Go后端
i=0
while true; do
    sleep 1
    if curl -s http://localhost:8080/health > /dev/null; then
        echo "✅ Go后端启动成功 (PID: $BACKEND_PID)"
        break
    fi
    i=$((i+1))
    if [ $i -ge 30 ]; then
        echo "❌ Go后端启动失败"
        kill $BACKEND_PID 2>/dev/null
        exit 1
    fi
    if [ $i -eq 5 ]; then
        echo "🔍 检查Go后端状态... ${i}s"
    fi
done

# 启动前端（阻塞进程）
echo "🎯 启动前端开发服务器..."
next dev --turbopack

# 前端停止后，清理所有进程
echo ""
echo "🛑 清理所有后台进程..."
kill $BACKEND_PID 2>/dev/null
echo "✅ 开发环境已完全停止" 