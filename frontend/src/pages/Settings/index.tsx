import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Col,
  Empty,
  Form,
  Input,
  InputNumber,
  Row,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  TimePicker,
  Typography,
  message,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import {
  BellOutlined,
  RobotOutlined,
  SaveOutlined,
  SendOutlined,
} from '@ant-design/icons'
import dayjs from 'dayjs'

import {
  getNotifyConfig,
  getTaskRuns,
  sendNotifyTest,
  updateFeishuConfig,
  updateNotifyEvent,
} from '../../services/api'
import type { FeishuConfig, NotifyEvent, TaskRunItem } from '../../types'
import { tablePagination } from '../../utils/table'

// 调度方式：weekday=每周几 / trading_day=交易日 / event=事件触发（由业务直接推送）
const SCHEDULE_OPTIONS = [
  { value: 'trading_day', label: '交易日' },
  { value: 'weekday', label: '每周几' },
  { value: 'event', label: '事件触发' },
]

const WEEKDAY_OPTIONS = [
  { value: '1', label: '周一' },
  { value: '2', label: '周二' },
  { value: '3', label: '周三' },
  { value: '4', label: '周四' },
  { value: '5', label: '周五' },
  { value: '6', label: '周六' },
  { value: '7', label: '周日' },
]

const TEMPLATE_OPTIONS = [
  { value: 'blue', label: '蓝' },
  { value: 'green', label: '绿' },
  { value: 'red', label: '红' },
]

// 任务执行记录 → 中文名（未收录的任务原样展示）
const TASK_LABELS: Record<string, string> = {
  calc_factors: '因子计算',
  generate_signals: '策略信号',
  consume_backtests: '回测任务',
  auto_trade: '模拟交易',
  nav_snapshot: '净值快照',
  daily_report: '每日日报',
}

const taskLabel = (name: string) => TASK_LABELS[name] ?? name

const runStatusTag = (s: TaskRunItem['status']) => {
  const map: Record<TaskRunItem['status'], [string, string]> = {
    success: ['green', '成功'],
    skipped: ['orange', '跳过'],
    failed: ['red', '失败'],
  }
  const [color, text] = map[s] ?? ['default', s]
  return <Tag color={color} style={{ borderRadius: 6 }}>{text}</Tag>
}

const fieldCaption = (text: string) => (
  <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 4 }}>
    {text}
  </Typography.Text>
)

