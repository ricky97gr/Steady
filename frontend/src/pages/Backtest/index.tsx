import { useEffect, useMemo, useState } from 'react'
import {
  Button,
  Card,
  Col,
  DatePicker,
  Empty,
  Form,
  InputNumber,
  Row,
  Statistic,
  Table,
  Tag,
  message,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  AccountBookOutlined,
  FallOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons'
import * as echarts from 'echarts'
import type { EChartsOption } from 'echarts'
import dayjs from 'dayjs'

import StatCard from '../../components/StatCard'
import { useEChart } from '../../hooks/useEChart'
import { getBacktest, getBacktests, submitBacktest } from '../../services/api'
import type { BacktestJobItem, BacktestNavItem } from '../../types'

const { RangePicker } = DatePicker

// A 股红涨绿跌
const UP = '#cf1322'
const DOWN = '#3f8600'

const statusTag = (s: BacktestJobItem['status']) => {
  const map: Record<BacktestJobItem['status'], [string, string]> = {
    pending: ['blue', '排队中'],
    running: ['cyan', '回测中'],
    done: ['green', '已完成'],
    failed: ['red', '失败'],
  }
  const [color, text] = map[s] ?? ['default', s]
  return <Tag color={color} style={{ borderRadius: 6 }}>{text}</Tag>
}

const pctColor = (v: number) => (v >= 0 ? UP : DOWN)

/** 指标卡：红涨绿跌（返回一个 Statistic，供 Col 包裹） */
function MetricStat({ title, value, precision = 2, suffix = '%' }: { title: string; value: number; precision?: number; suffix?: string }) {
  return (
    <Statistic title={title} value={value} precision={precision} suffix={suffix} valueStyle={{ color: pctColor(value), fontWeight: 600 }} />
  )
}

/** 收益曲线：策略净值 vs 沪深300（同起点归一化，benchmark 缺失日断线） */
function NavChart({ items }: { items: BacktestNavItem[] }) {
  const option = useMemo<EChartsOption | null>(() => {
    if (!items.length) return null
    const dates = items.map((it) => it.date.slice(5)) // YYYY-MM-DD → MM-DD
    const base = items[0].nav
    const benchBase = items.find((it) => it.benchmark != null)?.benchmark
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
          data: items.map((it) => it.nav / base),
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
          data: items.map((it) =>
            it.benchmark != null && benchBase ? it.benchmark / benchBase : null,
          ),
          smooth: true,
          symbol: 'none',
          lineStyle: { width: 2, color: '#f5a623', type: 'dashed' },
        },
      ],
    }
  }, [items])
  const domRef = useEChart(option, [items])
  return <div ref={domRef} style={{ height: 320 }} />
}

