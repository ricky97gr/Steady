-- ============================================================
-- Quant System 数据库初始化脚本
-- PostgreSQL 16
-- 表结构对应 docs/技术准备文档.md §5
-- ============================================================

-- 扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ------------------------------------------------------------
-- 1. 股票基本信息
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS stock_basic (
    code       VARCHAR(10)  PRIMARY KEY,
    name       VARCHAR(50)  NOT NULL,
    market     VARCHAR(10),                -- SH / SZ / BJ
    industry   VARCHAR(50),
    list_date  DATE,
    status     VARCHAR(10)  DEFAULT 'L',   -- L=上市 / D=退市
    universe   VARCHAR(20),                -- hs300 / zz500 / NULL=全市场
    created_at TIMESTAMP    DEFAULT NOW(),
    updated_at TIMESTAMP    DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stock_basic_universe ON stock_basic (universe);

-- ------------------------------------------------------------
-- 2. 日行情
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS daily_price (
    id         BIGSERIAL     PRIMARY KEY,
    code       VARCHAR(10)   NOT NULL REFERENCES stock_basic (code),
    trade_date DATE          NOT NULL,
    open       DECIMAL(10,2),
    high       DECIMAL(10,2),
    low        DECIMAL(10,2),
    close      DECIMAL(10,2),
    volume     BIGINT,                       -- 成交量（手）
    amount     DECIMAL(15,2),                -- 成交额（元）
    adj_factor DECIMAL(10,4)                 -- 复权因子
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_daily_price_code_date
    ON daily_price (code, trade_date);

-- ------------------------------------------------------------
-- 3. 财务指标（含公告日，防止未来函数）
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS financial_indicator (
    id             BIGSERIAL     PRIMARY KEY,
    code           VARCHAR(10)   NOT NULL REFERENCES stock_basic (code),
    report_date    DATE          NOT NULL,   -- 报告期
    pe             DECIMAL(10,2),
    pb             DECIMAL(10,2),
    roe            DECIMAL(10,4),
    profit_growth  DECIMAL(10,4),
    revenue_growth DECIMAL(10,4),
    debt_ratio     DECIMAL(10,4),
    gross_margin   DECIMAL(10,4),
    announce_date  DATE          NOT NULL,   -- 公告日（财报实际披露日）
    created_at     TIMESTAMP     DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_financial_code_report
    ON financial_indicator (code, report_date);
CREATE INDEX IF NOT EXISTS idx_financial_code_announce
    ON financial_indicator (code, announce_date);

-- ------------------------------------------------------------
-- 3.5 每日估值（Sprint 4 新增：日度 PE/PB/市值，东财 stock_value_em）
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS daily_valuation (
    id         BIGSERIAL     PRIMARY KEY,
    code       VARCHAR(10)   NOT NULL REFERENCES stock_basic (code),
    trade_date DATE          NOT NULL,
    close      DECIMAL(10,2),                -- 收盘价（元）
    total_mv   DECIMAL(18,2),                -- 总市值（元）
    float_mv   DECIMAL(18,2),                -- 流通市值（元）
    pe_ttm     DECIMAL(12,4),                -- 市盈率 TTM
    pe_static  DECIMAL(12,4),                -- 市盈率（静态）
    pb         DECIMAL(12,4),                -- 市净率
    created_at TIMESTAMP     DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_daily_valuation_code_date
    ON daily_valuation (code, trade_date);

-- ------------------------------------------------------------
-- 4. 因子定义
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS factor_definition (
    id          BIGSERIAL     PRIMARY KEY,
    name        VARCHAR(50)   UNIQUE NOT NULL,
    category    VARCHAR(20),                -- trend / value / quality / risk
    description TEXT,
    formula     TEXT,
    weight      DECIMAL(5,4),               -- 默认权重
    created_at  TIMESTAMP     DEFAULT NOW()
);

-- ------------------------------------------------------------
-- 5. 因子值
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS factor_value (
    id          BIGSERIAL     PRIMARY KEY,
    code        VARCHAR(10)   NOT NULL REFERENCES stock_basic (code),
    factor_name VARCHAR(50)   NOT NULL REFERENCES factor_definition (name),
    trade_date  DATE          NOT NULL,
    value       DECIMAL(15,6),
    rank        INT,                        -- 因子排名（横截面）
    normalized  DECIMAL(8,6)                -- 归一化值（0-1）
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_factor_value
    ON factor_value (code, factor_name, trade_date);

-- ------------------------------------------------------------
-- 6. 策略
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS strategy (
    id            BIGSERIAL     PRIMARY KEY,
    name          VARCHAR(50)   UNIQUE NOT NULL,
    description   TEXT,
    factor_weights JSONB,                   -- 因子权重配置
    params        JSONB,                    -- 策略参数（股票池/轮动阈值等）
    status        VARCHAR(10)   DEFAULT 'active',
    created_at    TIMESTAMP     DEFAULT NOW()
);

-- ------------------------------------------------------------
-- 7. 策略信号
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS strategy_signal (
    id            BIGSERIAL     PRIMARY KEY,
    strategy_name VARCHAR(50)   NOT NULL REFERENCES strategy (name),
    code          VARCHAR(10)   NOT NULL,
    trade_date    DATE          NOT NULL,
    score         DECIMAL(8,4),             -- 综合评分（0-100）
    action        VARCHAR(10),              -- BUY / SELL / HOLD
    reason        TEXT,
    created_at    TIMESTAMP     DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_strategy_signal
    ON strategy_signal (strategy_name, trade_date DESC);

-- ------------------------------------------------------------
-- 8. 账户
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS account (
    id           BIGSERIAL     PRIMARY KEY,
    name         VARCHAR(50)   DEFAULT '主账户',
    cash         DECIMAL(15,2) DEFAULT 100000,
    total_asset  DECIMAL(15,2),
    market_value DECIMAL(15,2) DEFAULT 0,
    profit       DECIMAL(15,2) DEFAULT 0,
    profit_rate  DECIMAL(8,4)  DEFAULT 0,
    max_drawdown DECIMAL(8,4)  DEFAULT 0,
    status       VARCHAR(10)   DEFAULT 'active',
    created_at   TIMESTAMP     DEFAULT NOW(),
    updated_at   TIMESTAMP     DEFAULT NOW()
);

-- ------------------------------------------------------------
-- 9. 持仓
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS position (
    id            BIGSERIAL     PRIMARY KEY,
    account_id    BIGINT        NOT NULL REFERENCES account (id),
    code          VARCHAR(10)   NOT NULL,
    quantity      INT           NOT NULL,   -- 持仓数量（股）
    available_qty INT           NOT NULL,   -- 可用数量（T+1：当日买入不计入）
    cost_price    DECIMAL(10,2),
    current_price DECIMAL(10,2),
    market_value  DECIMAL(15,2),
    profit        DECIMAL(15,2),
    profit_rate   DECIMAL(8,4),
    created_at    TIMESTAMP     DEFAULT NOW(),
    updated_at    TIMESTAMP     DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_position_account_code
    ON position (account_id, code);

-- ------------------------------------------------------------
-- 10. 委托单（order 为关键字，表名加引号）
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS "order" (
    id            BIGSERIAL     PRIMARY KEY,
    order_id      VARCHAR(36)   UNIQUE NOT NULL,  -- 委托编号（UUID，应用层生成）
    account_id    BIGINT        NOT NULL REFERENCES account (id),
    code          VARCHAR(10)   NOT NULL,
    direction     VARCHAR(10)   NOT NULL,         -- BUY / SELL
    order_type    VARCHAR(10)   DEFAULT 'LIMIT',  -- LIMIT / MARKET
    price         DECIMAL(10,2),
    quantity      INT           NOT NULL,
    filled_qty    INT           DEFAULT 0,
    avg_fill_price DECIMAL(10,2) DEFAULT 0,
    status        VARCHAR(20)   DEFAULT 'PENDING',
    reason        TEXT,
    source        VARCHAR(20)   DEFAULT 'strategy',
    created_at    TIMESTAMP     DEFAULT NOW(),
    updated_at    TIMESTAMP     DEFAULT NOW()
);

-- ------------------------------------------------------------
-- 11. 成交记录
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS trade (
    id          BIGSERIAL     PRIMARY KEY,
    trade_id    VARCHAR(36)   UNIQUE NOT NULL,
    order_id    VARCHAR(36)   NOT NULL,
    account_id  BIGINT        NOT NULL REFERENCES account (id),
    code        VARCHAR(10)   NOT NULL,
    direction   VARCHAR(10)   NOT NULL,
    price       DECIMAL(10,2),
    quantity    INT           NOT NULL,
    amount      DECIMAL(15,2),
    commission  DECIMAL(10,2),
    tax         DECIMAL(10,2),
    net_amount  DECIMAL(15,2),
    trade_date  DATE          NOT NULL,
    created_at  TIMESTAMP     DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_trade_order ON trade (order_id);
CREATE INDEX IF NOT EXISTS idx_trade_account ON trade (account_id, trade_date);

-- ------------------------------------------------------------
-- 12. 交易日历
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS trade_calendar (
    cal_date DATE         PRIMARY KEY,
    is_open  BOOLEAN      NOT NULL,
    exchange VARCHAR(10)  DEFAULT 'SSE'
);

-- ------------------------------------------------------------
-- 13. 每日净值快照
-- ------------------------------------------------------------
CREATE TABLE IF NOT EXISTS account_nav (
    id           BIGSERIAL     PRIMARY KEY,
    account_id   BIGINT        NOT NULL REFERENCES account (id),
    trade_date   DATE          NOT NULL,
    total_asset  DECIMAL(15,2),
    cash         DECIMAL(15,2),
    market_value DECIMAL(15,2),
    nav          DECIMAL(10,6),
    daily_return DECIMAL(8,4),
    drawdown     DECIMAL(8,4),
    created_at   TIMESTAMP     DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_account_nav
    ON account_nav (account_id, trade_date);

-- ============================================================
-- 预置数据
-- ============================================================

-- 因子定义（对应文档 §5.2.4）
INSERT INTO factor_definition (name, category, description, formula, weight) VALUES
    ('ma_trend',    'trend',   '均线趋势（MA5>MA20）', 'MA5 > MA20 为 1 否则 0',       0.2000),
    ('macd_signal', 'trend',   'MACD 金叉/死叉',     'DIF > DEA 为 1 否则 0',          0.2000),
    ('pe_ratio',    'value',   '市盈率',             'PE 越低分越高（横截面归一）',    0.1500),
    ('pb_ratio',    'value',   '市净率',             'PB 越低分越高（横截面归一）',    0.1500),
    ('roe_quality', 'quality', 'ROE 质量因子',       'ROE 越高分越高（横截面归一）',  0.2000),
    ('debt_risk',   'risk',    '负债风险因子',       '资产负债率越低分越高',          0.1000)
ON CONFLICT (name) DO NOTHING;

-- 策略定义（多因子：趋势40% + 价值30% + 质量20% + 风险10%）
INSERT INTO strategy (name, description, factor_weights, params) VALUES (
    'multi_factor',
    '多因子评分策略：趋势40% + 价值30% + 质量20% + 风险10%，每日排名轮动',
    '{"trend": 0.40, "value": 0.30, "quality": 0.20, "risk": 0.10}',
    '{"universe": "hs300+zz500", "top_n": 20, "buy_buffer": 15, "sell_buffer": 30, "max_position_pct": 0.20}'
) ON CONFLICT (name) DO NOTHING;

-- 默认模拟账户（初始资金 10 万；无唯一约束，用 NOT EXISTS 保证可重复执行）
INSERT INTO account (name, cash, total_asset, market_value, profit, profit_rate)
SELECT '主账户', 100000.00, 100000.00, 0.00, 0.00, 0.0000
WHERE NOT EXISTS (SELECT 1 FROM account);
