// 数值格式化工具
export function formatAmount(v: number | null | undefined): string {
  if (v === null || v === undefined || Number.isNaN(v)) return '--'
  return v.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

export function formatPercent(v: number | null | undefined): string {
  if (v === null || v === undefined || Number.isNaN(v)) return '--'
  return `${(v * 100).toFixed(2)}%`
}

export function formatProfit(v: number | null | undefined): { text: string; color: string } {
  if (v === null || v === undefined || Number.isNaN(v)) {
    return { text: '--', color: 'inherit' }
  }
  return {
    text: `${v >= 0 ? '+' : ''}${v.toFixed(2)}`,
    color: v >= 0 ? '#cf1322' : '#3f8600',
  }
}
