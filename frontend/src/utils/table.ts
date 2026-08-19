import type { TablePaginationConfig } from 'antd'

/**
 * 全站表格统一分页配置：默认 20 条/页，可切换 10/20/50/100。
 * 所有页面表格（含服务端分页）一律引用，保证交互一致。
 */
export const PAGE_SIZE_OPTIONS = ['10', '20', '50', '100']

export function tablePagination(opts: {
  current?: number
  pageSize?: number
  total?: number
} = {}): TablePaginationConfig {
  return {
    current: opts.current,
    pageSize: opts.pageSize ?? 20,
    total: opts.total,
    showSizeChanger: true,
    pageSizeOptions: PAGE_SIZE_OPTIONS,
    showTotal: (t) => `共 ${t} 条`,
  }
}
