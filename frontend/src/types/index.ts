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
}

export interface SignalQuery {
  strategy?: string
  date?: string
  action?: 'BUY' | 'SELL' | 'HOLD'
  limit?: number
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
