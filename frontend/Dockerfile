# 允许构建时替换镜像地址，默认使用官方 Node 镜像
ARG NODE_IMAGE=node:22-alpine

# 允许构建时替换镜像地址，默认使用官方 Nginx 镜像
ARG NGINX_IMAGE=nginx:alpine

# 第一阶段：构建 Vue 项目
FROM ${NODE_IMAGE} AS builder

# 设置构建目录
WORKDIR /app

# 先复制依赖清单，利用 Docker 缓存
COPY package.json package-lock.json ./

# 按照 package-lock.json 安装依赖
RUN npm ci

# 复制前端源码
COPY . .

# 执行生产构建，生成 dist
RUN npm run build


# 第二阶段：运行 Nginx
FROM ${NGINX_IMAGE}

# 删除 Nginx 默认页面
RUN rm -rf /usr/share/nginx/html/*

# 复制前端构建结果
COPY --from=builder /app/dist /usr/share/nginx/html

# 复制自定义 Nginx 配置
COPY nginx.conf /etc/nginx/conf.d/default.conf

# 声明监听 80 端口
EXPOSE 80

# 前台运行 Nginx
CMD ["nginx", "-g", "daemon off;"]