export default function SettingsPage() {
  const [feishu, setFeishu] = useState<FeishuConfig | null>(null)
  const [events, setEvents] = useState<NotifyEvent[]>([])
  const [runs, setRuns] = useState<TaskRunItem[]>([])
  const [loading, setLoading] = useState(true)
  const [savingFeishu, setSavingFeishu] = useState(false)
  const [testing, setTesting] = useState(false)
  const [savingKey, setSavingKey] = useState<string | null>(null)

  const load = () => {
    setLoading(true)
    Promise.all([getNotifyConfig(), getTaskRuns(50)])
      .then(([cfg, runsData]) => {
        setFeishu(cfg.feishu)
        setEvents(cfg.events)
        setRuns(runsData.items)
      })
      .catch(() => {
        // 错误已由 axios 拦截器统一弹出，保留空态供重试
      })
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const patchEvent = (eventKey: string, patch: Partial<NotifyEvent>) =>
    setEvents((prev) => prev.map((e) => (e.event_key === eventKey ? { ...e, ...patch } : e)))

  const onSaveFeishu = async () => {
    if (!feishu) return
    setSavingFeishu(true)
    try {
      await updateFeishuConfig(feishu)
      message.success('飞书配置已保存')
    } catch {
      // 拦截器已提示
    } finally {
      setSavingFeishu(false)
    }
  }

  const onTest = async () => {
    setTesting(true)
    try {
      await sendNotifyTest()
      message.success('测试卡片已发送，请查看飞书群')
    } catch {
      // 拦截器已提示
    } finally {
      setTesting(false)
    }
  }

  const onSaveEvent = async (ev: NotifyEvent) => {
    setSavingKey(ev.event_key)
    try {
      await updateNotifyEvent(ev.event_key, ev)
      message.success(`${ev.name}配置已保存`)
    } catch {
      // 拦截器已提示
    } finally {
      setSavingKey(null)
    }
  }

  const runColumns = useMemo<ColumnsType<TaskRunItem>>(
    () => [
      { title: '交易日', dataIndex: 'run_date', width: 110 },
      { title: '任务', dataIndex: 'task_name', width: 130, render: (v: string) => taskLabel(v) },
      { title: '状态', dataIndex: 'status', width: 80, render: (v: TaskRunItem['status']) => runStatusTag(v) },
      { title: '消息', dataIndex: 'message', ellipsis: true },
      { title: '记录时间', dataIndex: 'created_at', width: 170 },
    ],
    [],
  )

  return (
    <div className="page-container">
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={14}>
          <Card
            title="飞书机器人"
            extra={
              <Space>
                <Button icon={<SendOutlined />} loading={testing} onClick={onTest}>
                  发送测试卡片
                </Button>
                <Button type="primary" icon={<SaveOutlined />} loading={savingFeishu} onClick={onSaveFeishu}>
                  保存
                </Button>
              </Space>
            }
          >
            <Form layout="vertical">
              <Form.Item label="启用飞书通知">
                <Switch
                  checked={feishu?.enabled ?? false}
                  onChange={(v) => feishu && setFeishu({ ...feishu, enabled: v })}
                />
              </Form.Item>
              <Form.Item label="通知时 @所有人" extra="开启后所有通知卡片都会 @ 群内所有人（个人通知群推荐开启）">
                <Switch
                  checked={feishu?.at_all ?? false}
                  onChange={(v) => feishu && setFeishu({ ...feishu, at_all: v })}
                />
              </Form.Item>
              <Form.Item label="Webhook URL" extra="飞书群机器人 webhook，形如 https://open.feishu.cn/open-apis/bot/v2/hook/xxx">
                <Input
                  value={feishu?.webhook_url ?? ''}
                  onChange={(e) => feishu && setFeishu({ ...feishu, webhook_url: e.target.value })}
                  placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..."
                />
              </Form.Item>
              <Form.Item label="签名密钥（Secret）" extra="机器人在飞书开启「签名校验」后必填；留空则请求不签名">
                <Input.Password
                  value={feishu?.secret ?? ''}
                  onChange={(e) => feishu && setFeishu({ ...feishu, secret: e.target.value })}
                  placeholder="机器人签名校验密钥（可选）"
                  autoComplete="off"
                />
              </Form.Item>
              <Form.Item label="Dashboard 地址" extra="卡片内跳转链接 base，留空默认 http://localhost">
                <Input
                  value={feishu?.dashboard_url ?? ''}
                  onChange={(e) => feishu && setFeishu({ ...feishu, dashboard_url: e.target.value })}
                  placeholder="http://localhost"
                />
              </Form.Item>
              <Row gutter={16}>
                <Col span={12}>
                  <Form.Item label="请求超时（秒）">
                    <InputNumber
                      min={1}
                      max={60}
                      value={feishu?.timeout ?? 10}
                      onChange={(v) => feishu && setFeishu({ ...feishu, timeout: v ?? 10 })}
                      style={{ width: '100%' }}
                    />
                  </Form.Item>
                </Col>
                <Col span={12}>
                  <Form.Item label="失败重试（次）">
                    <InputNumber
                      min={0}
                      max={5}
                      value={feishu?.max_retries ?? 0}
                      onChange={(v) => feishu && setFeishu({ ...feishu, max_retries: v ?? 0 })}
                      style={{ width: '100%' }}
                    />
                  </Form.Item>
                </Col>
              </Row>
            </Form>
          </Card>
        </Col>
        <Col xs={24} lg={10}>
          <Card title={<Space><RobotOutlined />大模型能力（预留）</Space>}>
            <Alert
              type="info"
              showIcon
              message="后续版本接入大模型"
              description="届时可在此配置 provider / model / api_key / base_url，用于日报、信号点评等内容的 AI 生成。当前版本推送固定内容。"
            />
          </Card>
        </Col>
      </Row>

      <div style={{ height: 16 }} />

      <Card
        title={<Space><BellOutlined />通知事件</Space>}
        extra={<Typography.Text type="secondary" style={{ fontSize: 12 }}>每个事件可独立配置开关 / 调度方式 / 发送时刻</Typography.Text>}
      >
        {events.length === 0 ? (
          <Empty description="暂无通知事件配置" style={{ padding: '24px 0' }} />
        ) : (
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            {events.map((ev) => (
              <div
                key={ev.event_key}
                style={{
                  border: '1px solid #e6ecf5',
                  borderRadius: 12,
                  padding: '14px 16px',
                  background: '#fbfcff',
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                  <Space>
                    <Typography.Text strong>{ev.name}</Typography.Text>
                    <Tag style={{ borderRadius: 6 }}>{ev.event_key}</Tag>
                  </Space>
                  <Space size={12}>
                    <Switch checked={ev.enabled} onChange={(v) => patchEvent(ev.event_key, { enabled: v })} />
                    <Typography.Text type={ev.enabled ? 'success' : 'secondary'} style={{ fontSize: 12 }}>
                      {ev.enabled ? '已启用' : '已停用'}
                    </Typography.Text>
                    <Button
                      size="small"
                      type="primary"
                      icon={<SaveOutlined />}
                      loading={savingKey === ev.event_key}
                      onClick={() => onSaveEvent(ev)}
                    >
                      保存
                    </Button>
                  </Space>
                </div>
                <Row gutter={16}>
                  <Col xs={24} sm={8}>
                    {fieldCaption('调度方式')}
                    <Select
                      style={{ width: '100%' }}
                      value={ev.schedule_type}
                      options={SCHEDULE_OPTIONS}
                      onChange={(v: NotifyEvent['schedule_type']) =>
                        patchEvent(
                          ev.event_key,
                          v === 'event'
                            ? { schedule_type: v, send_at: null, weekdays: '' }
                            : { schedule_type: v },
                        )
                      }
                    />
                  </Col>
                  {ev.schedule_type === 'weekday' && (
                    <Col xs={24} sm={8}>
                      {fieldCaption('每周几（可多选）')}
                      <Select
                        mode="multiple"
                        style={{ width: '100%' }}
                        value={ev.weekdays ? ev.weekdays.split(',') : []}
                        options={WEEKDAY_OPTIONS}
                        onChange={(vals: string[]) => patchEvent(ev.event_key, { weekdays: vals.join(',') })}
                        placeholder="选择星期"
                      />
                    </Col>
                  )}
                  {ev.schedule_type !== 'event' && (
                    <Col xs={12} sm={6}>
                      {fieldCaption('发送时刻')}
                      <TimePicker
                        format="HH:mm"
                        style={{ width: '100%' }}
                        value={ev.send_at ? dayjs(ev.send_at, 'HH:mm') : null}
                        onChange={(t) => patchEvent(ev.event_key, { send_at: t ? t.format('HH:mm') : null })}
                        placeholder="19:30"
                      />
                    </Col>
                  )}
                  <Col xs={ev.schedule_type === 'weekday' ? 12 : ev.schedule_type === 'event' ? 16 : 10} sm={4}>
                    {fieldCaption('卡片模板')}
                    <Select
                      style={{ width: '100%' }}
                      value={ev.template}
                      options={TEMPLATE_OPTIONS}
                      onChange={(v) => patchEvent(ev.event_key, { template: v })}
                    />
                  </Col>
                </Row>
              </div>
            ))}
          </Space>
        )}
      </Card>

      <div style={{ height: 16 }} />

      <Card title="任务执行记录" extra={<Typography.Text type="secondary" style={{ fontSize: 12 }}>每日任务对账台账：该做没做、是否失败</Typography.Text>}>
        <Table<TaskRunItem>
          rowKey="id"
          columns={runColumns}
          dataSource={runs}
          loading={loading}
          size="small"
          pagination={tablePagination()}
          expandable={{
            rowExpandable: (r) => !!r.detail,
            expandedRowRender: (r) =>
              r.detail ? (
                <pre style={{ margin: 0, fontSize: 12, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                  {JSON.stringify(r.detail, null, 2)}
                </pre>
              ) : (
                <Typography.Text type="secondary">无明细</Typography.Text>
              ),
          }}
          locale={{ emptyText: '暂无任务执行记录' }}
        />
      </Card>
    </div>
  )
}
