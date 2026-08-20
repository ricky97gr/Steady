import { useCallback, useEffect, useState } from 'react'
import { Card, Col, DatePicker, Descriptions, Empty, Row, Space, Table, Tag } from 'antd'
import type { Dayjs } from 'dayjs'

import { getMorningBrief } from '../../services/api'
import type {
  MorningBriefData,
  MorningBriefHotStockItem,
  MorningBriefIndexItem,
  MorningBriefSectorFlowItem,
  MorningBriefSectorGainItem,
} from '../../types'
import { formatAmount, formatPercent } from '../../utils/format'

// 市场节涨跌幅已是百分比数值（0.22 = 0.22%），与 formatPercent（小数单位）不同
const fmtPct = (v: number | null | undefined): string => {
  if (v === null || v === undefined || Number.isNaN(v)) return '--'
  return `${v > 0 ? '+' : ''}${v.toFixed(2)}%`
}

const statusTag = (s: string): React.ReactNode => {
  const map: Record<string, [string, string]> = {
    success: ['green', '成功'],
    skipped: ['orange', '跳过'],
    failed: ['red', '失败'],
    none: ['default', '未执行'],
    ok: ['green', '通过'],
    warn: ['orange', '警告'],
    fail: ['red', '异常'],
  }
  const [color, text] = map[s] ?? ['default', s]
  return <Tag color={color} style={{ borderRadius: 6 }}>{text}</Tag>
}

