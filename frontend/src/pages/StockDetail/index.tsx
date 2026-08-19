import { useEffect, useState } from 'react'
import { Alert, Button, Card, Col, Descriptions, Empty, Result, Row, Skeleton, Statistic, Table, Tag } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { ArrowLeftOutlined } from '@ant-design/icons'
import { useNavigate, useParams } from 'react-router-dom'

import KLineChart from '../../components/KLineChart'
import { getFinancial, getStockDetail } from '../../services/api'
import type { FinancialItem, Market, StockDetail as StockDetailData } from '../../types'
import { formatAmount, formatPct, formatWanYi } from '../../utils/format'

const MARKET_LABEL: Record<Market, string> = { SH: '沪市', SZ: '深市', BJ: '北交所' }

function universeLabel(u: string) {
  if (u === 'hs300') return '沪深300'
  if (u === 'zz500') return '中证500'
  return '候选'
}

// 财务字段：百分比字段用 formatPct（原始百分数，≤0 视为缺失）；倍数字段 0 视为缺失
function ratio(v: number): string {
  return v > 0 ? formatAmount(v) : '--'
}

// 财务数值字段（字符串字段不入摘要统计卡）
type FinNumberKey = Exclude<keyof FinancialItem, 'report_date' | 'announce_date'>

const SUMMARY_ITEMS: { key: FinNumberKey; label: string; fmt: (v: number) => string }[] = [
  { key: 'pe', label: 'PE', fmt: ratio },
  { key: 'pb', label: 'PB', fmt: ratio },
  { key: 'roe', label: 'ROE', fmt: formatPct },
  { key: 'profit_growth', label: '净利润增速', fmt: formatPct },
  { key: 'revenue_growth', label: '营收增速', fmt: formatPct },
  { key: 'debt_ratio', label: '资产负债率', fmt: formatPct },
  { key: 'gross_margin', label: '毛利率', fmt: formatPct },
]

const FINANCIAL_COLUMNS: ColumnsType<FinancialItem> = [
  { title: '报告期', dataIndex: 'report_date', width: 110 },
  { title: '公告日', dataIndex: 'announce_date', width: 110 },
  { title: 'PE', dataIndex: 'pe', width: 80, render: ratio },
  { title: 'PB', dataIndex: 'pb', width: 80, render: ratio },
  { title: 'ROE', dataIndex: 'roe', width: 90, render: formatPct },
  { title: '净利润增速', dataIndex: 'profit_growth', width: 110, render: formatPct },
  { title: '营收增速', dataIndex: 'revenue_growth', width: 110, render: formatPct },
  { title: '资产负债率', dataIndex: 'debt_ratio', width: 110, render: formatPct },
  { title: '毛利率', dataIndex: 'gross_margin', width: 100, render: formatPct },
]

