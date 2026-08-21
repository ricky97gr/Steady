# Steady 部署运维手册

> 目标机器：ESXi 上的 Linux VM（2C7G/100G）。**VM 只运行、不编译**——
> 镜像在开发机构建后以 tar 包搬运，版本以 git 提交 SHA 标识，支持秒级回滚。

## 架构约定

| 载体 | 内容 | 升级方式 |
|---|---|---|
| **镜像**（`steady/*`） | 业务代码（collector/quant-engine/backend/frontend） | 本地构建 → 搬运 → `load` |
| **仓库文件**（git clone） | compose / nginx.conf / init.sql / config.yaml / 脚本 | VM 上 `git pull` |
| **`deploy/.env`** | 仅数据库凭据（唯一密钥，chmod 600） | 手动改，不入库 |
| **`app_config` 表** | 飞书/Tushare 等业务配置 | 设置页改，以库为准 |
| **`postgres_data` 卷** | 数据库数据 | 升级不动，永不删 |

## 首次部署（VM）

```bash
# 1. Docker 引擎 + compose 插件
curl -fsSL https://get.docker.com | sh
sudo systemctl enable --now docker

# 2. 拉仓库（只取文件，不编译）
git clone https://github.com/ricky97gr/Steady.git
cd Steady/deploy

# 3. 数据库凭据（页面配置不了：页面本身要连库，密码须跳出这条链路）
cp .env.example .env
vim .env    # DB_PASSWORD = $(openssl rand -hex 24)
chmod 600 .env

# 4. 从开发机拿镜像包（见下），然后发布
~/Steady/scripts/deploy-images.sh ~/steady-XXXX.tar.gz
```

## 发布新版本（升级）

**开发机（每次发版）**：
```bash
./scripts/build-images.sh          # 构建 + 打 g<SHA> 标签 + 打包 deploy/images/steady-<日期>-<SHA>.tar.gz
scp deploy/images/steady-*.tar.gz  <user>@<vm-ip>:~
```

**VM（应用）**：
```bash
cd Steady
./scripts/deploy-images.sh ~/steady-<日期>-<SHA>.tar.gz
```

要点：
- 只拉镜像包不够时，若 compose/nginx.conf/config.yaml 也有改动，先 `git pull`。
- 版本 = 镜像包文件名里的 `日期-SHA`；`g<SHA>` 就是镜像标签，对应确切代码提交，可追溯。

## 回滚（一键回到上一版）

```bash
# 保留上一份 tar 包即可（VM 上 deploy/images/ 建议留最近 3 份）
./scripts/deploy-images.sh ~/steady-<上一个-日期>-<上一个-SHA>.tar.gz
```

原理：`docker load` 会把 `:latest` 指回旧镜像 ID，compose `up -d` 检测到变化即重建。
数据库在命名卷里，**回滚不影响任何数据**。

## 查看当前运行版本

```bash
cat deploy/RELEASES.md                                   # 发布历史（脚本自动追加）
docker compose -f docker-compose.run.yml ps              # 容器与镜像
docker image inspect steady/backend --format '{{join .RepoTags ","}}'   # 含 g<SHA> 标签
```

## 备份与恢复

```bash
# 每天 02:30 全库 gzip 备份到 deploy/backup/，留 30 份
crontab -e    # 30 2 * * * /path/Steady/scripts/backup-db.sh

# 恢复
gunzip -c deploy/backup/quant_system_YYYYMMDD_HHMMSS.sql.gz | \
  docker exec -i quant-postgres psql -U quant -d quant_system
```

## 安全基线

- 仅 `nginx:80` 对外；backend/postgres/frontend 全部绑定 `127.0.0.1`（API 无鉴权，绝不直接暴露）。
- `.env` 只存数据库凭据，chmod 600，gitignored；业务配置全走设置页 → `app_config`。
- VM 上开启防火墙仅放行 80/22（及 SSH 来源白名单）。
