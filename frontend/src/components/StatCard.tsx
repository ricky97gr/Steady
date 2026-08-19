import { Card, Statistic } from 'antd'
import type { ReactNode } from 'react'

interface StatCardProps {
  title: string
  value: number | string
  suffix?: string
  precision?: number
  icon: ReactNode
  color: 'blue' | 'green' | 'orange' | 'purple'
  footer?: string
}

/** 渐变统计卡片（Dashboard 复用） */
export default function StatCard({ title, value, suffix, precision, icon, color, footer }: StatCardProps) {
  return (
    <Card className={`stat-card ${color}`} styles={{ body: { padding: '20px 24px' } }}>
      <div className="stat-icon">{icon}</div>
      <Statistic title={title} value={value} suffix={suffix} precision={precision} />
      {footer && <div className="stat-footer">{footer}</div>}
    </Card>
  )
}
