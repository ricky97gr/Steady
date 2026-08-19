import { useEffect, useMemo, useState } from 'react'
import { Card, Input, Space, Table, Tag } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { SearchOutlined } from '@ant-design/icons'

import { getStocks } from '../../services/api'
import type { StockBasic } from '../../types'

const MARKET_COLORS: Record<string, string> = {
  沪市: 'blue',
  深市: 'purple',
  创业板: 'green',
  科创板: 'magenta',
  北交所: 'orange',
}

const columns: ColumnsType<StockBasic> = [
  { title: '代码', dataIndex: 'code', width: 100 },
  {
    title: '名称',
    dataIndex: 'name',
    width: 160,
    render: (n: string) => <span style={{ fontWeight: 500 }}>{n}</span>,
  },
  {
    title: '市场',
    dataIndex: 'market',
    width: 90,
    render: (m: string) => <Tag color={MARKET_COLORS[m] ?? 'default'}>{m ?? '--'}</Tag>,
  },
  { title: '行业', dataIndex: 'industry', render: (i: string) => i ?? '--' },
  {
    title: '股票池',
    dataIndex: 'universe',
    width: 140,
    render: (u: string) =>
      u ? <Tag color="blue" style={{ borderRadius: 6 }}>{u}</Tag> : <Tag style={{ borderRadius: 6 }}>候选</Tag>,
  },
]

// TODO(Sprint 3): 接入评分/排名/推荐理由列
export default function StockPoolPage() {
  const [items, setItems] = useState<StockBasic[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')

  useEffect(() => {
    setLoading(true)
    getStocks({ page, page_size: 20 })
      .then((data) => {
        setItems(data.items)
        setTotal(data.total)
      })
      .finally(() => setLoading(false))
  }, [page])

  // 本地过滤（后端模糊搜索 Sprint 3 接入）
  const filtered = useMemo(() => {
    if (!keyword.trim()) return items
    const k = keyword.trim().toLowerCase()
    return items.filter((s) => s.code.toLowerCase().includes(k) || s.name.toLowerCase().includes(k))
  }, [items, keyword])

  return (
    <Card
      title="股票池"
      extra={
        <Space>
          <Input
            allowClear
            prefix={<SearchOutlined style={{ color: '#a0aec0' }} />}
            placeholder="搜索代码 / 名称"
            style={{ width: 220 }}
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
          />
          <Tag color="blue" style={{ marginRight: 0, borderRadius: 6 }}>
            共 {total} 只
          </Tag>
        </Space>
      }
    >
      <Table<StockBasic>
        rowKey="code"
        loading={loading}
        columns={columns}
        dataSource={filtered}
        pagination={{
          current: page,
          total,
          pageSize: 20,
          showSizeChanger: false,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (p) => setPage(p),
        }}
        locale={{ emptyText: '股票池为空 — 待 Sprint 1 数据回填后展示' }}
      />
    </Card>
  )
}
