import { useEffect, useMemo, useState } from 'react'
import {
  Button,
  Card,
  Col,
  Empty,
  Popconfirm,
  Row,
  Space,
  Statistic,
  Table,
  Tabs,
  Tag,
  message,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  AccountBookOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  RiseOutlined,
  WalletOutlined,
} from '@ant-design/icons'
import * as echarts from 'echarts'
import type { EChartsOption } from 'echarts'

import StatCard from '../../components/StatCard'
import { useEChart } from '../../hooks/useEChart'
import {
  getAccount,
  getAccountNav,
  getOrders,
  getPositions,
  getTrades,
  manualExecuteDay,
} from '../../services/api'
import type { AccountData, AccountNavItem, OrderItem, PositionItem, TradeItem } from '../../types'
import { formatAmount, formatPercent } from '../../utils/format'
import { tablePagination } from '../../utils/table'

// A 股红涨绿跌
const UP = '#cf1322'
const DOWN = '#3f8600'

function ProfitText({ value, suffix = '' }: { value: number | undefined; suffix?: string }) {
  const sign = (value ?? 0) >= 0 ? '+' : ''
  return (
    <span style={{ color: (value ?? 0) >= 0 ? UP : DOWN }}>
      {sign}
      {formatAmount(value)}
      {suffix}
    </span>
  )
}

/** 净值曲线：y 轴为累计收益率（nav-1），与 Dashboard 样式一致 */
function NavChart({ items }: { items: AccountNavItem[] }) {
  const option = useMemo<EChartsOption | null>(() => {
    if (!items.length) return null
    const dates = items.map((it) => it.trade_date.slice(5)) // YYYY-MM-DD → MM-DD
    return {
      tooltip: {
        trigger: 'axis',
        // valueFormatter 参数类型为 OptionDataValue，转 number 计算
        valueFormatter: (v) => `${((Number(v) - 1) * 100).toFixed(2)}%`,
      },
      grid: { left: 16, right: 20, top: 28, bottom: 12, containLabel: true },
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
          name: '账户净值',
          type: 'line',
          data: items.map((it) => it.nav),
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
      ],
    }
  }, [items])
  const domRef = useEChart(option, [items])
  return <div ref={domRef} style={{ height: 320 }} />
}

const directionTag = (d: string) =>
  d === 'BUY' ? (
    <Tag color="red" style={{ borderRadius: 6 }}>买入</Tag>
  ) : (
    <Tag color="green" style={{ borderRadius: 6 }}>卖出</Tag>
  )

const statusTag = (s: OrderItem['status']) => {
  const map: Record<OrderItem['status'], [string, string]> = {
    PENDING: ['blue', '待成交'],
    FILLED: ['green', '已成交'],
    REJECTED: ['red', '已拒绝'],
    CANCELLED: ['default', '已撤销'],
  }
  const [color, text] = map[s] ?? ['default', s]
  return <Tag color={color} style={{ borderRadius: 6 }}>{text}</Tag>
}

