# Steady 部署运维手册

> 目标：ESXi 上的 Linux VM（2C7G/100G）。**VM 只运行、不编译**。
> 分支模型：**生产只发 `master`**，本地开发在 `dev`。
> 交付物：一个发布目录（安装脚本 + 配置包 + 镜像包），VM 上只跑一个脚本。

## 分支与发布模型

```text
dev 分支（日常开发）  ──发布时──▶  master 分支（生产唯一来源）  ──build-release.sh──▶  release 目录  ──scp──▶  VM
```

- `master` 是生产唯一来源，`build-release.sh` 校验当前分支，非 master 拒绝打包。
- 发版动作：`git checkout master && git merge dev && git push`，然后在本地 `./scripts/build-release.sh`。

## 交付物（三件套）

`build-release.sh`（本地、master 分支）产出 `deploy/release/steady-<日期>-<短SHA>/`：

| 文件 | 作用 |
|---|---|
| `install.sh` | VM 唯一入口，自动：检测既有部署 → 解压配置 → 复用/生成 .env → 加载镜像 → `compose up -d` |
| `config.tar.gz` | 配置包：compose / nginx.conf / init.sql / config.yaml / .env.example / backup 脚本 |
| `steady-images.tar.gz` | 4 个业务镜像（collector / quant-engine / backend / frontend） |

postgres / nginx 来自 Docker Hub，无需打包。

## 首次部署（VM）

**0. 系统准备（一次性）：**
```bash
# SSH 密钥登录已配好；可选：sshd 只监听内网 IP
# echo 'ListenAddress <内网IP>' | sudo tee -a /etc/ssh/sshd_config && sudo sshd -t && sudo systemctl restart ssh
```

**1. 本机构建发布产物（须在 master 分支）：**
```bash
git checkout master && git merge dev && git push
./scripts/build-release.sh
scp -r deploy/release/steady-<日期>-<SHA> <user>@<vm-ip>:~/
```

**2. VM 安装（唯一命令）：**
```bash
cd ~/steady-<日期>-<SHA>
./install.sh
```

install.sh 自动完成：检测既有部署 → 解压配置包 → 生成 `.env`（随机 24 字节数据库强密码并打印，**请抄录**）→ 加载镜像 → `compose up -d` → 写入 `RELEASES.md` → 打印容器状态。

首次部署的 compose 项目名**固定为 `steady`**（数据卷 `steady_postgres_data`）。之后升级会自动复用既有项目名，数据卷永远不变。

## 升级

```bash
# 本地（master 分支）：改代码合入 master → 重新打包
./scripts/build-release.sh && scp -r deploy/release/steady-* <user>@<vm-ip>:~/
# VM：进新目录再跑一次即可
cd ~/steady-<新版本> && ./install.sh
```

升级自动安全：install.sh 检测到运行中 `quant-postgres` 后，**复用其 compose 项目名与数据库密码**（`.env`）——数据卷/网络/容器名全继承，只重建镜像或配置有变化的容器。**不会**新建空数据卷或改密码。

> ⚠️ 历史教训（2026-08-22 修复）：早期 install.sh 在**新目录**运行，因目录里无 `.env` 会生成新密码，且 compose 按目录名建新项目 → 新建空数据卷，升级数据"看起来丢了"。现已改为自动复用。

## 回滚

```bash
# 只要旧版本目录还在，进去跑 install.sh 就回到旧版（旧镜像 load 会覆盖 :latest）
cd ~/steady-<旧版本> && ./install.sh
```

数据库在命名卷里，**回滚/升级都不动数据**（卷名=项目名 + `_postgres_data`，全新安装即 `steady_postgres_data`）。

## 查看当前版本

```bash
cat RELEASES.md                                    # 发布历史
docker compose ls                                  # 列出 compose 项目（升级后项目名为既有项目）
docker compose -p <项目名> -f docker-compose.run.yml ps   # 容器状态（项目名见 RELEASES.md / install.sh 输出）
```

## 备份与恢复

```bash
# 每天 02:30 全库 gzip 备份到 backup/，留 30 份（backup-db.sh 在 config 包内已就位）
crontab -e    # 30 2 * * * /home/<user>/steady-*/scripts/backup-db.sh

# 恢复
gunzip -c backup/quant_system_YYYYMMDD_HHMMSS.sql.gz | \
  docker exec -i quant-postgres psql -U quant -d quant_system
```

## 安全基线

- **仅内网**：路由器不要给 22/80 做端口转发；SSH 密钥登录、禁密码/root。
- Docker 发布的端口不走 ufw（走 FORWARD 链），nginx:80 的边界靠 compose 绑定：默认 `80:80`（NAT 内网下即仅内网可达）；要更严格可在 `.env` 设 `HOST_LAN_IP=<内网IP>` 绑定单 IP，或保持 `127.0.0.1` + SSH 隧道。
- backend / postgres / frontend 全部绑定 `127.0.0.1`（API 无鉴权，绝不直接暴露）。
- `.env` 只存数据库凭据，chmod 600；业务配置（飞书/Tushare 等）全走设置页 → `app_config` 表。
