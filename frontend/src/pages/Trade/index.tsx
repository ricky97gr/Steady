import { Card, Col, Empty, Row, Statistic, Tag } from 'antd'
import { SwapOutlined } from '@ant-design/icons'

// TODO(Sprint 5/6): 持仓、订单、成交记录、盈亏
export default function TradePage() {
  return (
    <div className="page-container">
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={8}>
          <Card>
            <Statistic title="持仓市值" value={0} precision={2} suffix="元" />
            <Tag color="blue" style={{ marginTop: 8, borderRadius: 6 }}>主账户 · 100,000</Tag>
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card>
            <Statistic title="可用资金" value={100000} precision={2} suffix="元" />
            <Tag color="green" style={{ marginTop: 8, borderRadius: 6 }}>满仓可用</Tag>
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card>
            <Statistic title="持仓标的" value={0} suffix="只" />
            <Tag color="orange" style={{ marginTop: 8, borderRadius: 6 }}>当日盈亏 0.00 元</Tag>
          </Card>
        </Col>
        <Col span={24}>
          <Card title="持仓与订单" extra={<SwapOutlined />}>
            <Empty description="持仓、订单与成交记录，待 Sprint 5 实现" style={{ padding: '64px 0' }} />
          </Card>
        </Col>
      </Row>
    </div>
  )
}
