import { useEffect, useMemo, useRef, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import { Button, Flex, Input, Space, Spin, Tag, Typography } from 'antd'
import { CopyOutlined, ReloadOutlined, StopOutlined } from '@ant-design/icons'
import dayjs, { type Dayjs } from 'dayjs'
import { useQuery } from '@tanstack/react-query'
import { fetchCopilotSessionMessages, fetchCopilotSessions, queryCopilotWithOptions, streamCopilot } from '../lib/copilotApi'
import { isUnauthorizedError } from '../lib/http'
import type { ChatMessage, CopilotResponse } from '../types/api'

const { Text } = Typography

interface CopilotPanelProps {
  symbol?: string
  range: [Dayjs | null, Dayjs | null] | null
  provider?: string
  providerApiKey?: string
  providerEnabled?: boolean
}

function tagColor(bias?: string) {
  switch (bias) {
    case 'bullish':
      return 'red'
    case 'bearish':
      return 'green'
    default:
      return 'default'
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

function groupSessionsByDate(sessions: Array<{ id: string; session_date: string; title: string; summary: string; is_compressed?: boolean; message_count: number }>) {
  const groups: Array<{ label: string; sessions: typeof sessions }> = []
  const today = dayjs()
  const yesterday = today.subtract(1, 'day')
  const twoDaysAgo = today.subtract(2, 'day')

  const todaySessions = sessions.filter((s) => dayjs(s.session_date).isSame(today, 'day'))
  const yesterdaySessions = sessions.filter((s) => dayjs(s.session_date).isSame(yesterday, 'day'))
  const twoDaysAgoSessions = sessions.filter((s) => dayjs(s.session_date).isSame(twoDaysAgo, 'day'))
  const olderSessions = sessions.filter((s) => dayjs(s.session_date).isBefore(twoDaysAgo, 'day'))

  if (todaySessions.length) groups.push({ label: '今天', sessions: todaySessions })
  if (yesterdaySessions.length) groups.push({ label: '昨天', sessions: yesterdaySessions })
  if (twoDaysAgoSessions.length) groups.push({ label: '前天', sessions: twoDaysAgoSessions })
  if (olderSessions.length) groups.push({ label: '更早', sessions: olderSessions })

  return groups
}

export function CopilotPanel({ symbol, range, provider, providerApiKey, providerEnabled = false }: CopilotPanelProps) {
  const [question, setQuestion] = useState('这段走势说明了什么，接下来要观察什么？')
  const [loading, setLoading] = useState(false)
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

    setActiveSessionId((current) => {
      if (current) return current
      const todaySessions = sessionsQuery.data.filter((s) => dayjs(s.session_date).isSame(dayjs(), 'day'))
      return todaySessions.length ? todaySessions[0].id : sessionsQuery.data[0].id
    })
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
      setStreamStage('正在连接')
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
            setStreamStage('已连接')
            if (streamStart.session_id) {
              setActiveSessionId(streamStart.session_id)
            }
          },
          onStage: (stageEvent) => {
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
        setStreamStage('流式不可用，回退到普通请求')
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
        setStreamStage(undefined)
        setError('已停止生成')
      } else {
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
  const latestAssistantIndex = useMemo(
    () => [...history].map((item, index) => ({ item, index })).reverse().find(({ item }) => item.role === 'assistant')?.index,
    [history],
  )
  const resultBiasLabel = result?.bias === 'bullish' ? '偏多' : result?.bias === 'bearish' ? '偏空' : '中性'

  const groupedSessions = useMemo(() => {
    if (!sessionsQuery.data) return []
    return groupSessionsByDate(sessionsQuery.data)
  }, [sessionsQuery.data])

  return (
    <div className="copilot-panel">
      <div className="copilot-scroll" ref={scrollRef}>
        <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
          {groupedSessions.length > 0 && (
            <div className="session-list">
              {groupedSessions.map((group) => (
                <div key={group.label}>
                  <div className="session-group-label">{group.label}</div>
                  {group.sessions.map((session) => (
                    <div
                      key={session.id}
                      className={`session-item ${session.id === activeSessionId ? 'active' : ''}`}
                      onClick={() => setActiveSessionId(session.id)}
                    >
                      <span className="session-item-title">
                        {session.title || session.session_date}
                      </span>
                      {session.is_compressed && (
                        <span className="session-item-badge compressed">摘要</span>
                      )}
                    </div>
                  ))}
                </div>
              ))}
            </div>
          )}

          {!providerEnabled && !providerApiKey?.trim() ? (
            <Tag color="warning">未配置 AI Key，将使用规则分析</Tag>
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
                  {item.role === 'assistant' ? (
                    <div className="markdown-content">
                      <ReactMarkdown>{item.content}</ReactMarkdown>
                    </div>
                  ) : (
                    <div className="user-message">{item.content}</div>
                  )}
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
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          ) : null}

          {loading ? (
            <div className="copilot-loading">
              <Spin size="small" />
              <span>{streamStage || 'AI 正在分析...'}</span>
            </div>
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
            {symbol ?? '未选择股票'}
          </Text>
          <Space>
            {loading ? (
              <Button danger size="small" icon={<StopOutlined />} onClick={handleStop}>
                停止
              </Button>
            ) : (
              <Button type="primary" size="small" onClick={handleSubmit}>
                发送
              </Button>
            )}
          </Space>
        </Flex>
        {error ? <Tag color="error">{error}</Tag> : null}
      </div>
    </div>
  )
}
