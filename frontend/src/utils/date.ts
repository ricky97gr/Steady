// 日期工具：后端 K线 start/end 参数要求 YYYY-MM-DD（本地时区即可）
export function formatDateStr(d: Date): string {
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}

export function addDays(d: Date, n: number): Date {
  const r = new Date(d)
  r.setDate(r.getDate() + n)
  return r
}

/** 按日历月数换算起始日期窗口（3月=90 天 / 6月=180 / 1年=365），end 为今天 */
export function rangeByMonth(months: number): { start: string; end: string } {
  const end = new Date()
  return { start: formatDateStr(addDays(end, -Math.round(months * 30))), end: formatDateStr(end) }
}
