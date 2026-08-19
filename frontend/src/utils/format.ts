// 数值格式化工具
export function formatAmount(v: number | null | undefined): string {
  if (v === null || v === undefined || Number.isNaN(v)) return '--'
  return v.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

export function formatPercent(v: number | null | undefined): string {
  if (v === null || v === undefined || Number.isNaN(v)) return '--'
  return `${(v * 100).toFixed(2)}%`
}

// 财务字段为原始百分数值（15.2 = 15.2%），直接拼 %；≤0 视为缺失（Go 侧空值序列化为 0）
export function formatPct(v: number | null | undefined): string {
  if (v === null || v === undefined || Number.isNaN(v) || v <= 0) return '--'
  return `${v.toFixed(2)}%`
}

// 量/额格式化：≥1e8 → x.xx亿，≥1e4 → x.xx万；volume 单位手、amount 单位元
export function formatWanYi(v: number | null | undefined): string {
  if (v === null || v === undefined || Number.isNaN(v) || v <= 0) return '--'
  if (v >= 1e8) return `${(v / 1e8).toFixed(2)}亿`
  if (v >= 1e4) return `${(v / 1e4).toFixed(2)}万`
  return String(v)
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
