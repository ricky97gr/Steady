import { useEffect, useState } from 'react'
import { Table, Tag } from 'antd'
import type { ColumnsType } from 'antd/es/table'

import { getStocks } from '../../services/api'
import type { StockBasic } from '../../types'

const columns: ColumnsType<StockBasic> = [
  { title: '代码', dataIndex: 'code', width: 100 },
  { title: '名称', dataIndex: 'name', width: 140 },
  {
    title: '市场',
    dataIndex: 'market',
    width: 80,
    render: (m: string) => <Tag>{m}</Tag>,
  },
  { title: '行业', dataIndex: 'industry' },
  {
    title: '股票池',
    dataIndex: 'universe',
    width: 120,
    render: (u: string) => (u ? <Tag color="blue">{u}</Tag> : '--'),
  },
]

// TODO(Sprint 3): 接入评分/排名/推荐理由
export default function StockPoolPage() {
  const [items, setItems] = useState<StockBasic[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [page, setPage] = useState(1)

  useEffect(() => {
    setLoading(true)
    getStocks({ page, page_size: 20 })
      .then((data) => {
        setItems(data.items)
        setTotal(data.total)
      })
      .finally(() => setLoading(false))
  }, [page])

  return (
    <Table<StockBasic>
      rowKey="code"
      loading={loading}
      columns={columns}
      dataSource={items}
      pagination={{
        current: page,
        total,
        pageSize: 20,
        onChange: (p) => setPage(p),
      }}
    />
  )
}
