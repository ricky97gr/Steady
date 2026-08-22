import { useEffect, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Empty,
  Input,
  Skeleton,
  Space,
  Spin,
  Tabs,
  Typography,
} from 'antd'
import {
  BookOutlined,
  CommentOutlined,
  QuestionCircleOutlined,
  SendOutlined,
} from '@ant-design/icons'
import { useSearchParams } from 'react-router-dom'

import { askProject, explainTerm, getLLMConfig, interpretBrief } from '../../services/api'

const { TextArea } = Input
const { Paragraph } = Typography

// Markdown 代码块 → 代码段；行内 `code` → 高亮（问答/解读中 LLM 常给代码示例）
const mdCode = (s: string) =>
  s
    .split(/```(\w*)\n?/)
    .map((part, i) => (i % 2 === 1 ? <code key={i}>{part}</code> : <span key={i}>{part}</span>))

export default function LLMPage() {
  const [searchParams] = useSearchParams()
  const [enabled, setEnabled] = useState<boolean | null>(null) // null = 加载中
  const [active, setActive] = useState('interpret') // interpret/glossary/ask
  const [loading, setLoading] = useState(false)

  const [briefDate, setBriefDate] = useState(searchParams.get('date') ?? '')
  const [interpretation, setInterpretation] = useState('')
  const [term, setTerm] = useState('')
  const [termRes, setTermRes] = useState('')
  const [question, setQuestion] = useState('')
  const [askRes, setAskRes] = useState('')

  useEffect(() => {
    getLLMConfig()
      .then((cfg) => setEnabled(cfg.enabled))
      .catch(() => setEnabled(false))
  }, [])

  // 从早盘简报页跳入时，自动解读该日简报
  useEffect(() => {
    const d = searchParams.get('date')
    if (d) {
      setBriefDate(d)
      if (enabled) doInterpret(d)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, searchParams])

  const doInterpret = async (d: string) => {
    setLoading(true)
    setInterpretation('')
    try {
      const res = await interpretBrief(d || undefined)
      setInterpretation(res.interpretation)
    } catch {
      // 拦截器已提示
    } finally {
      setLoading(false)
    }
  }

  const doExplain = async () => {
    const t = term.trim()
    if (!t) return
    setLoading(true)
    setTermRes('')
    try {
      const res = await explainTerm(t)
      setTermRes(res.explanation)
    } catch {
      // 拦截器已提示
    } finally {
      setLoading(false)
    }
  }

  const doAsk = async () => {
    const q = question.trim()
    if (!q) return
    setLoading(true)
    setAskRes('')
    try {
      const res = await askProject(q)
      setAskRes(res.answer)
    } catch {
      // 拦截器已提示
    } finally {
      setLoading(false)
    }
  }

  if (enabled === null) {
    return (
      <Card>
        <Skeleton active />
      </Card>
    )
  }

  if (!enabled) {
    return (
      <Alert
        type="warning"
        showIcon
        message="大模型能力未启用"
        description={
          <Space direction="vertical">
            <span>前往「设置 → 大模型能力」填写 provider / model / api_key 并开启开关后，即可使用本页功能。</span>
            <span>启用后每天 09:20 会自动推送当日早盘简报的 AI 解读飞书卡片。</span>
          </Space>
        }
      />
    )
  }

  return (
    <div>
      <Alert
        type="info"
        showIcon
        message="AI 助手"
        description="术语解释与项目问答基于内置知识库（docs/llm），大模型不直接访问数据库。输入将发送至云端 API，请勿粘贴敏感信息。"
        style={{ marginBottom: 16 }}
      />
      <Tabs
        activeKey={active}
        onChange={setActive}
        items={[
          {
            key: 'interpret',
            label: (
              <span><CommentOutlined /> 简报解读</span>
            ),
            children: (
              <Card size="small">
                <Space direction="vertical" style={{ width: '100%' }} size="middle">
                  <Space.Compact style={{ width: '100%' }}>
                    <Input
                      value={briefDate}
                      onChange={(e) => setBriefDate(e.target.value)}
                      placeholder="早报日期 YYYY-MM-DD（留空 = 最近一份）"
                    />
                    <Button
                      type="primary"
                      icon={<SendOutlined />}
                      loading={loading}
                      onClick={() => doInterpret(briefDate.trim())}
                    >
                      解读
                    </Button>
                  </Space.Compact>
                  <Paragraph
                    style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}
                  >
                    {loading ? <Spin tip="正在生成解读…" /> : interpretation ? mdCode(interpretation) : <TextHint />}
                  </Paragraph>
                </Space>
              </Card>
            ),
          },
          {
            key: 'glossary',
            label: (
              <span><BookOutlined /> 术语解释</span>
            ),
            children: (
              <Card size="small">
                <Space direction="vertical" style={{ width: '100%' }} size="middle">
                  <Space.Compact style={{ width: '100%' }}>
                    <Input
                      value={term}
                      onChange={(e) => setTerm(e.target.value)}
                      placeholder="输入金融术语，如：夏普比率 / 换手率 / T+1"
                      onPressEnter={() => doExplain()}
                    />
                    <Button
                      type="primary"
                      icon={<SendOutlined />}
                      loading={loading}
                      onClick={doExplain}
                    >
                      解释
                    </Button>
                  </Space.Compact>
                  <Paragraph style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}>
                    {loading ? <Spin tip="正在解释…" /> : termRes ? mdCode(termRes) : <TextHint />}
                  </Paragraph>
                </Space>
              </Card>
            ),
          },
          {
            key: 'ask',
            label: (
              <span><QuestionCircleOutlined /> 项目问答</span>
            ),
            children: (
              <Card size="small">
                <Space direction="vertical" style={{ width: '100%' }} size="middle">
                  <TextArea
                    value={question}
                    onChange={(e) => setQuestion(e.target.value)}
                    placeholder="关于 Steady 系统的任何问题，如：数据流是怎样的？如何回测？"
                    autoSize={{ minRows: 2, maxRows: 4 }}
                    maxLength={2000}
                  />
                  <Button
                    type="primary"
                    icon={<SendOutlined />}
                    loading={loading}
                    onClick={doAsk}
                    style={{ alignSelf: 'flex-start' }}
                  >
                    提问
                  </Button>
                  <Paragraph style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}>
                    {loading ? <Spin tip="正在回答…" /> : askRes ? mdCode(askRes) : <TextHint />}
                  </Paragraph>
                </Space>
              </Card>
            ),
          },
        ]}
      />
    </div>
  )
}

const TextHint = () => (
  <Typography.Text type="secondary">
    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="输入内容后生成结果" />
  </Typography.Text>
)
