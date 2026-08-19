# Quant System — 个人 A 股量化研究与模拟交易平台

从行情数据 → 策略计算 → 模拟交易 → 收益分析的完整闭环。

## 设计原则

1. 数据可靠 > 策略复杂
2. 可解释策略 > 黑盒模型
3. 模拟验证 > 实盘尝试
4. 长期复利 > 短期暴利

## 架构

```
A股数据源 (AkShare / Tushare)
        │
        ▼
Data Collector (Python) ──► PostgreSQL
        │
        ├── Quant Engine (Python) 因子/策略/回测
        │
        └── Backend Service (Go)  API + 模拟交易
                    │
                    ▼
            React Dashboard
```

V1 不包含：高频交易、AI 预测、复杂机器学习、Kafka、微服务拆分、Redis（延至 Phase 2）。

## 目录结构

```
├── backend/          # Go 后端（API + 模拟交易，单进程）
├── collector/        # Python 数据采集（AkShare/Tushare）
├── quant-engine/     # Python 量化引擎（因子/策略/回测）
├── frontend/         # React + TypeScript + Ant Design
├── deploy/           # Docker Compose、DDL、Nginx 配置
├── docs/             # 设计文档
└── scripts/          # 开发/运维脚本
```

## 快速开始

```bash
# 1. 配置环境变量
cp deploy/.env.example deploy/.env
vim deploy/.env

# 2. 启动全部服务
cd deploy && docker compose up -d

# 3. 初始化数据库
docker exec -i quant-postgres psql -U quant -d quant_system < postgres/init.sql

# API: http://localhost:8080/api/v1/stocks
# 前端: http://localhost:3000
```

详细设计见 [docs/设计思路.md](docs/设计思路.md) 与 [docs/技术准备文档.md](docs/技术准备文档.md)。

## 开发路线

| 阶段 | 内容 | 状态 |
|------|------|------|
| Sprint 0 | 工程初始化（仓库/DB/部署骨架） | ✅ 完成 |
| Sprint 1 | 数据中心（采集/回填/质量校验） | ✅ 完成 |
| Sprint 2 | 查询服务（股票/K线 API） | ✅ 完成 |
| Sprint 3 | 前端基础（列表/K线图表） | ✅ 完成 |
| Sprint 4 | 策略框架（因子/评分/轮动信号） | ✅ 完成 |
| Sprint 5 | 模拟交易（A股规则/净值快照） | 待开发 |
| Sprint 6 | Dashboard 完善（收益曲线/报告） | 待开发 |
