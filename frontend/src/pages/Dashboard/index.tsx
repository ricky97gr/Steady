import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, Card, Col, Empty, Row, Statistic } from 'antd'
import {
  AccountBookOutlined,
  BarChartOutlined,
  LineChartOutlined,
  ReloadOutlined,
  StockOutlined,
} from '@ant-design/icons'
import * as echarts from 'echarts'
import type { EChartsOption } from 'echarts'

import StatCard from '../../components/StatCard'
import { useEChart } from '../../hooks/useEChart'
import {
  getAccount,
  getAccountNav,
  getIndexNav,
  getSignals,
  getStocks,
  getStrategies,
} from '../../services/api'
import type { AccountData, AccountNavItem, StrategyInfo } from '../../types'
import { formatAmount } from '../../utils/format'

// A 股红涨绿跌
const UP = '#cf1322'
const DOWN = '#3f8600'

/** 策略净值 vs 沪深300 双线（账户快照日轴，指数按日期对齐） */
function NavChart({
  accountItems,
  indexItems,
}: {
  accountItems: AccountNavItem[]
  indexItems: { trade_date: string; nav: number }[]
}) {
  const indexByDate = useMemo(() => {
    const m = new Map<string, number>()
    for (const it of indexItems) m.set(it.trade_date, it.nav)
    return m
  }, [indexItems])

  const option = useMemo<EChartsOption | null>(() => {
    if (!accountItems.length) return null
    const dates = accountItems.map((it) => it.trade_date.slice(5)) // YYYY-MM-DD → MM-DD
    return {
      tooltip: {
        trigger: 'axis',
        valueFormatter: (v) => `${((Number(v) - 1) * 100).toFixed(2)}%`,
      },
      legend: { data: ['策略净值', '沪深300'], top: 4, right: 12, icon: 'roundRect' },
      grid: { left: 16, right: 20, top: 44, bottom: 12, containLabel: true },
      xAxis: {
        type: 'category',
        data: dates,
        axisLine: { lineStyle: { color: '#e6ecf5' } },
        axisTick: { show: false },
        axisLabel: { color: '#8a97ab' },
      },
      yAxis: {
        type: 'value',
        axisLabel: { color: '#8a97ab', formatter: (v: number) => `${((v - 1) * 100).toFixed(0)}%` },
        splitLine: { lineStyle: { color: '#eef2fa' } },
      },
      series: [
        {
          name: '策略净值',
          type: 'line',
          data: accountItems.map((it) => it.nav),
          smooth: true,
          symbol: 'none',
          lineStyle: { width: 3, color: '#1d39c4' },
          areaStyle: {
            color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
              { offset: 0, color: 'rgba(29, 57, 196, 0.28)' },
              { offset: 1, color: 'rgba(29, 57, 196, 0.02)' },
            ]),
          },
        },
        {
          name: '沪深300',
          type: 'line',
          data: accountItems.map((it) => indexByDate.get(it.trade_date) ?? null),
          smooth: true,
          symbol: 'none',
          lineStyle: { width: 2, color: '#f5a623', type: 'dashed' },
        },
      ],
    }
  }, [accountItems, indexByDate])
  const domRef = useEChart(option, [accountItems, indexByDate])
  return <div ref={domRef} style={{ height: 320 }} />
}

/** 策略速览单行 */
function InfoRow({ label, value, tag }: { label: string; value: string; tag?: string }) {
  return (
    <div
      style={{
        background: '#f7f9fd',
        borderRadius: 10,
        padding: '10px 14px',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
      }}
    >
      <div>
        <div style={{ fontSize: 13, color: '#8a97ab' }}>{label}</div>
        <div style={{ fontWeight: 600 }}>{value}</div>
      </div>
      {tag && <div style={{ fontSize: 11, color: '#8a97ab', textAlign: 'right' }}>{tag}</div>}
    </div>
  )
}

