import axios from 'axios'
import { message } from 'antd'
import type {
  Adjust,
  ApiResponse,
  FinancialListData,
  KLineData,
  StockDetail,
  StockListData,
  StockListQuery,
} from '../types'

// 业务错误：code 为后端业务码（40001/40004/50001），status 为 HTTP 状态（网络错误时为 0）
export class ApiError extends Error {
  constructor(message: string, readonly code: number, readonly status: number) {
    super(message)
  }
}

// axios 实例：统一 baseURL 与错误处理
const http = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
})

http.interceptors.response.use(
  (resp) => {
    const body = resp.data as ApiResponse<unknown>
    if (body.code !== 0) {
      message.error(body.message || '请求失败')
      return Promise.reject(new ApiError(body.message || '请求失败', body.code, resp.status))
    }
    return resp
  },
  (err) => {
    const resp = err.response
    if (resp?.data?.message) {
      message.error(resp.data.message)
      return Promise.reject(new ApiError(resp.data.message, resp.data.code ?? 50001, resp.status))
    }
    message.error('网络异常，请稍后重试')
    return Promise.reject(new ApiError('网络异常，请稍后重试', 50001, resp?.status ?? 0))
  },
)

export async function getStocks(params: StockListQuery): Promise<StockListData> {
  const resp = await http.get<ApiResponse<StockListData>>('/stocks', { params })
  return resp.data.data
}

export async function getStockDetail(code: string): Promise<StockDetail> {
  const resp = await http.get<ApiResponse<StockDetail>>(`/stocks/${code}`)
  return resp.data.data
}

export async function getKline(
  code: string,
  params?: { period?: 'day'; adjust?: Adjust; start?: string; end?: string },
): Promise<KLineData> {
  const resp = await http.get<ApiResponse<KLineData>>(`/kline/${code}`, { params })
  return resp.data.data
}

export async function getFinancial(code: string, limit = 20): Promise<FinancialListData> {
  const resp = await http.get<ApiResponse<FinancialListData>>(`/stocks/${code}/financial`, {
    params: { limit },
  })
  return resp.data.data
}

export const api = { getStocks, getStockDetail, getKline, getFinancial }
