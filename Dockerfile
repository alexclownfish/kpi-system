# 后端构建阶段
FROM golang:1.23.10-bookworm AS backend-builder

# 设置工作目录
WORKDIR /app

# 复制后端相关文件和目录
COPY server/ ./

# 安装依赖
RUN go mod tidy

# 设置环境变量
ENV GIN_MODE=release

# 构建后端
RUN CGO_ENABLED=1 go build -o server main.go

# =============================================================
# =============================================================
# =============================================================

# 前端构建阶段
FROM node:slim AS builder

# 设置工作目录
WORKDIR /web

# 复制前端相关文件和目录
COPY . ./

# 安装依赖
RUN npm install

# 设置环境变量
ENV NEXT_PUBLIC_BASE_PATH=/apps/kpi
ENV NEXT_PUBLIC_API_URL=/apps/kpi/api
ENV NEXT_OUTPUT_MODE=standalone

# 构建项目
RUN npm run build

# =============================================================
# =============================================================
# =============================================================

# 生产阶段
FROM node:slim AS production

# 设置工作目录
WORKDIR /web

# 复制后端构建产物
COPY --from=backend-builder /app/server /web/server

# 复制前端构建产物
COPY --from=builder /web/.next/standalone/ /web/
COPY --from=builder /web/.next/static/ /web/.next/static/
COPY --from=builder /web/public/ /web/public/

# 创建启动脚本
RUN cat <<'EOF' > /web/start.sh
#!/bin/bash

# 启动后端
./server &

# 启动前端
node /web/server.js
EOF

# 设置权限
RUN chmod +x /web/start.sh

# 设置环境变量
ENV SYSTEM_MODE=integrated

# 启动项目
CMD ["/web/start.sh"]