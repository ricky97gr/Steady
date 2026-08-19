// 统一 API 响应结构（对应后端 response.Body）
export interface ApiResponse<T> {
  code: number
  message: string
  data: T
  timestamp: string
}

export interface StockBasic {
  code: string
  name: string
  market: string
  industry: string
  list_date: string
  status: string
  universe: string
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
  volume: number
  amount: number
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
