// 统一 API 响应结构（对应后端 response.Body）
export interface ApiResponse<T> {
  code: number
  message: string
  data: T
  timestamp: string
}

// 枚举类型（与后端参数/字段取值对齐）
export type Market = 'SH' | 'SZ' | 'BJ'
export type Universe = 'hs300' | 'zz500'
export type Adjust = 'none' | 'qfq' | 'hfq'
export type SortField = 'code' | 'name' | 'list_date' | 'market' | 'industry'

export interface StockBasic {
  code: string
  name: string
  market: Market
  industry: string
  list_date: string // YYYY-MM-DD，缺失时为空串
  status: string
  universe: string // hs300 / zz500 / ''（空串=全市场候选）
}

export interface StockListData {
  total: number
  page: number
  page_size: number
  items: StockBasic[]
}

export interface KLineItem {
  date: string
  open: number
  high: number
  low: number
  close: number
  volume: number // 单位：手
  amount: number // 单位：元
}

export interface KLineData {
  code: string
  period: string
  adjust: string
  items: KLineItem[]
}

export interface FinancialItem {
  report_date: string // YYYY-MM-DD
  announce_date: string
  pe: number
  pb: number
  roe: number // 以下 5 个字段为原始百分数值（15.2 = 15.2%），0 视为缺失
  profit_growth: number
  revenue_growth: number
  debt_ratio: number
  gross_margin: number
}

export interface FinancialListData {
  code: string
  items: FinancialItem[]
}

// 每日估值（日度 PE(TTM)/PB，Sprint 4 daily_valuation 回填后可用）
export interface Valuation {
  trade_date: string // YYYY-MM-DD
  pe_ttm: number
  pb: number
}

export interface StockDetail extends StockBasic {
  latest_bar: KLineItem | null // 行情未回填时为 null
  financial_summary: FinancialItem | null // 财务未回填时为 null
  valuation: Valuation | null // 日度估值未回填时为 null
}

export interface StockListQuery {
  page?: number
  page_size?: number
  industry?: string
  keyword?: string
  market?: Market
  universe?: Universe
  sort?: SortField
  order?: 'asc' | 'desc'
}

// ---- 模拟交易（Sprint 5）----

export interface AccountData {
  account_id: number
  name: string
  cash: number
  market_value: number
  total_asset: number
  profit: number
  profit_rate: number
  max_drawdown: number
  initial_cash: number
}

export interface AccountNavItem {
  trade_date: string // YYYY-MM-DD
  total_asset: number
  nav: number
  daily_return: number
  drawdown: number
}

export interface AccountNavData {
  items: AccountNavItem[]
}

export interface PositionItem {
  code: string
  name: string
  quantity: number
  available_qty: number // < quantity 表示 T+1 冻结
  cost_price: number
  current_price: number
  market_value: number
  profit: number
  profit_rate: number
}

export interface PositionsData {
  items: PositionItem[]
}

export type OrderStatus = 'PENDING' | 'FILLED' | 'REJECTED' | 'CANCELLED'
export type OrderDirection = 'BUY' | 'SELL'

export interface OrderItem {
  order_id: string
  code: string
  direction: OrderDirection
  order_type: string
  price: number
  quantity: number
  filled_qty: number
  avg_fill_price: number
  status: OrderStatus
  reason: string
  source: string
  created_at: string // YYYY-MM-DD
}

export interface OrdersData {
  items: OrderItem[]
  total: number
  page: number
  page_size: number
}

export interface TradeItem {
  trade_id: string
  order_id: string
  code: string
  direction: OrderDirection
  price: number
  quantity: number
  amount: number
  commission: number
  tax: number
  net_amount: number
  trade_date: string // YYYY-MM-DD
}

export interface TradesData {
  items: TradeItem[]
  total: number
  page: number
  page_size: number
}

// 策略信号（列表项：含股票代码/名称）
export interface StrategySignal {
  code: string
  name: string
  score: number
  action: 'BUY' | 'SELL' | 'HOLD'
  reason: string
}

export interface SignalsData {
  strategy: string
  trade_date: string // '' = 尚无信号
  items: StrategySignal[]
  total: number
  page: number
  page_size: number
}

export interface SignalQuery {
  strategy?: string
  date?: string
  action?: 'BUY' | 'SELL' | 'HOLD'
  page?: number
  page_size?: number
}

// 策略定义（factor_weights 与 params 由 strategy 表 JSON 列反序列化）
export interface StrategyInfo {
  name: string
  description: string
  factor_weights: Record<string, number>
  params: Record<string, unknown>
  status: string
}

export interface StrategiesData {
  items: StrategyInfo[]
}

// ---- 指数基准 + 回测（Sprint 6）----

export interface IndexNavItem {
  trade_date: string // YYYY-MM-DD
  nav: number // 归一化净值（close/区间首日 close）
}

