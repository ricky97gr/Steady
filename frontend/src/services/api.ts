import axios from 'axios'
import type { ApiResponse, StockListData } from '../types'

// axios 实例：统一 baseURL 与错误处理
const http = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
})

http.interceptors.response.use(
  (resp) => resp,
  (err) => {
    // TODO(Sprint 3): 统一错误提示（antd message）
    console.error('API 请求失败:', err)
    return Promise.reject(err)
  },
)

export async function getStocks(params: {
  page?: number
  page_size?: number
  industry?: string
}): Promise<StockListData> {
  const resp = await http.get<ApiResponse<StockListData>>('/stocks', { params })
  return resp.data.data
}

export const api = { getStocks }