export default function MorningBriefPage() {
  const [brief, setBrief] = useState<MorningBriefData | null>(null)
  const [date, setDate] = useState<Dayjs | null>(null)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async (d?: Dayjs) => {
    setLoading(true)
    try {
      setBrief(await getMorningBrief(d ? d.format('YYYY-MM-DD') : undefined))
    } catch {
      setBrief(null) // 40004 无数据：Empty 兜底
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const onDateChange = (d: Dayjs | null) => {
    setDate(d)
    void load(d ?? undefined)
  }

  if (!brief && !loading) {
    return (
      <Empty
        description="该日暂无早盘简报（每日 09:10 由量化引擎生成）"
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        style={{ padding: 80 }}
      />
    )
  }

  const sections = brief?.sections
  const market = sections?.market
  const yesterday = sections?.yesterday
  const today = sections?.today

  const indexColumns = [
    { title: '名称', dataIndex: 'name' },
    { title: '代码', dataIndex: 'code', width: 90 },
    {
      title: '最新点位',
      dataIndex: 'close',
      width: 110,
      render: (v: number | null) => (v == null ? '--' : v.toLocaleString('zh-CN', { maximumFractionDigits: 2 })),
    },
    { title: '涨跌幅', dataIndex: 'change_pct', width: 110, render: fmtPct },
  ]

  const gainColumns = [
    { title: '板块', dataIndex: 'name' },
    { title: '涨跌幅', dataIndex: 'change_pct', width: 100, render: fmtPct },
    { title: '领涨股', dataIndex: 'leader', render: (v: string | null) => v || '--' },
  ]

  const flowColumns = [
    { title: '板块', dataIndex: 'name' },
    { title: '净流入', dataIndex: 'net_inflow', width: 120 },
  ]

  const hotColumns = [
    { title: '排名', dataIndex: 'rank', width: 60 },
    { title: '代码', dataIndex: 'code', width: 90 },
    { title: '名称', dataIndex: 'name' },
    { title: '涨跌幅', dataIndex: 'change_pct', width: 100, render: fmtPct },
    {
      title: '连板',
      dataIndex: 'board_days',
      width: 70,
      render: (v: number | undefined) => (v != null && v > 1 ? <Tag color="volcano">{v}板</Tag> : '--'),
    },
    { title: '行业', dataIndex: 'industry', render: (v: string | null | undefined) => v || '--' },
  ]

  const taskColumns = [
    { title: '任务', dataIndex: 'task_name', render: (v: string) => <code>{v}</code> },
    { title: '状态', dataIndex: 'status', width: 90, render: statusTag },
    { title: '信息', dataIndex: 'message' },
  ]

  return (
    <div>
      <Row justify="space-between" align="middle" style={{ marginBottom: 16 }}>
        <Space size="middle" wrap>
          <DatePicker
            value={date}
            onChange={onDateChange}
            allowClear
            placeholder="选择日期（缺省最近）"
            format="YYYY-MM-DD"
          />
          {sections?.brief_date && (
            <span style={{ color: '#666' }}>
              <b>{sections.brief_date}</b>
              {sections.is_open_today ? '（开市）' : '（休市）'}
              {sections.trade_date && <> · 回顾 {sections.trade_date}</>}
            </span>
          )}
        </Space>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} xl={14}>
          <Card title="🌏 市场概览" size="small">
            <Table<MorningBriefIndexItem>
              rowKey="code"
              size="small"
              pagination={false}
              dataSource={market?.indices ?? []}
              columns={indexColumns}
            />
          </Card>
        </Col>
        <Col xs={24} xl={10}>
          <Card title="💼 持仓" size="small" styles={{ body: { padding: 16 } }}>
            {today?.positions?.length ? (
              today.positions.map((p) => (
                <div key={p.code} style={{ display: 'flex', justifyContent: 'space-between', padding: '4px 0' }}>
                  <span>{p.name} <code>{p.code}</code></span>
                  <span style={{ color: '#666' }}>
                    {p.quantity} 股 · ¥{formatAmount(p.market_value)}
                    <span style={{ marginLeft: 8, color: (p.profit_rate ?? 0) >= 0 ? '#cf1322' : '#3f8600' }}>
                      {formatPercent(p.profit_rate)}
                    </span>
                  </span>
                </div>
              ))
            ) : (
              <Empty description="暂无持仓" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} xl={12}>
          <Card title="🔥 板块涨幅 TOP" size="small">
            <Table<MorningBriefSectorGainItem>
              rowKey="name"
              size="small"
              pagination={false}
              dataSource={market?.sectors_gain ?? []}
              columns={gainColumns}
            />
          </Card>
        </Col>
        <Col xs={24} xl={12}>
          <Card title="💰 板块资金净流入 TOP" size="small">
            <Table<MorningBriefSectorFlowItem>
              rowKey="name"
              size="small"
              pagination={false}
              dataSource={market?.sectors_flow ?? []}
              columns={flowColumns}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} xl={12}>
          <Card title="⭐ 活跃个股" size="small">
            <Table<MorningBriefHotStockItem>
              rowKey="code"
              size="small"
              pagination={false}
              dataSource={market?.hot_stocks ?? []}
              columns={hotColumns}
            />
          </Card>
        </Col>
        <Col xs={24} xl={12}>
          <Card title="📅 今日计划" size="small" styles={{ body: { padding: 16 } }}>
            {today?.checklist?.length ? (
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                {today.checklist.map((c) => (
                  <Tag key={c.time + c.task} color="blue" style={{ borderRadius: 6 }}>
                    {c.time} {c.task}
                  </Tag>
                ))}
              </div>
            ) : (
              <Empty description="暂无计划" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
            <div style={{ marginTop: 16 }}>
              <Descriptions size="small" column={1} title="昨日回顾" colon={false}>
                <Descriptions.Item label="📈 信号">
                  {yesterday?.signal ? `${yesterday.signal.total} 条` : '--'}
                  {yesterday?.signal?.counts
                    ? Object.entries(yesterday.signal.counts).map(([a, c]) => (
                        <Tag key={a} style={{ marginLeft: 8 }}>{a} {c}</Tag>
                      ))
                    : null}
                </Descriptions.Item>
                <Descriptions.Item label="💹 成交">
                  {yesterday?.trade
                    ? `买 ${yesterday.trade.buy_count} · 卖 ${yesterday.trade.sell_count}`
                    : '--'}
                </Descriptions.Item>
                <Descriptions.Item label="💰 净值">
                  {yesterday?.nav?.nav != null
                    ? `${yesterday.nav.nav}（${formatPercent(yesterday.nav.daily_return)}）`
                    : '--'}
                </Descriptions.Item>
                <Descriptions.Item label="✅ 数据健康">
                  {statusTag(yesterday?.data_health?.overall ?? 'none')}
                  <span style={{ marginLeft: 8, color: '#888' }}>
                    {yesterday?.data_health?.message ?? ''}
                  </span>
                </Descriptions.Item>
              </Descriptions>
            </div>
          </Card>
        </Col>
      </Row>

      <Card title="🗂️ 昨日任务执行" size="small" style={{ marginTop: 16 }}>
        <Table
          rowKey={(r: { task_name: string }) => r.task_name}
          size="small"
          pagination={false}
          dataSource={yesterday?.tasks ?? []}
          columns={taskColumns}
        />
      </Card>
    </div>
  )
}
