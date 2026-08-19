import { Card, Col, Empty, Progress, Row } from 'antd'
import { ExperimentOutlined } from '@ant-design/icons'

const FACTOR_WEIGHTS = [
  { name: '趋势（ma_trend + macd_signal）', weight: 40, color: '#1d39c4' },
  { name: '价值（pe_ratio + pb_ratio）', weight: 30, color: '#3ecf7a' },
  { name: '质量（roe_quality）', weight: 20, color: '#f5a623' },
  { name: '风险（debt_risk）', weight: 10, color: '#8b5cf6' },
]

// TODO(Sprint 4/6): 当前策略、因子权重、历史表现
export default function StrategyPage() {
  return (
    <div className="page-container">
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={10}>
          <Card title="多因子权重" extra={<ExperimentOutlined />}>
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
              调仓规则：每日收盘后按因子评分横截面排名，rank ≤ 15 买入、rank &gt; 30 卖出，单票上限 20%（Sprint 4 实现）
            </div>
          </Card>
        </Col>
        <Col xs={24} lg={14}>
          <Card title="策略信号">
            <Empty description="因子计算与调仓信号，待 Sprint 4 实现" style={{ padding: '48px 0' }} />
          </Card>
        </Col>
      </Row>
    </div>
  )
}
