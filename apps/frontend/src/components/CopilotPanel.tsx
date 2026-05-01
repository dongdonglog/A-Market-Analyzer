import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert, Button, Card, Collapse, Flex, Input, Space, Spin, Tag, Typography } from 'antd'
import { CopyOutlined, ReloadOutlined } from '@ant-design/icons'
import dayjs, { type Dayjs } from 'dayjs'
import { useQuery } from '@tanstack/react-query'
import { fetchCopilotSessionMessages, fetchCopilotSessions, queryCopilotWithOptions, streamCopilot } from '../lib/copilotApi'
import { isUnauthorizedError } from '../lib/http'
import type { ChatMessage, CopilotResponse } from '../types/api'

const { Paragraph, Text } = Typography

interface CopilotPanelProps {
  symbol?: string
  range: [Dayjs | null, Dayjs | null] | null
  provider?: string
  providerApiKey?: string
  providerLabel?: string
  providerEnabled?: boolean
}

const stageMeta = [
  { key: 'loading_ohlc', label: 'K线' },
  { key: 'syncing_symbol', label: '同步行情' },
  { key: 'checking_allowance', label: '准备' },
  { key: 'loading_news', label: '新闻' },
  { key: 'generating_answer', label: '生成' },
  { key: 'consuming_allowance', label: '记录' },
  { key: 'saving_session', label: '保存' },
] as const

function tagColor(bias?: string) {
  switch (bias) {
    case 'bullish':
      return 'green'
    case 'bearish':
      return 'red'
    default:
      return 'gold'
  }
}

function isAbortError(error: unknown) {
  if (error instanceof DOMException) {
    return error.name === 'AbortError'
  }
  if (typeof error === 'object' && error !== null && 'name' in error) {
    return String(error.name) === 'AbortError' || String(error.name) === 'CanceledError'
  }
  return false
}

