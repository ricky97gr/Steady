# ---- 构建阶段 ----
FROM node:22-alpine AS builder

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm config set registry https://registry.npmmirror.com && npm ci

COPY . .
RUN npm run build

# ---- 运行阶段：Nginx 托管静态文件 ----
FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
