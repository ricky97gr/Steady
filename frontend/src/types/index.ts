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

export interface StockDetail extends StockBasic {
  latest_bar: KLineItem | null // 行情未回填时为 null
  financial_summary: FinancialItem | null // 财务未回填时为 null
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

export interface AccountInfo {
  id: number
  name: string
  cash: number
  total_asset: number
  market_value: number
  profit: number
  profit_rate: number
  max_drawdown: number
}

export interface PositionItem {
  code: string
  quantity: number
  available_qty: number
  cost_price: number
  current_price: number
  market_value: number
  profit: number
  profit_rate: number
}

export interface StrategySignal {
  code: string
  name: string
  score: number
  action: 'BUY' | 'SELL' | 'HOLD'
  reason: string
}