export default function TradePage() {
  const [account, setAccount] = useState<AccountData | null>(null)
  const [navItems, setNavItems] = useState<AccountNavItem[]>([])
  const [positions, setPositions] = useState<PositionItem[]>([])
  const [orders, setOrders] = useState<OrderItem[]>([])
  const [trades, setTrades] = useState<TradeItem[]>([])
  const [loading, setLoading] = useState(true)
  const [reloadTick, setReloadTick] = useState(0)
  const [executing, setExecuting] = useState(false)

  // 手动触发 ExecuteDay + SnapshotDay：兜底"定时 19:35 已过 / 漏跑"的场景
  const onManualExecute = async () => {
    setExecuting(true)
    try {
      const res = await manualExecuteDay()
      if (res.skipped) {
        message.info(`已跳过：无最新行情（交易日 ${res.trade_date}），请先补跑信号`)
      } else {
        message.success(
          `执行完成：买入 ${res.buy_count} / 卖出 ${res.sell_count} / 拒绝 ${res.rejected}，` +
            `净值 ${res.nav.toFixed(4)}`,
        )
      }
      setReloadTick((t) => t + 1) // 刷新持仓 / 委托 / 净值
    } catch {
      // 错误已由 axios 拦截器统一弹出
    } finally {
      setExecuting(false)
    }
  }

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    // 客户端分页：一次拉满上限，翻页/切页大小不反复请求
    Promise.all([
      getAccount(),
      getAccountNav(),
      getPositions(),
      getOrders({ page_size: 100 }),
      getTrades({ page_size: 100 }),
    ])
      .then(([acc, nav, pos, ord, trd]) => {
        if (cancelled) return
        setAccount(acc)
        setNavItems(nav.items)
        setPositions(pos.items)
        setOrders(ord.items)
        setTrades(trd.items)
      })
      .catch(() => {
        if (!cancelled) setAccount(null) // 错误提示已由 axios 拦截器统一弹出，保留空态供重试
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [reloadTick])

  const positionColumns = useMemo<ColumnsType<PositionItem>>(
    () => [
      { title: '代码', dataIndex: 'code', width: 100 },
      { title: '名称', dataIndex: 'name', width: 120 },
      { title: '持仓', dataIndex: 'quantity', align: 'right', width: 80 },
      {
        title: '可用',
        dataIndex: 'available_qty',
        align: 'right',
        width: 110,
        render: (v: number, r) =>
          r.available_qty < r.quantity ? (
            <span>
              {v} <Tag color="orange" style={{ borderRadius: 6, marginLeft: 4 }}>T+1</Tag>
            </span>
          ) : (
            v
          ),
      },
      {
        title: '成本价',
        dataIndex: 'cost_price',
        align: 'right',
        width: 100,
        render: (v: number) => formatAmount(v),
      },
      {
        title: '现价',
        dataIndex: 'current_price',
        align: 'right',
        width: 100,
        render: (v: number) => formatAmount(v),
      },
      {
        title: '市值',
        dataIndex: 'market_value',
        align: 'right',
        width: 120,
        render: (v: number) => formatAmount(v),
      },
      {
        title: '盈亏',
        dataIndex: 'profit',
        align: 'right',
        width: 110,
        render: (v: number) => <ProfitText value={v} />,
      },
      {
        title: '盈亏率',
        dataIndex: 'profit_rate',
        align: 'right',
        width: 100,
        render: (v: number) => (
          <span style={{ color: v >= 0 ? UP : DOWN }}>{formatPercent(v)}</span>
        ),
      },
    ],
    [],
  )

  const orderColumns = useMemo<ColumnsType<OrderItem>>(
    () => [
      { title: '日期', dataIndex: 'created_at', width: 110 },
      { title: '代码', dataIndex: 'code', width: 90 },
      { title: '方向', dataIndex: 'direction', width: 80, render: directionTag },
      {
        title: '类型',
        dataIndex: 'order_type',
        width: 80,
        render: (v: string) => (v === 'MARKET' ? '市价' : '限价'),
      },
      {
        title: '委托价',
        dataIndex: 'price',
        align: 'right',
        width: 90,
        render: (v: number) => formatAmount(v),
      },
      { title: '数量', dataIndex: 'quantity', align: 'right', width: 80 },
      { title: '成交', dataIndex: 'filled_qty', align: 'right', width: 80 },
      {
        title: '成交均价',
        dataIndex: 'avg_fill_price',
        align: 'right',
        width: 90,
        render: (v: number) => (v > 0 ? formatAmount(v) : '--'),
      },
      { title: '状态', dataIndex: 'status', width: 90, render: statusTag },
      {
        title: '来源',
        dataIndex: 'source',
        width: 70,
        render: (v: string) => (v === 'strategy' ? '策略' : '手动'),
      },
      { title: '原因', dataIndex: 'reason', ellipsis: true },
    ],
    [],
  )

  const tradeColumns = useMemo<ColumnsType<TradeItem>>(
    () => [
      { title: '日期', dataIndex: 'trade_date', width: 110 },
      { title: '代码', dataIndex: 'code', width: 90 },
      { title: '方向', dataIndex: 'direction', width: 80, render: directionTag },
      {
        title: '成交价',
        dataIndex: 'price',
        align: 'right',
        width: 100,
        render: (v: number) => formatAmount(v),
      },
      { title: '数量', dataIndex: 'quantity', align: 'right', width: 80 },
      {
        title: '金额',
        dataIndex: 'amount',
        align: 'right',
        width: 110,
        render: (v: number) => formatAmount(v),
      },
      {
        title: '佣金',
        dataIndex: 'commission',
        align: 'right',
        width: 100,
        render: (v: number) => formatAmount(v),
      },
      {
        title: '印花税',
        dataIndex: 'tax',
        align: 'right',
        width: 100,
        render: (v: number) => formatAmount(v),
      },
      {
        title: '净额',
        dataIndex: 'net_amount',
        align: 'right',
        width: 110,
        render: (v: number) => formatAmount(v),
      },
    ],
    [],
  )

  const profit = account?.profit ?? 0
  const profitRate = account?.profit_rate ?? 0

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
          <StatCard
            title="可用资金"
            value={account ? account.cash : 0}
            precision={2}
            suffix="元"
            icon={<WalletOutlined />}
            color="green"
            footer={account ? `持仓市值 ${formatAmount(account.market_value)} 元` : '--'}
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card styles={{ body: { padding: '20px 24px' } }}>
            <Statistic
              title="累计盈亏"
              value={account ? account.profit : 0}
              precision={2}
              suffix="元"
              valueStyle={{ color: profit >= 0 ? UP : DOWN, fontWeight: 600 }}
            />
            <div style={{ marginTop: 8, color: '#8a97ab', fontSize: 12 }}>
              收益率 <ProfitText value={profitRate * 100} suffix="%" />
            </div>
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <StatCard
            title="最大回撤"
            value={account ? account.max_drawdown * 100 : 0}
            precision={2}
            suffix="%"
            icon={<RiseOutlined />}
            color="purple"
            footer="净值快照每日 21:05 更新"
          />
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card
            title="账户净值曲线"
            extra={
              <Space>
                <Popconfirm
                  title="确认手动执行当日交易？"
                  description="将按最新信号生成委托并撮合，同时写入当日净值快照。定时 19:35 执行过则自动跳过。"
                  okText="执行"
                  cancelText="取消"
                  onConfirm={onManualExecute}
                >
                  <Button
                    size="small"
                    type="primary"
                    ghost
                    icon={<PlayCircleOutlined />}
                    loading={executing}
                  >
                    手动执行当日交易
                  </Button>
                </Popconfirm>
                <Button
                  size="small"
                  icon={<ReloadOutlined />}
                  loading={loading}
                  onClick={() => setReloadTick((t) => t + 1)}
                >
                  刷新
                </Button>
              </Space>
            }
            styles={{ body: { padding: '8px 16px 16px' } }}
          >
            {navItems.length > 0 ? (
              <NavChart items={navItems} />
            ) : (
              <Empty
                description="暂无净值快照（首个交易日 21:05 后生成）"
                style={{ padding: '48px 0' }}
              />
            )}
          </Card>
        </Col>
      </Row>

      <Card styles={{ body: { paddingTop: 8 } }}>
        <Tabs
          defaultActiveKey="positions"
          items={[
            {
              key: 'positions',
              label: `持仓（${positions.length}）`,
              children: (
                <Table<PositionItem>
                  rowKey="code"
                  columns={positionColumns}
                  dataSource={positions}
                  loading={loading}
                  size="small"
                  pagination={tablePagination()}
                  locale={{ emptyText: '暂无持仓，等待策略信号自动建仓' }}
                />
              ),
            },
            {
              key: 'orders',
              label: `委托（${orders.length}）`,
              children: (
                <Table<OrderItem>
                  rowKey="order_id"
                  columns={orderColumns}
                  dataSource={orders}
                  loading={loading}
                  size="small"
                  pagination={tablePagination()}
                  locale={{ emptyText: '暂无委托记录' }}
                />
              ),
            },
            {
              key: 'trades',
              label: `成交（${trades.length}）`,
              children: (
                <Table<TradeItem>
                  rowKey="trade_id"
                  columns={tradeColumns}
                  dataSource={trades}
                  loading={loading}
                  size="small"
                  pagination={tablePagination()}
                  locale={{ emptyText: '暂无成交记录' }}
                />
              ),
            },
          ]}
        />
      </Card>
    </div>
  )
}