export default function BacktestPage() {
  const [form] = Form.useForm<{ range: [dayjs.Dayjs, dayjs.Dayjs]; topN: number }>()
  const [jobs, setJobs] = useState<BacktestJobItem[]>([])
  const [selected, setSelected] = useState<BacktestJobItem | null>(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [reloadTick, setReloadTick] = useState(0)

  // 加载列表 + 选中任务详情（任务未终结时由轮询接管）
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    Promise.all([
      getBacktests(20),
      selected?.id ? getBacktest(selected.id).catch(() => null) : Promise.resolve(null),
    ])
      .then(([list, detail]) => {
        if (cancelled) return
        setJobs(list.items)
        if (detail) setSelected(detail)
      })
      .catch(() => undefined)
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [reloadTick, selected?.id])

  // 轮询：存在 pending/running 任务时每 5s 刷新；全部终结后停止
  useEffect(() => {
    const active = jobs.some((j) => j.status === 'pending' || j.status === 'running')
    if (!active) return
    const timer = setInterval(() => {
      if (!document.hidden) setReloadTick((t) => t + 1)
    }, 5000)
    return () => clearInterval(timer)
  }, [jobs])

  const onSubmit = async () => {
    const values = await form.validateFields()
    const [start, end] = values.range
    setSubmitting(true)
    try {
      const res = await submitBacktest({
        start_date: start.format('YYYY-MM-DD'),
        end_date: end.format('YYYY-MM-DD'),
        top_n: values.topN ?? 20,
      })
      message.success(`回测任务 #${res.job_id} 已提交，引擎每 5 分钟执行一次`)
      form.resetFields()
      setReloadTick((t) => t + 1)
    } catch {
      // 错误提示已由 axios 拦截器统一弹出
    } finally {
      setSubmitting(false)
    }
  }

  const columns = useMemo<ColumnsType<BacktestJobItem>>(
    () => [
      { title: 'ID', dataIndex: 'id', width: 60 },
      { title: '区间', key: 'range', width: 180, render: (_, j) => `${j.start_date} ~ ${j.end_date}` },
      { title: 'TopN', dataIndex: 'top_n', width: 70, align: 'right' },
      {
        title: '总收益',
        dataIndex: 'total_return',
        width: 100,
        align: 'right',
        render: (v: number, j) =>
          j.status === 'done' ? (
            <span style={{ color: pctColor(v) }}>{(v * 100).toFixed(2)}%</span>
          ) : (
            '--'
          ),
      },
      {
        title: '最大回撤',
        dataIndex: 'max_drawdown',
        width: 100,
        align: 'right',
        render: (v: number, j) => (j.status === 'done' ? `${(v * 100).toFixed(2)}%` : '--'),
      },
      {
        title: '年化',
        dataIndex: 'annualized_return',
        width: 100,
        align: 'right',
        render: (v: number, j) =>
          j.status === 'done' ? (
            <span style={{ color: pctColor(v) }}>{(v * 100).toFixed(2)}%</span>
          ) : (
            '--'
          ),
      },
      { title: '状态', dataIndex: 'status', width: 90, render: statusTag },
      {
        title: '创建时间',
        dataIndex: 'created_at',
        width: 110,
        render: (v: string) => v.slice(5), // YYYY-MM-DD → MM-DD
      },
    ],
    [],
  )

  const r = selected
  const nav = r?.nav ?? []

  return (
    <div className="page-container">
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={8}>
          <Card title="新建回测" styles={{ body: { padding: 16 } }}>
            <Form form={form} layout="vertical" initialValues={{ topN: 20 }}>
              <Form.Item
                name="range"
                label="回测区间"
                rules={[{ required: true, message: '请选择回测区间' }]}
              >
                <RangePicker style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item name="topN" label="目标持仓数（TopN）">
                <InputNumber min={1} max={50} style={{ width: '100%' }} />
              </Form.Item>
              <Button type="primary" block loading={submitting} onClick={onSubmit}>
                提交回测
              </Button>
              <div style={{ color: '#8a97ab', fontSize: 12, marginTop: 10 }}>
                ※ 引擎每 5 分钟执行一次任务；同参数重复提交不重复执行
              </div>
            </Form>
          </Card>
        </Col>
        <Col xs={24} lg={16}>
          <Card
            title="回测任务"
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
            styles={{ body: { paddingTop: 8 } }}
          >
            <Table<BacktestJobItem>
              rowKey="id"
              columns={columns}
              dataSource={jobs}
              loading={loading}
              size="small"
              pagination={false}
              onRow={(j) => ({
                onClick: () => setSelected(j),
                style: { cursor: 'pointer' },
              })}
              locale={{ emptyText: '暂无回测任务，左侧新建一个试试' }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col span={24}>
          <Card
            title={
              r ? `回测详情 #${r.id}（${r.start_date} ~ ${r.end_date}，Top${r.top_n}，${r.strategy_name}）` : '回测详情'
            }
          >
            {!r || r.status !== 'done' ? (
              <Empty
                description={
                  !r
                    ? '选择左侧任务查看详情'
                    : r.status === 'failed'
                      ? `回测失败：${r.error || '未知错误'}`
                      : `任务${r.status === 'pending' ? '排队中' : '回测中'}，结果生成后自动展示`
                }
                style={{ padding: '40px 0' }}
              />
            ) : (
              <>
                <Row gutter={[16, 16]}>
                  <Col xs={12} lg={4}>
                    <Card styles={{ body: { padding: '20px 24px' } }}>
                      <MetricStat title="总收益" value={r.total_return * 100} />
                      <div style={{ marginTop: 8, color: '#8a97ab', fontSize: 12 }}>
                        {r.trading_days} 个交易日
                      </div>
                    </Card>
                  </Col>
                  <Col xs={12} lg={4}>
                    <Card styles={{ body: { padding: '20px 24px' } }}>
                      <MetricStat title="年化收益" value={r.annualized_return * 100} />
                      <div style={{ marginTop: 8, color: '#8a97ab', fontSize: 12 }}>
                        基准 <span style={{ color: pctColor(r.benchmark_return) }}>{(r.benchmark_return * 100).toFixed(2)}%</span>
                      </div>
                    </Card>
                  </Col>
                  <Col xs={12} lg={4}>
                    <StatCard
                      title="最大回撤"
                      value={r.max_drawdown * 100}
                      precision={2}
                      suffix="%"
                      icon={<FallOutlined />}
                      color="orange"
                    />
                  </Col>
                  <Col xs={12} lg={4}>
                    <StatCard
                      title="夏普比率"
                      value={r.sharpe}
                      precision={2}
                      icon={<ThunderboltOutlined />}
                      color="purple"
                    />
                  </Col>
                  <Col xs={12} lg={4}>
                    <Card styles={{ body: { padding: '20px 24px' } }}>
                      <MetricStat title="超额收益" value={r.excess_return * 100} />
                      <div style={{ marginTop: 8, color: '#8a97ab', fontSize: 12 }}>
                        相对沪深300
                      </div>
                    </Card>
                  </Col>
                  <Col xs={12} lg={4}>
                    <StatCard
                      title="期末净值"
                      value={r.final_value}
                      precision={2}
                      suffix="元"
                      icon={<AccountBookOutlined />}
                      color="blue"
                      footer={`成交 ${r.trades} 笔 · 期末持仓 ${r.positions} 只`}
                    />
                  </Col>
                </Row>
                <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
                  <Col span={24}>
                    <Card size="small" title="策略净值 vs 沪深300（同起点归一化）">
                      {nav.length ? (
                        <NavChart items={nav} />
                      ) : (
                        <Empty description="净值序列为空" style={{ padding: '40px 0' }} />
                      )}
                    </Card>
                  </Col>
                </Row>
              </>
            )}
          </Card>
        </Col>
      </Row>
    </div>
  )
}