export interface IndexNavData {
  code: string
  items: IndexNavItem[]
}

export type BacktestStatus = 'pending' | 'running' | 'done' | 'failed'

export interface BacktestNavItem {
  date: string
  nav: number
  benchmark: number | null // 指数缺失日 null
}

export interface BacktestJobItem {
  id: number
  strategy_name: string
  start_date: string // YYYY-MM-DD
  end_date: string
  top_n: number
  status: BacktestStatus
  error: string
  created_at: string
  finished_at: string
  // 结果指标（done 后非零）
  total_return: number
  annualized_return: number
  max_drawdown: number
  sharpe: number
  trading_days: number
  final_value: number
  trades: number
  positions: number
  benchmark_return: number
  excess_return: number
  nav: BacktestNavItem[] // 详情接口才有
}

export interface BacktestsData {
  items: BacktestJobItem[]
}

// ---- 个股信号历史（个股页 tab，按日期倒序）----
export interface SignalHistoryItem {
  trade_date: string
  score: number
  action: 'BUY' | 'SELL' | 'HOLD'
  reason: string
}

export interface SignalHistoryData {
  code: string
  items: SignalHistoryItem[]
}

// ---- 通知与任务监控（Issue #5）----

export type NotifyScheduleType = 'weekday' | 'trading_day' | 'event'

export interface NotifyEvent {
  event_key: string
  name: string
  enabled: boolean
  schedule_type: NotifyScheduleType
  weekdays: string // '1,2,3,4,5'（1=周一..7=周日）
  send_at: string | null // HH:MM；event 型为 null
  template: string // 卡片模板色 blue/red/green
}

export interface FeishuConfig {
  enabled: boolean
  webhook_url: string
  dashboard_url: string
  timeout: number
  max_retries: number
  secret: string // 签名校验密钥；留空=不签名
  at_all: boolean // 通知卡片 @所有人
}

export interface NotifyConfigData {
  events: NotifyEvent[]
  feishu: FeishuConfig
}

// ---- 数据源配置（Tushare）----

export interface TushareConfig {
  configured: boolean // 是否已配置 token
  token_masked: string // 掩码预览 "****abcd"；未配置为 ""
}

export type TaskRunStatus = 'success' | 'skipped' | 'failed'

export interface TaskRunItem {
  id: number
  task_name: string
  run_date: string // YYYY-MM-DD
  status: TaskRunStatus
  message: string
  detail: Record<string, unknown> | null
  created_at: string
}

export interface TaskRunsData {
  items: TaskRunItem[]
}

// 手动触发 ExecuteDay 结果
export interface ExecuteDayResult {
  trade_date: string
  skipped: boolean
  buy_count: number
  sell_count: number
  manual: number
  rejected: number
  nav: number
}

// ---- 早盘简报（Issue #4）----

export interface MorningBriefIndexItem {
  name: string
  code: string
  close: number | null
  change_pct: number | null // 已是百分比数值（0.22 = 0.22%）
}

export interface MorningBriefSectorGainItem {
  name: string
  change_pct: number | null
  leader: string | null
}

export interface MorningBriefSectorFlowItem {
  name: string
  net_inflow: string | null // 已格式化字符串（"7.82亿"）
}

export interface MorningBriefHotStockItem {
  rank: number
  code: string
  name: string
  change_pct: number | null
  board_days?: number // 涨停池兜底时有（连板数）
  industry?: string | null
}

export interface MorningBriefMarket {
  indices: MorningBriefIndexItem[]
  sectors_gain: MorningBriefSectorGainItem[]
  sectors_flow: MorningBriefSectorFlowItem[]
  hot_stocks: MorningBriefHotStockItem[]
}

export interface MorningBriefTradeOrder {
  code: string
  direction: 'BUY' | 'SELL'
  price: number | null
  quantity: number
}

export interface MorningBriefYesterday {
  signal: { total: number; counts: Record<string, number>; top_buys: string[] }
  trade: {
    buy_count: number
    sell_count: number
    orders: MorningBriefTradeOrder[]
    message?: string
  }
  nav: { nav: number | null; daily_return: number | null; drawdown: number | null; total_asset: number | null }
  data_health: { overall: string; fail: number; warn: number; message: string }
  tasks: { task_name: string; status: string; message: string | null }[]
}

export interface MorningBriefToday {
  checklist: { time: string; task: string }[]
  positions: {
    code: string
    name: string
    quantity: number
    market_value: number | null
    profit_rate: number | null
  }[]
}

export interface MorningBriefSections {
  brief_date: string
  trade_date: string
  is_open_today: boolean
  market: MorningBriefMarket
  yesterday: MorningBriefYesterday
  today: MorningBriefToday
}

export interface MorningBriefData {
  brief_date: string
  sections: MorningBriefSections
}
