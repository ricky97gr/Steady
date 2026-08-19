import { useEffect, useState } from 'react'
import { Card, Col, Empty, Progress, Row, Select, Table, Tag } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ExperimentOutlined, ReloadOutlined } from '@ant-design/icons'
import { Link } from 'react-router-dom'

import { getSignals } from '../../services/api'
import type { StrategySignal } from '../../types'

const FACTOR_WEIGHTS = [
  { name: '趋势（ma_trend + macd_signal）', weight: 40, color: '#1d39c4' },
  { name: '价值（pe_ratio + pb_ratio）', weight: 30, color: '#3ecf7a' },
  { name: '质量（roe_quality）', weight: 20, color: '#f5a623' },
  { name: '风险（debt_risk）', weight: 10, color: '#8b5cf6' },
]

const ACTION_LABEL: Record<string, string> = { BUY: '买入', SELL: '卖出', HOLD: '持有' }
const ACTION_COLOR: Record<string, string> = { BUY: 'green', SELL: 'red', HOLD: 'blue' }
const ACTION_OPTIONS = [
  { value: 'BUY', label: '买入' },
  { value: 'SELL', label: '卖出' },
  { value: 'HOLD', label: '持有' },
]

export default function StrategyPage() {
  const [action, setAction] = useState<'BUY' | 'SELL' | 'HOLD' | undefined>()
  const [reloadTick, setReloadTick] = useState(0)
  const [tradeDate, setTradeDate] = useState('')
  const [items, setItems] = useState<StrategySignal[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    getSignals({ action, limit: 500 })
      .then((d) => {
        if (cancelled) return
        setTradeDate(d.trade_date)
        setItems(d.items)
      })
      .catch(() => {
        if (cancelled) return
        setItems([])
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [action, reloadTick])

  const columns: ColumnsType<StrategySignal> = [
    {
      title: '代码',
      dataIndex: 'code',
      width: 90,
      render: (v: string) => <Link to={`/stocks/${v}`}>{v}</Link>,
    },
    { title: '名称', dataIndex: 'name', width: 130, ellipsis: true },
    {
      title: '评分',
      dataIndex: 'score',
      width: 90,
      render: (v: number) => <span style={{ fontWeight: 600 }}>{v.toFixed(1)}</span>,
    },
    {
      title: '信号',
      dataIndex: 'action',
      width: 90,
      render: (v: string) => (
        <Tag color={ACTION_COLOR[v]} style={{ borderRadius: 6 }}>
          {ACTION_LABEL[v] ?? v}
        </Tag>
      ),
    },
    { title: '说明', dataIndex: 'reason', ellipsis: true },
  ]

  return (
    <div className="page-container">
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={10}>
          <Card title="多因子权重" size="small" extra={<ExperimentOutlined />}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              {FACTOR_WEIGHTS.map((f) => (
                <div key={f.name}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
                    <span style={{ fontSize: 13 }}>{f.name}</span>
                    <span style={{ fontSize: 13, fontWeight: 600, color: f.color }}>{f.weight}%</span>
                  </div>
                  <Progress
                    percent={f.weight}
                    showInfo={false}
                    strokeColor={f.color}
                    strokeWidth={8}
                    trailColor="#eef2fa"
                  />
                </div>
              ))}
            </div>
            <div
              style={{
                marginTop: 16,
                padding: '10px 14px',
                borderRadius: 10,
                background: '#f0f5ff',
                color: '#1d39c4',
                fontSize: 12,
              }}
            >
              调仓规则：每日收盘后按因子评分横截面排名，rank ≤ 15 买入、rank &gt; 30 卖出，单票上限 20%
            </div>
          </Card>
        </Col>
        <Col xs={24} lg={14}>
          <Card
            title="策略信号"
            size="small"
            extra={
              <Select
                allowClear
                placeholder="全部信号"
                style={{ width: 130 }}
                options={ACTION_OPTIONS}
                onChange={(v) => setAction(v)}
              />
            }
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12, color: '#8a97ab', fontSize: 12 }}>
              <span>信号日期：{tradeDate || '--'}</span>
              <span>·</span>
              <span>共 {items.length} 条（评分降序）</span>
              <span style={{ marginLeft: 'auto' }}>
                <a onClick={() => setReloadTick((t) => t + 1)}>
                  <ReloadOutlined /> 刷新
                </a>
              </span>
            </div>
            <Table<StrategySignal>
              rowKey="code"
              columns={columns}
              dataSource={items}
              size="small"
              loading={loading}
              pagination={false}
              locale={{ emptyText: <Empty description="暂无信号（因子/信号任务尚未运行或当日非交易日）" image={Empty.PRESENTED_IMAGE_SIMPLE} /> }}
            />
          </Card>
        </Col>
      </Row>
    </div>
  )
}
