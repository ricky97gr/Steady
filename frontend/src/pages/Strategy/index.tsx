import { Card, Empty } from 'antd'

// TODO(Sprint 4/6): 当前策略、因子权重、历史表现
export default function StrategyPage() {
  return (
    <Card title="多因子策略（趋势40% + 价值30% + 质量20% + 风险10%）">
      <Empty description="策略信号与因子权重展示，待 Sprint 4 实现" />
    </Card>
  )
}
