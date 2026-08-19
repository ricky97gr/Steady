import { useEffect, useMemo, useState } from 'react'
import { Card, Input, Select, Space, Table, Tag } from 'antd'
import type { ColumnsType } from 'antd/es/table'
import { Link } from 'react-router-dom'
import { SearchOutlined } from '@ant-design/icons'

import { getStocks } from '../../services/api'
import type { Market, SortField, StockBasic, StockListQuery, Universe } from '../../types'
import { tablePagination } from '../../utils/table'

const MARKET_META: Record<Market, { label: string; color: string }> = {
  SH: { label: '沪市', color: 'blue' },
  SZ: { label: '深市', color: 'purple' },
  BJ: { label: '北交所', color: 'orange' },
}

const MARKET_OPTIONS = Object.entries(MARKET_META).map(([value, meta]) => ({ value, label: meta.label }))

const UNIVERSE_OPTIONS: { value: Universe; label: string }[] = [
  { value: 'hs300', label: '沪深300' },
  { value: 'zz500', label: '中证500' },
]

function universeTag(u: string) {
  if (u === 'hs300') return <Tag color="blue" style={{ borderRadius: 6 }}>沪深300</Tag>
  if (u === 'zz500') return <Tag color="purple" style={{ borderRadius: 6 }}>中证500</Tag>
  return <Tag style={{ borderRadius: 6 }}>候选</Tag>
}

// TODO(Sprint 4): 接入评分/排名/推荐理由列
export default function StockPoolPage() {
  const [items, setItems] = useState<StockBasic[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [query, setQuery] = useState<StockListQuery>({ page: 1, page_size: 20 })
  const [keywordInput, setKeywordInput] = useState('') // 输入框中间态，与 query 分离做防抖

  // 服务端搜索：300ms 防抖，变化后回到第 1 页
  useEffect(() => {
    const t = setTimeout(() => {
      const kw = keywordInput.trim()
      setQuery((q) => ({ ...q, keyword: kw || undefined, page: 1 }))
    }, 300)
    return () => clearTimeout(t)
  }, [keywordInput])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    getStocks(query)
      .then((data) => {
        if (cancelled) return
        setItems(data.items)
        setTotal(data.total)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [query])

  const sortOrder = (field: SortField): 'ascend' | 'descend' | null =>
    query.sort === field ? (query.order === 'asc' ? 'ascend' : 'descend') : null

  const columns = useMemo<ColumnsType<StockBasic>>(
    () => [
      {
        title: '代码',
        dataIndex: 'code',
        width: 110,
        sorter: true,
        sortOrder: sortOrder('code'),
        render: (code: string) => <Link to={`/stocks/${code}`}>{code}</Link>,
      },
      {
        title: '名称',
        dataIndex: 'name',
        width: 160,
        sorter: true,
        sortOrder: sortOrder('name'),
        render: (n: string) => <span style={{ fontWeight: 500 }}>{n ?? '--'}</span>,
      },
      {
        title: '市场',
        dataIndex: 'market',
        width: 100,
        sorter: true,
        sortOrder: sortOrder('market'),
        render: (m: Market) => (m ? <Tag color={MARKET_META[m]?.color ?? 'default'} style={{ borderRadius: 6 }}>{MARKET_META[m]?.label ?? m}</Tag> : '--'),
      },
      {
        title: '行业',
        dataIndex: 'industry',
        sorter: true,
        sortOrder: sortOrder('industry'),
        render: (i: string) => i ?? '--',
      },
      {
        title: '上市日期',
        dataIndex: 'list_date',
        width: 120,
        sorter: true,
        sortOrder: sortOrder('list_date'),
        render: (d: string) => d || '--',
      },
      {
        title: '股票池',
        dataIndex: 'universe',
        width: 100,
        render: universeTag,
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [query.sort, query.order],
  )

  return (
    <Card
      title="股票池"
      extra={
        <Space wrap>
          <Input
            allowClear
            prefix={<SearchOutlined style={{ color: '#a0aec0' }} />}
            placeholder="搜索代码 / 名称"
            style={{ width: 200 }}
            value={keywordInput}
            onChange={(e) => setKeywordInput(e.target.value)}
          />
          <Select
            allowClear
            placeholder="市场"
            style={{ width: 110 }}
            options={MARKET_OPTIONS}
            value={query.market}
            onChange={(v: Market | undefined) => setQuery((q) => ({ ...q, market: v, page: 1 }))}
          />
          <Select
            allowClear
            placeholder="股票池"
            style={{ width: 120 }}
            options={UNIVERSE_OPTIONS}
            value={query.universe}
            onChange={(v: Universe | undefined) => setQuery((q) => ({ ...q, universe: v, page: 1 }))}
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
        dataSource={items}
        pagination={tablePagination({ current: query.page, pageSize: query.page_size, total })}
        onChange={(pagination, _filters, sorter) => {
          const s = Array.isArray(sorter) ? sorter[0] : sorter
          setQuery((q) => ({
            ...q,
            page: pagination.current ?? 1,
            page_size: pagination.pageSize ?? 20,
            sort: s?.order ? (s.columnKey as SortField) : undefined,
            order: s?.order === 'ascend' ? 'asc' : s?.order === 'descend' ? 'desc' : undefined,
          }))
        }}
        locale={{ emptyText: '未找到匹配的股票' }}
      />
    </Card>
  )
}