export function CopilotPanel({ symbol, range, provider, providerApiKey, providerLabel, providerEnabled = false }: CopilotPanelProps) {
  const [question, setQuestion] = useState('这段走势说明了什么，接下来要观察什么？')
  const [loading, setLoading] = useState(false)
  const [stageKey, setStageKey] = useState<string>()
  const [streamStage, setStreamStage] = useState<string>()
  const [error, setError] = useState<string>()
  const [result, setResult] = useState<CopilotResponse>()
  const [history, setHistory] = useState<ChatMessage[]>([])
  const [activeSessionId, setActiveSessionId] = useState<string>()
  const [copied, setCopied] = useState(false)
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const abortControllerRef = useRef<AbortController | null>(null)

  const sessionsQuery = useQuery({
    queryKey: ['copilot-sessions', symbol],
    enabled: Boolean(symbol),
    queryFn: () => fetchCopilotSessions(symbol!),
  })

  const sessionMessagesQuery = useQuery({
    queryKey: ['copilot-session-messages', activeSessionId],
    enabled: Boolean(activeSessionId),
    queryFn: () => fetchCopilotSessionMessages(activeSessionId!),
  })

  useEffect(() => {
    setHistory([])
    setResult(undefined)
    setError(undefined)
    setStageKey(undefined)
    setStreamStage(undefined)
    setActiveSessionId(undefined)
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
  }, [symbol])

  useEffect(() => () => {
    abortControllerRef.current?.abort()
    abortControllerRef.current = null
  }, [])

  useEffect(() => {
    if (!sessionsQuery.data?.length) {
      return
    }

    setActiveSessionId((current) => current ?? sessionsQuery.data[0].id)
  }, [sessionsQuery.data])

  useEffect(() => {
    if (!sessionMessagesQuery.data) {
      return
    }

    setHistory(sessionMessagesQuery.data.messages)
    setResult(undefined)
  }, [sessionMessagesQuery.data])

  useEffect(() => {
    if (!copied) {
      return
    }
    const timer = window.setTimeout(() => setCopied(false), 1400)
    return () => window.clearTimeout(timer)
  }, [copied])

  useEffect(() => {
    const node = scrollRef.current
    if (!node) {
      return
    }
    node.scrollTo({
      top: node.scrollHeight,
      behavior: 'smooth',
    })
  }, [history, loading, result])

  async function handleSubmit() {
    if (!symbol) {
      setError('先选择一个股票。')
      return
    }

    setLoading(true)
    setError(undefined)

    try {
      const nextQuestion = question.trim()
      if (!nextQuestion) {
        setError('请输入问题。')
        setLoading(false)
        return
      }

      const payload = {
        symbol,
        provider,
        provider_api_key: providerApiKey?.trim() || undefined,
        question: nextQuestion,
        start_date: range?.[0] ? dayjs(range[0]).format('YYYY-MM-DD') : undefined,
        end_date: range?.[1] ? dayjs(range[1]).format('YYYY-MM-DD') : undefined,
        history,
      }
      const controller = new AbortController()
      abortControllerRef.current = controller
      setStageKey(undefined)
      setStreamStage('正在建立流式连接')
      const nextHistory = [
        ...history,
        { role: 'user', content: nextQuestion },
        { role: 'assistant', content: '' },
      ]
      setHistory(nextHistory)
      setResult(undefined)

      let streamResult: CopilotResponse | undefined
      try {
        await streamCopilot(payload, {
          onStart: (streamStart) => {
            setStreamStage('已连接，准备返回结果')
            if (streamStart.session_id) {
              setActiveSessionId(streamStart.session_id)
            }
          },
          onStage: (stageEvent) => {
            setStageKey(stageEvent.stage)
            setStreamStage(stageEvent.message)
          },
          onDelta: (delta) => {
            setHistory((current) => {
              if (!current.length) {
                return current
              }
              const updated = [...current]
              const last = updated[updated.length - 1]
              updated[updated.length - 1] = {
                ...last,
                content: `${last.content}${delta.content}`,
              }
              return updated
            })
          },
          onError: (streamErrorPayload) => {
            throw new Error(streamErrorPayload.error || 'AI 分析失败')
          },
          onResult: (response) => {
            streamResult = response
            setStageKey(undefined)
            setStreamStage(undefined)
            setResult(response)
            if (response.session_id) {
              setActiveSessionId(response.session_id)
            }
          },
        }, { signal: controller.signal })
      } catch (streamError) {
        if (isAbortError(streamError)) {
          throw streamError
        }
        console.error(streamError)
        setStageKey(undefined)
        setStreamStage('流式不可用，正在回退到普通请求')
        const response = await queryCopilotWithOptions(payload, { signal: controller.signal })
        streamResult = response
        setHistory([
          ...history,
          { role: 'user', content: nextQuestion },
          { role: 'assistant', content: response.answer },
        ])
        setResult(response)
        if (response.session_id) {
          setActiveSessionId(response.session_id)
        }
      }

      if (streamResult) {
        setStreamStage(undefined)
        void sessionsQuery.refetch()
        setQuestion('继续追问这个区间里最关键的信号是什么？')
      }
    } catch (requestError) {
      if (isAbortError(requestError)) {
        setHistory((current) => {
          if (!current.length) {
            return current
          }
          const last = current[current.length - 1]
          if (last.role === 'assistant' && !last.content.trim()) {
            return current.slice(0, -1)
          }
          return current
        })
        setStageKey(undefined)
        setStreamStage(undefined)
        setError('已停止生成')
      } else {
        setStageKey(undefined)
        setStreamStage(undefined)
        setError(
          isUnauthorizedError(requestError)
            ? '登录已失效，正在返回登录页。'
            : 'AI 分析失败，请确认 backend 与模型配置。',
        )
      }
      console.error(requestError)
    } finally {
      abortControllerRef.current = null
      setLoading(false)
    }
  }

  function handleStop() {
    abortControllerRef.current?.abort()
  }

  async function handleCopyAnswer() {
    if (!result?.answer) {
      return
    }

    try {
      await navigator.clipboard.writeText(result.answer)
      setCopied(true)
    } catch (copyError) {
      console.error(copyError)
    }
  }

  const hasHistory = history.length > 0
  const hasConnectedSession = Boolean(activeSessionId)
  const latestAssistantIndex = useMemo(
    () => [...history].map((item, index) => ({ item, index })).reverse().find(({ item }) => item.role === 'assistant')?.index,
    [history],
  )
  const resultBiasLabel = result?.bias === 'bullish' ? '偏多' : result?.bias === 'bearish' ? '偏空' : '中性'
  const activeStageIndex = stageMeta.findIndex((item) => item.key === stageKey)

  return (
    <Card
      className="panel-card copilot-card"
      title={(
        <div className="copilot-toolbar">
          <div className="copilot-toolbar-title">
            <strong>AI 助手</strong>
            <span>{symbol ?? '未选择股票'}</span>
          </div>
          <Space size={6}>
            {hasConnectedSession ? <Tag color="green">已连接会话</Tag> : <Tag>新会话</Tag>}
            <Tag color={providerApiKey?.trim() || providerEnabled ? 'blue' : 'gold'}>
              {providerApiKey?.trim() ? '自带 Key' : providerEnabled ? (providerLabel ?? provider ?? '模型已连接') : '规则回退'}
            </Tag>
            <Tag color={loading ? 'processing' : 'default'}>{loading ? '分析中' : '就绪'}</Tag>
            {streamStage ? <Tag color="cyan">{streamStage}</Tag> : null}
          </Space>
        </div>
      )}
      styles={{ body: { padding: 18 } }}
    >
      <div className="copilot-panel">
        <div className="copilot-scroll" ref={scrollRef}>
          <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
            {!providerEnabled && !providerApiKey?.trim() ? (
              <Alert
                type="warning"
                showIcon
                title="还没有填写 AI Key，回复会先使用规则分析。"
              />
            ) : null}
            {hasHistory ? (
              <div className="chat-history">
                {history.map((item, index) => (
                  <div
                    key={`${item.role}-${index}`}
                    className={`chat-bubble chat-bubble--${item.role}`}
                  >
                    <div className="chat-bubble-head">
                      <span className="chat-bubble-role">
                        {item.role === 'user' ? '我' : 'AI'}
                      </span>
                      {item.role === 'assistant' && latestAssistantIndex === index && result ? (
                        <Space size={6}>
                          <Tag color={tagColor(result.bias)}>{resultBiasLabel}</Tag>
                          <Button type="text" size="small" icon={<CopyOutlined />} onClick={() => void handleCopyAnswer()}>
                            {copied ? '已复制' : '复制'}
                          </Button>
                          <Button type="text" size="small" icon={<ReloadOutlined />} loading={loading} onClick={() => void handleSubmit()}>
                            重试
                          </Button>
                        </Space>
                      ) : null}
                    </div>
                    <Paragraph style={{ marginBottom: 0 }}>{item.content}</Paragraph>
                    {item.role === 'assistant' && latestAssistantIndex === index && result ? (
                      <div className="assistant-analysis-stack">
                        <div className="assistant-level-strip">
                          <div className="assistant-level-chip">
                            <span>支撑位</span>
                            <strong>{result.levels?.support?.value?.toFixed(2) ?? '--'}</strong>
                          </div>
                          <div className="assistant-level-chip">
                            <span>压力位</span>
                            <strong>{result.levels?.pressure?.value?.toFixed(2) ?? '--'}</strong>
                          </div>
                          <div className="assistant-level-chip">
                            <span>风险位</span>
                            <strong>{result.levels?.risk?.value?.toFixed(2) ?? '--'}</strong>
                          </div>
                        </div>
                        <Collapse
                          ghost
                          className="copilot-sections"
                          items={[
                            {
                              key: 'levels',
                              label: '结构化分析',
                              children: (
                                <div className="result-block result-block--subtle">
                                  <div className="copilot-level-strip">
                                    <div className="copilot-level-chip">
                                      <span>支撑位</span>
                                      <strong>{result.levels?.support?.value?.toFixed(2) ?? '--'}</strong>
                                      <small>{result.levels?.support?.reason ?? '无'}</small>
                                    </div>
                                    <div className="copilot-level-chip">
                                      <span>压力位</span>
                                      <strong>{result.levels?.pressure?.value?.toFixed(2) ?? '--'}</strong>
                                      <small>{result.levels?.pressure?.reason ?? '无'}</small>
                                    </div>
                                    <div className="copilot-level-chip">
                                      <span>风险位</span>
                                      <strong>{result.levels?.risk?.value?.toFixed(2) ?? '--'}</strong>
                                      <small>{result.levels?.risk?.reason ?? '无'}</small>
                                    </div>
                                  </div>
                                </div>
                              ),
                            },
                            {
                              key: 'news',
                              label: (
                                <Flex justify="space-between" align="center" style={{ width: '100%' }}>
                                  <span>新闻证据</span>
                                  <Tag color={result.news_context?.used ? 'blue' : 'default'}>
                                    {result.news_context?.used ? `${result.news_context.count} 条` : '仅图表'}
                                  </Tag>
                                </Flex>
                              ),
                              children: (
                                <>
                                  <Paragraph type="secondary" style={{ marginTop: 0 }}>
                                    {result.news_context?.note || '当前没有新闻证据。'}
                                  </Paragraph>
                                  {result.news_context?.items?.length ? (
                                    <div className="news-evidence-list">
                                      {result.news_context.items.map((newsItem) => (
                                        <div key={`${newsItem.source}-${newsItem.published_at}-${newsItem.title}`} className="news-evidence-item">
                                          <strong>{newsItem.title}</strong>
                                          <span>{newsItem.source} · {newsItem.published_at}</span>
                                          <small>{newsItem.summary || newsItem.relevance_reason}</small>
                                        </div>
                                      ))}
                                    </div>
                                  ) : null}
                                </>
                              ),
                            },
                            {
                              key: 'key-points',
                              label: '关键点',
                              children: (
                                <ul>
                                  {result.key_points.map((point) => (
                                    <li key={point}>{point}</li>
                                  ))}
                                </ul>
                              ),
                            },
                            {
                              key: 'risk-points',
                              label: '风险点',
                              children: (
                                <ul>
                                  {result.risk_points.map((point) => (
                                    <li key={point}>{point}</li>
                                  ))}
                                </ul>
                              ),
                            },
                            {
                              key: 'watch-items',
                              label: '下一步观察',
                              children: (
                                <ul>
                                  {result.watch_items.map((watchItem) => (
                                    <li key={watchItem}>{watchItem}</li>
                                  ))}
                                </ul>
                              ),
                            },
                          ]}
                        />
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>
            ) : null}
            {loading ? (
              <div className="copilot-loading">
                <Spin size="small" />
                <span>{streamStage || 'AI 正在整理完整分析，可以随时停止。'}</span>
              </div>
            ) : null}
            {loading && activeStageIndex >= 0 ? (
              <Space wrap size={6}>
                {stageMeta.map((item, index) => (
                  <Tag
                    key={item.key}
                    color={index < activeStageIndex ? 'green' : index === activeStageIndex ? 'processing' : 'default'}
                  >
                    {item.label}
                  </Tag>
                ))}
              </Space>
            ) : null}
            {!hasHistory && !result && !loading ? (
              <div className="copilot-empty">
                <strong>像对话一样提问</strong>
                <span>先框选区间，再问趋势、关键位、风险或新闻催化。</span>
              </div>
            ) : null}
          </Space>
        </div>
        <div className="copilot-footer">
          <Input.TextArea
            autoSize={{ minRows: 3, maxRows: 8 }}
            value={question}
            disabled={loading}
            onChange={(event) => setQuestion(event.target.value)}
            onPressEnter={(event) => {
              if (event.shiftKey) {
                return
              }
              event.preventDefault()
              void handleSubmit()
            }}
            placeholder="输入你想问的问题"
            className="copilot-composer"
          />
          <Flex justify="space-between" align="center" gap={12} wrap="wrap">
            <Text type="secondary">
              当前股票: <strong>{symbol ?? '--'}</strong>
            </Text>
            <Text type="secondary">
              区间: <strong>{range?.[0] && range?.[1] ? `${range[0].format('MM-DD')} -> ${range[1].format('MM-DD')}` : '全部'}</strong>
            </Text>
            <Space>
              {loading ? (
                <Button danger onClick={handleStop}>
                  停止生成
                </Button>
              ) : (
                <Button type="primary" onClick={handleSubmit}>
                  生成分析
                </Button>
              )}
            </Space>
          </Flex>
          {error ? <Alert type="error" title={error} showIcon /> : null}
        </div>
      </div>
    </Card>
  )
}