export default function StockDetailPage() {
  const { code = '' } = useParams<{ code: string }>()
  const navigate = useNavigate()

  const [detail, setDetail] = useState<StockDetailData | null>(null)
  const [financial, setFinancial] = useState<FinancialItem[]>([])
  const [loading, setLoading] = useState(true)
  const [notFound, setNotFound] = useState(false)
  const [error, setError] = useState('')
  const [retryTick, setRetryTick] = useState(0)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setNotFound(false)
    setError('')
    Promise.all([getStockDetail(code), getFinancial(code, 20)])
      .then(([d, f]) => {
        if (cancelled) return
        setDetail(d)
        setFinancial(f.items)
      })
      .catch((e: Error) => {
        if (cancelled) return
        // ApiError.code 40004 = 股票不存在（拦截器已 message 提示）
        if ('code' in e && (e as { code: number }).code === 40004) setNotFound(true)
        else setError(e.message)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [code, retryTick])

  if (loading) {
    return (
      <Card>
        <Skeleton active paragraph={{ rows: 8 }} />
      </Card>
    )
  }

  if (notFound) {
    return (
      <Result
        status="404"
        title="股票不存在"
        subTitle={`未找到代码 ${code} 对应的股票`}
        extra={
          <Button type="primary" onClick={() => navigate('/stocks')}>
            返回股票池
          </Button>
        }
      />
    )
  }

  if (error || !detail) {
    return (
      <Card>
        <Alert
          type="error"
          message="加载失败"
          description={error || '未获取到股票数据'}
          action={
            <Button size="small" onClick={() => setRetryTick((t) => t + 1)}>
              重试
            </Button>
          }
        />
      </Card>
    )
  }

  const bar = detail.latest_bar
  const summary = detail.financial_summary

  return (
    <div className="page-container">
      <div style={{ marginBottom: 16 }}>
        <Button type="text" icon={<ArrowLeftOutlined />} onClick={() => navigate('/stocks')} style={{ marginRight: 8 }}>
          返回股票池
        </Button>
        <span style={{ fontSize: 20, fontWeight: 600 }}>
          {detail.code} · {detail.name || '--'}
        </span>
        <Tag color={MARKET_LABEL[detail.market] ? 'blue' : 'default'} style={{ marginLeft: 12, borderRadius: 6 }}>
          {MARKET_LABEL[detail.market] ?? detail.market}
        </Tag>
        <Tag style={{ borderRadius: 6 }}>{universeLabel(detail.universe)}</Tag>
        <Tag style={{ borderRadius: 6 }}>{detail.status || '--'}</Tag>
      </div>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={16}>
          <Card title="基本信息" size="small">
            <Descriptions column={2} size="small">
              <Descriptions.Item label="代码">{detail.code}</Descriptions.Item>
              <Descriptions.Item label="名称">{detail.name || '--'}</Descriptions.Item>
              <Descriptions.Item label="市场">{MARKET_LABEL[detail.market] ?? detail.market}</Descriptions.Item>
              <Descriptions.Item label="行业">{detail.industry || '--'}</Descriptions.Item>
              <Descriptions.Item label="上市日期">{detail.list_date || '--'}</Descriptions.Item>
              <Descriptions.Item label="股票池">{universeLabel(detail.universe)}</Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card title="最新行情" size="small">
            {bar ? (
              <Row gutter={16}>
                <Col span={24} style={{ marginBottom: 12 }}>
                  <Statistic title={`收盘价（${bar.date}）`} value={bar.close} precision={2} />
                </Col>
                <Col span={8}>
                  <Statistic title="开盘" value={bar.open} precision={2} valueStyle={{ fontSize: 15 }} />
                </Col>
                <Col span={8}>
                  <Statistic title="最高" value={bar.high} precision={2} valueStyle={{ fontSize: 15 }} />
                </Col>
                <Col span={8}>
                  <Statistic title="最低" value={bar.low} precision={2} valueStyle={{ fontSize: 15 }} />
                </Col>
                <Col span={12} style={{ marginTop: 12 }}>
                  <Statistic title="成交量（手）" value={formatWanYi(bar.volume)} valueStyle={{ fontSize: 15 }} />
                </Col>
                <Col span={12} style={{ marginTop: 12 }}>
                  <Statistic title="成交额" value={formatWanYi(bar.amount)} valueStyle={{ fontSize: 15 }} />
                </Col>
              </Row>
            ) : (
              <Empty description="行情数据待回填" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </Card>
        </Col>
      </Row>

      <Card title="日K线" size="small" style={{ marginTop: 16 }}>
        <KLineChart code={detail.code} />
      </Card>

      <Card title="财务摘要" size="small" style={{ marginTop: 16 }}>
        {summary ? (
          <>
            <div style={{ color: '#8a97ab', fontSize: 12, marginBottom: 12 }}>
              报告期 {summary.report_date || '--'} · 公告日 {summary.announce_date || '--'}（百分数均为原始值，0 或缺失显示 --）
            </div>
            <Row gutter={[16, 16]}>
              {SUMMARY_ITEMS.map((it) => (
                <Col xs={12} sm={8} md={6} key={it.key}>
                  <Statistic title={it.label} value={it.fmt(summary[it.key])} valueStyle={{ fontSize: 18 }} />
                </Col>
              ))}
            </Row>
          </>
        ) : (
          <Empty description="暂无财务数据，待数据回填" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        )}
      </Card>

      <Card title="财务明细（近 20 期）" size="small" style={{ marginTop: 16 }}>
        <Table<FinancialItem>
          rowKey="report_date"
          columns={FINANCIAL_COLUMNS}
          dataSource={financial}
          size="small"
          pagination={{ pageSize: 10, showSizeChanger: false, showTotal: (t) => `共 ${t} 期` }}
          locale={{ emptyText: '暂无财务数据' }}
        />
      </Card>
    </div>
  )
}
