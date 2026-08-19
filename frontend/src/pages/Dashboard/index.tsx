import { useEffect, useRef } from 'react'
import { Card, Col, Row } from 'antd'
import {
  AccountBookOutlined,
  LineChartOutlined,
  RiseOutlined,
  StockOutlined,
  TeamOutlined,
} from '@ant-design/icons'
import * as echarts from 'echarts'

import StatCard from '../../components/StatCard'

// TODO(Sprint 6): 以下均为占位数据，改为从 /account、/account/nav、/stocks、/factors 拉取
const MOCK_NAV = {
  strategy: [1.0, 1.012, 1.008, 1.021, 1.03, 1.026, 1.041, 1.055, 1.049, 1.062],
  benchmark: [1.0, 1.005, 1.002, 1.01, 1.008, 1.015, 1.012, 1.02, 1.024, 1.019],
}

function NavChart() {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const chart = echarts.init(ref.current!)
    chart.setOption({
      tooltip: { trigger: 'axis', valueFormatter: (v: number) => `${((v - 1) * 100).toFixed(2)}%` },
      legend: { data: ['策略净值', '沪深300'], top: 4, right: 12, icon: 'roundRect' },
      grid: { left: 16, right: 20, top: 44, bottom: 12, containLabel: true },
      xAxis: {
        type: 'category',
        data: ['06-01', '06-08', '06-15', '06-22', '06-29', '07-06', '07-13', '07-20', '07-27', '08-03'],
        axisLine: { lineStyle: { color: '#e6ecf5' } },
        axisTick: { show: false },
        axisLabel: { color: '#8a97ab' },
      },
      yAxis: {
        type: 'value',
        axisLabel: { color: '#8a97ab', formatter: (v: number) => `${(v - 1) * 100}%` },
        splitLine: { lineStyle: { color: '#eef2fa' } },
      },
      series: [
        {
          name: '策略净值',
          type: 'line',
          data: MOCK_NAV.strategy,
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
          data: MOCK_NAV.benchmark,
          smooth: true,
          symbol: 'none',
          lineStyle: { width: 2, color: '#f5a623', type: 'dashed' },
        },
      ],
    })
    const onResize = () => chart.resize()
    window.addEventListener('resize', onResize)
    return () => {
      window.removeEventListener('resize', onResize)
      chart.dispose()
    }
  }, [])

  return <div ref={ref} style={{ height: 320 }} />
}

export default function DashboardPage() {
  return (
    <div className="page-container">
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <StatCard
            title="账户总资产"
            value={100000}
            precision={2}
            icon={<AccountBookOutlined />}
            color="blue"
            footer="模拟账户 · 初始资金 100,000"
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <StatCard
            title="累计收益率"
            value={0}
            suffix="%"
            precision={2}
            icon={<RiseOutlined />}
            color="green"
            footer="对比沪深300：--"
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <StatCard
            title="股票池规模"
            value={800}
            icon={<StockOutlined />}
            color="orange"
            footer="沪深300 + 中证500（含ST剔除）"
          />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <StatCard
            title="监控因子"
            value={6}
            icon={<LineChartOutlined />}
            color="purple"
            footer="趋势 / 价值 / 质量 / 风险"
          />
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={16}>
          <Card
            title="策略收益曲线（对比沪深300）"
            styles={{ body: { padding: '8px 16px 16px' } }}
          >
            <NavChart />
            <div style={{ color: '#8a97ab', fontSize: 12 }}>
              ※ 占位数据，Sprint 6 接入真实净值
            </div>
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card title="策略速览" styles={{ body: { padding: 16 } }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {[
                { label: '当前策略', value: 'multi_factor 多因子', tag: '趋势40% + 价值30% + 质量20% + 风险10%' },
                { label: '调仓方式', value: '每日排名轮动', tag: 'top20 · 买入缓冲15 · 卖出缓冲30' },
                { label: '股票池', value: '沪深300 + 中证500', tag: '单票上限 20% · 等权' },
              ].map((row) => (
                <div
                  key={row.label}
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
                    <div style={{ fontSize: 13, color: '#8a97ab' }}>{row.label}</div>
                    <div style={{ fontWeight: 600 }}>{row.value}</div>
                  </div>
                  <div style={{ fontSize: 11, color: '#8a97ab', textAlign: 'right' }}>{row.tag}</div>
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
                display: 'flex',
                gap: 8,
                alignItems: 'center',
              }}
            >
              <TeamOutlined />
              模拟交易引擎已挂载 · 回测引擎就绪（Sprint 4 接入）
            </div>
          </Card>
        </Col>
      </Row>
    </div>
  )
}
