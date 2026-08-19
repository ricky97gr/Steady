import { Card, Col, Row, Statistic } from 'antd'
import { ArrowDownOutlined, ArrowUpOutlined } from '@ant-design/icons'

// TODO(Sprint 6): 从 /account、/account/nav 拉真实数据
export default function DashboardPage() {
  return (
    <Row gutter={[16, 16]}>
      <Col span={6}>
        <Card>
          <Statistic title="总资产" value={100000} precision={2} />
        </Card>
      </Col>
      <Col span={6}>
        <Card>
          <Statistic title="累计收益" value={0} precision={2} suffix="%" />
        </Card>
      </Col>
      <Col span={6}>
        <Card>
          <Statistic
            title="今日收益"
            value={0}
            precision={2}
            prefix={<ArrowUpOutlined />}
          />
        </Card>
      </Col>
      <Col span={6}>
        <Card>
          <Statistic
            title="最大回撤"
            value={0}
            precision={2}
            prefix={<ArrowDownOutlined />}
          />
        </Card>
      </Col>
      <Col span={24}>
        <Card title="收益曲线（对比沪深300）——待 Sprint 6 实现">
          {/* TODO(Sprint 6): ECharts 折线图，数据源 /account/nav */}
        </Card>
      </Col>
    </Row>
  )
}