export default function DashboardPage() {
  const [account, setAccount] = useState<AccountData | null>(null)
  const [navItems, setNavItems] = useState<AccountNavItem[]>([])
  const [indexItems, setIndexItems] = useState<{ trade_date: string; nav: number }[]>([])
  const [poolSize, setPoolSize] = useState<number | null>(null)
  const [strategies, setStrategies] = useState<StrategyInfo[]>([])
  const [signals, setSignals] = useState<{ trade_date: string; buys: { name: string; score: number }[] } | null>(null)
  const [loading, setLoading] = useState(true)
  const [reloadTick, setReloadTick] = useState(0)

  const load = useCallback(async () => {
    const [acc, accNav, stockList, stg, sig] = await Promise.all([
      getAccount(),
      getAccountNav(),
      getStocks({ page_size: 1 }),
      getStrategies(),
      getSignals({}),
    ])
    // 基准与账户同起点：首快照日起
    const firstDate = accNav.items[0]?.trade_date
    const index = firstDate
      ? await getIndexNav('sh000300', { start: firstDate }).catch(() => null)
      : null
    setAccount(acc)
    setNavItems(accNav.items)
    setIndexItems(index?.items ?? [])
    setPoolSize(stockList.total)
    setStrategies(stg.items)
    const s = sig
    setSignals(
      s.items.length
        ? {
            trade_date: s.trade_date,
            buys: s.items
              .filter((it) => it.action === 'BUY')
              .slice(0, 5)
              .map((it) => ({ name: it.name, score: it.score })),
          }
        : null,
    )
  }, [])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    load()
      .catch(() => undefined)
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [reloadTick, load])

  // 60s 自动刷新（页面隐藏时暂停）
  useEffect(() => {
    const timer = setInterval(() => {
      if (!document.hidden) setReloadTick((t) => t + 1)
    }, 60000)
    return () => clearInterval(timer)
  }, [])

  const profitRate = account ? account.profit_rate * 100 : 0
  const firstIdx = indexItems[0]?.nav
  const lastIdx = indexItems[indexItems.length - 1]?.nav
  const benchReturn = firstIdx && lastIdx ? ((lastIdx / firstIdx - 1) * 100).toFixed(2) : '--'

  const strategy = strategies[0]
  const factorNames = strategy ? Object.keys(strategy.factor_weights ?? {}) : []
  const params = (strategy?.params ?? {}) as Record<string, unknown>
  const signalsBuyCount = signals?.buys.length ?? 0

  return (
    <div className="page-container">
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <StatCard
            title="账户总资产"
            value={account ? account.total_asset : 0}
            precision={2}
            suffix="元"
            icon={<AccountBookOutlined />}
            color="blue"
            footer={account ? `初始资金 ${formatAmount(account.initial_cash)} 元` : '--'}
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card styles={{ body: { padding: '20px 24px' } }}>
            <Statistic
              title="累计收益率"
              value={account ? profitRate : 0}
              precision={2}
              suffix="%"
              valueStyle={{ color: profitRate >= 0 ? UP : DOWN, fontWeight: 600 }}
            />
            <div style={{ marginTop: 8, color: '#8a97ab', fontSize: 12 }}>
              同期沪深300 {benchReturn}
            </div>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <StatCard
            title="股票池规模"
            value={poolSize ?? 0}
            icon={<StockOutlined />}
            color="orange"
            footer="沪深300 + 中证500（含ST剔除）"
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <StatCard
            title="监控因子"
            value={factorNames.length}
            icon={<LineChartOutlined />}
            color="purple"
            footer={factorNames.length ? factorNames.join(' / ') : '--'}
          />
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={16}>
          <Card
            title="策略收益曲线（对比沪深300）"
            extra={
              <Button
                size="small"
                icon={<ReloadOutlined />}
                loading={loading}
                onClick={() => setReloadTick((t) => t + 1)}
              >
                刷新
              </Button>
            }
            styles={{ body: { padding: '8px 16px 16px' } }}
          >
            {navItems.length ? (
              <NavChart accountItems={navItems} indexItems={indexItems} />
            ) : (
              <Empty
                description="暂无净值快照（模拟账户净值每日 21:05 生成），稍后自动刷新"
                style={{ padding: '40px 0' }}
              />
            )}
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card title="策略速览" styles={{ body: { padding: 16 } }}>
            {!strategies.length ? (
              <Empty description="暂无策略数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <InfoRow label="当前策略" value={strategy.name} tag={strategy.description || undefined} />
                <InfoRow
                  label="参数"
                  value={`Top${(params.top_n as number) ?? 20} · 单票上限 ${((params.max_position_pct as number) ?? 20)}%`}
                  tag="每日排名轮动"
                />
                <InfoRow
                  label="因子权重"
                  value={factorNames.length ? factorNames.map((k) => `${k} ${((strategy.factor_weights[k] ?? 0) * 100).toFixed(0)}%`).join(' + ') : '--'}
                />
                <InfoRow
                  label="最新信号"
                  value={signals ? `${signals.trade_date} · ${signalsBuyCount} 只买入` : '暂无信号（因子每日 19:00 计算）'}
                  tag={signals?.buys.length ? signals.buys.map((b) => b.name).join('、') : undefined}
                />
                <div
                  style={{
                    marginTop: 4,
                    padding: '10px 14px',
                    borderRadius: 10,
                    background: '#f0f5ff',
                    color: '#1d39c4',
                    fontSize: 12,
                    display: 'flex',
                    gap: 8,
                    alignItems: 'center',
                  }}
                >
                  <BarChartOutlined />
                  模拟交易 19:35 自动执行 · 回测引擎每 5 分钟消费任务
                </div>
              </div>
            )}
          </Card>
        </Col>
      </Row>
    </div>
  )
}
