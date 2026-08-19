import { useEffect, useMemo, useState } from 'react'
import { Alert, Button, Empty, Segmented, Spin } from 'antd'
import type { EChartsOption } from 'echarts'
import { getKline } from '../../services/api'
import { useEChart } from '../../hooks/useEChart'
import { rangeByMonth } from '../../utils/date'
import { formatWanYi } from '../../utils/format'
import type { Adjust, KLineItem } from '../../types'

interface KLineChartProps {
  code: string
  height?: number // 图表区域高度，默认 420
}

const ADJUST_OPTIONS = [
  { label: '不复权', value: 'none' },
  { label: '前复权', value: 'qfq' },
  { label: '后复权', value: 'hfq' },
]

const RANGE_OPTIONS = [
  { label: '近3月', value: '3m' },
  { label: '近6月', value: '6m' },
  { label: '近1年', value: '1y' },
]

const RANGE_MONTHS: Record<string, number> = { '3m': 3, '6m': 6, '1y': 12 }

const UP_COLOR = '#ef232a'
const DOWN_COLOR = '#14b143'
const MA_COLORS = { MA5: '#f5a623', MA10: '#1d39c4', MA20: '#8b5cf6' }

// 均线纯函数：前 n-1 位为 null，窗口均值保留 2 位
function calcMA(closes: number[], n: number): (number | null)[] {
  const result: (number | null)[] = new Array(closes.length).fill(null)
  let sum = 0
  for (let i = 0; i < closes.length; i++) {
    sum += closes[i]
    if (i >= n) sum -= closes[i - n]
    if (i >= n - 1) result[i] = Math.round((sum / n) * 100) / 100
  }
  return result
}

export default function KLineChart({ code, height = 420 }: KLineChartProps) {
  const [adjust, setAdjust] = useState<Adjust>('none')
  const [range, setRange] = useState('6m')
  const [items, setItems] = useState<KLineItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [retryTick, setRetryTick] = useState(0)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    const { start, end } = rangeByMonth(RANGE_MONTHS[range])
    getKline(code, { period: 'day', adjust, start, end })
      .then((data) => {
        if (!cancelled) setItems(data.items)
      })
      .catch((e: Error) => {
        if (!cancelled) setError(e.message)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [code, adjust, range, retryTick])

  const option = useMemo<EChartsOption | null>(() => {
    if (items.length === 0) return null
    const dates = items.map((i) => i.date)
    const closes = items.map((i) => i.close)
    const ma5 = calcMA(closes, 5)
    const ma10 = calcMA(closes, 10)
    const ma20 = calcMA(closes, 20)

    return {
      animation: false,
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'cross', link: [{ xAxisIndex: 'all' }] },
        formatter: (params: unknown) => {
          const list = params as { seriesType: string; dataIndex: number }[]
          const k = list.find((p) => p.seriesType === 'candlestick')
          if (!k) return ''
          const i = items[k.dataIndex]
          const prevClose = k.dataIndex > 0 ? items[k.dataIndex - 1].close : null
          const chg = prevClose ? ((i.close - prevClose) / prevClose) * 100 : null
          const rows = [
            `<div style="font-weight:600">${i.date}</div>`,
            `开盘 ${i.open.toFixed(2)}　收盘 <span style="color:${i.close >= i.open ? UP_COLOR : DOWN_COLOR}">${i.close.toFixed(2)}</span>`,
            `最高 ${i.high.toFixed(2)}　最低 ${i.low.toFixed(2)}`,
            `涨跌 ${chg !== null ? (chg >= 0 ? '+' : '') + chg.toFixed(2) + '%' : '--'}`,
            `成交量 ${formatWanYi(i.volume)}手　成交额 ${formatWanYi(i.amount)}`,
          ]
          return rows.join('<br/>')
        },
      },
      legend: {
        data: ['MA5', 'MA10', 'MA20'],
        top: 0,
        right: 12,
        icon: 'roundRect',
        itemWidth: 14,
        itemHeight: 2,
        textStyle: { color: '#8a97ab' },
      },
      grid: [
        { left: 12, right: 12, top: 34, height: '58%' },
        { left: 12, right: 12, top: '74%', height: '15%', bottom: 40 },
      ],
      xAxis: [
        { type: 'category', data: dates, boundaryGap: true, axisLabel: { hideOverlap: true, color: '#8a97ab' }, axisLine: { lineStyle: { color: '#e6ecf5' } } },
        { type: 'category', gridIndex: 1, data: dates, axisLabel: { show: false }, axisLine: { show: false }, axisTick: { show: false } },
      ],
      yAxis: [
        { scale: true, splitLine: { lineStyle: { color: '#eef2fa' } }, axisLabel: { color: '#8a97ab' } },
        { gridIndex: 1, splitNumber: 2, axisLabel: { show: false }, splitLine: { show: false } },
      ],
      dataZoom: [
        { type: 'inside', xAxisIndex: [0, 1], start: 0, end: 100 },
        { type: 'slider', xAxisIndex: [0, 1], bottom: 0, height: 18, borderColor: '#e6ecf5', textStyle: { color: '#8a97ab' } },
      ],
      series: [
        {
          name: '日K',
          type: 'candlestick',
          data: items.map((i) => [i.open, i.close, i.low, i.high]),
          itemStyle: { color: UP_COLOR, color0: DOWN_COLOR, borderColor: UP_COLOR, borderColor0: DOWN_COLOR },
        },
        { name: 'MA5', type: 'line', data: ma5, smooth: true, symbol: 'none', lineStyle: { width: 1, color: MA_COLORS.MA5 } },
        { name: 'MA10', type: 'line', data: ma10, smooth: true, symbol: 'none', lineStyle: { width: 1, color: MA_COLORS.MA10 } },
        { name: 'MA20', type: 'line', data: ma20, smooth: true, symbol: 'none', lineStyle: { width: 1, color: MA_COLORS.MA20 } },
        {
          name: '成交量',
          type: 'bar',
          xAxisIndex: 1,
          yAxisIndex: 1,
          data: items.map((i) => i.volume),
          itemStyle: {
            color: (p: { dataIndex: number }) => (items[p.dataIndex].close >= items[p.dataIndex].open ? UP_COLOR : DOWN_COLOR),
          },
        },
      ],
    }
  }, [items])

  const chartRef = useEChart(option, [option])

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 12, flexWrap: 'wrap', gap: 8 }}>
        <Segmented options={ADJUST_OPTIONS} value={adjust} onChange={(v) => setAdjust(v as Adjust)} size="small" />
        <Segmented options={RANGE_OPTIONS} value={range} onChange={(v) => setRange(String(v))} size="small" />
      </div>
      <div style={{ height, position: 'relative' }}>
        {loading && (
          <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
            <Spin tip="加载K线中..." />
          </div>
        )}
        {!loading && error && (
          <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
            <Alert
              type="error"
              message="K线加载失败"
              description={error}
              action={
                <Button size="small" onClick={() => setRetryTick((t) => t + 1)}>
                  重试
                </Button>
              }
            />
          </div>
        )}
        {!loading && !error && items.length === 0 && (
          <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%' }}>
            <Empty description="暂无行情数据，待数据回填" />
          </div>
        )}
        {!loading && !error && items.length > 0 && <div ref={chartRef} style={{ height: '100%' }} />}
      </div>
    </div>
  )
}
