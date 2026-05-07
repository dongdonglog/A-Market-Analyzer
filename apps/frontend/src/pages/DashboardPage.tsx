import { useEffect, useMemo, useState } from 'react'
import {
  App,
  Button,
  Empty,
  Flex,
  Input,
  Select,
  Space,
  Spin,
} from 'antd'
import {
  CloseOutlined,
  DeleteOutlined,
  MessageOutlined,
  PlusOutlined,
  ReloadOutlined,
} from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { type Dayjs } from 'dayjs'
import { ChartPanel } from '../components/ChartPanel'
import { CopilotPanel } from '../components/CopilotPanel'
import { fetchAIProviders } from '../lib/copilotApi'
import { addSymbol, deleteSymbol, fetchOHLC, fetchSymbols, logout } from '../lib/api'
import { clearAuth } from '../lib/auth'

export function DashboardPage() {
  const { message } = App.useApp()
  const [selectedSymbol, setSelectedSymbol] = useState<string>()
  const [range, setRange] = useState<[Dayjs | null, Dayjs | null] | null>(null)
  const [refreshSeed, setRefreshSeed] = useState(0)
  const [symbolInput, setSymbolInput] = useState('')
  const [provider, setProvider] = useState<string>('auto')
  const [providerApiKey, setProviderApiKey] = useState(() => localStorage.getItem('market.aiKey') ?? '')
  const [aiDrawerOpen, setAiDrawerOpen] = useState(false)

  const symbolsQuery = useQuery({
    queryKey: ['symbols', refreshSeed],
    queryFn: () => fetchSymbols(refreshSeed > 0),
  })

  const providersQuery = useQuery({
    queryKey: ['ai-providers'],
    queryFn: fetchAIProviders,
  })

  const providerList = Array.isArray(providersQuery.data) ? providersQuery.data : []
  const symbolList = Array.isArray(symbolsQuery.data) ? symbolsQuery.data : []

  useEffect(() => {
    if (!selectedSymbol && symbolList.length) {
      setSelectedSymbol(symbolList[0].symbol)
    }
  }, [selectedSymbol, symbolList])

  useEffect(() => {
    setRange(null)
  }, [selectedSymbol])

  useEffect(() => {
    const defaultProvider = providerList.find((item) => item.is_default && item.enabled)
      ?? providerList.find((item) => item.enabled)
    if (defaultProvider && provider !== 'auto') {
      setProvider((current) => current || defaultProvider.id)
    }
  }, [provider, providerList])

  useEffect(() => {
    localStorage.setItem('market.aiKey', providerApiKey.trim())
  }, [providerApiKey])

  const ohlcQuery = useQuery({
    queryKey: ['ohlc', selectedSymbol, refreshSeed],
    enabled: Boolean(selectedSymbol),
    queryFn: () => fetchOHLC(selectedSymbol!),
  })

  const currentSymbol = useMemo(
    () => symbolList.find((item) => item.symbol === selectedSymbol),
    [selectedSymbol, symbolList],
  )

  const latestOHLC = useMemo(() => {
    const rows = ohlcQuery.data ?? []
    return rows.length > 0 ? rows[rows.length - 1] : null
  }, [ohlcQuery.data])

  const providerOptions = providerList.map((item) => ({
    label: `${item.name} · ${item.model}`,
    value: item.id,
  }))

  async function handleLogout() {
    try {
      await logout()
    } catch (error) {
      console.error(error)
    } finally {
      clearAuth()
      window.location.href = '/login'
    }
  }

  function handleRefresh() {
    setRefreshSeed((value) => value + 1)
    message.info('正在刷新行情数据')
  }

  async function handleAddSymbol(rawValue: string) {
    const normalized = rawValue.trim().replace(/^sh|^sz/i, '')
    if (!/^\d{6}$/.test(normalized)) {
      message.error('请输入 6 位股票代码')
      return
    }

    if (symbolList.some((item) => item.symbol === normalized)) {
      setSelectedSymbol(normalized)
      setSymbolInput('')
      message.info('已在列表中')
      return
    }

    try {
      await fetchOHLC(normalized)
      setRefreshSeed((value) => value + 1)
      setSelectedSymbol(normalized)
      setSymbolInput('')
      message.success(`已添加 ${normalized}`)
    } catch {
      try {
        await addSymbol(normalized)
        setRefreshSeed((value) => value + 1)
        setSelectedSymbol(normalized)
        setSymbolInput('')
        message.success(`已添加 ${normalized}`)
      } catch {
        message.error('添加失败')
      }
    }
  }

  async function handleDeleteSymbol(symbol: string) {
    try {
      await deleteSymbol(symbol)
      await symbolsQuery.refetch()
      if (selectedSymbol === symbol) {
        setSelectedSymbol(undefined)
      }
      message.success(`已删除 ${symbol}`)
    } catch {
      message.error('删除失败')
    }
  }

  const changePercent = useMemo(() => {
    if (!latestOHLC || !latestOHLC.change_rate) return null
    return latestOHLC.change_rate
  }, [latestOHLC])

  const isUp = changePercent !== null && changePercent > 0

  return (
    <div className="dashboard-shell">
      <div className="dashboard-topbar">
        <div className="topbar-logo">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
          </svg>
          <span>市场助手</span>
        </div>

        <div className="topbar-symbols">
          {symbolList.map((item) => {
            const isActive = item.symbol === selectedSymbol
            return (
              <div
                key={item.symbol}
                className={`topbar-symbol ${isActive ? 'active' : ''}`}
                onClick={() => setSelectedSymbol(item.symbol)}
              >
                <span className="topbar-symbol-code">{item.symbol}</span>
                <span className="topbar-symbol-name">{item.name}</span>
                <Button
                  type="text"
                  size="small"
                  icon={<DeleteOutlined style={{ fontSize: 10 }} />}
                  style={{ padding: 0, height: 'auto', color: 'inherit' }}
                  onClick={(e) => {
                    e.stopPropagation()
                    void handleDeleteSymbol(item.symbol)
                  }}
                />
              </div>
            )
          })}
          <Input
            size="small"
            value={symbolInput}
            onChange={(e) => setSymbolInput(e.target.value)}
            onPressEnter={() => handleAddSymbol(symbolInput)}
            placeholder="+ 添加"
            style={{ width: 100, background: 'transparent', border: 'none' }}
            prefix={<PlusOutlined style={{ fontSize: 10 }} />}
          />
        </div>

        <div className="topbar-actions">
          <Button
            size="small"
            icon={<ReloadOutlined spin={ohlcQuery.isFetching} />}
            onClick={handleRefresh}
          >
            刷新
          </Button>
          <Button size="small" onClick={handleLogout}>
            退出
          </Button>
        </div>
      </div>

      {currentSymbol && (
        <div className="dashboard-info-bar">
          <span className="info-bar-symbol">
            {currentSymbol.name} ({currentSymbol.symbol})
          </span>
          {latestOHLC && (
            <>
              <div className="info-bar-divider" />
              <span className={`info-bar-price ${isUp ? 'up' : 'down'}`}>
                {latestOHLC.close.toFixed(2)}
              </span>
              {changePercent !== null && (
                <span className={`info-bar-change ${isUp ? 'up' : 'down'}`}>
                  {isUp ? '+' : ''}{changePercent.toFixed(2)}%
                </span>
              )}
              <div className="info-bar-divider" />
              <span className="info-bar-label">开</span>
              <span className="info-bar-value">{latestOHLC.open.toFixed(2)}</span>
              <div className="info-bar-divider" />
              <span className="info-bar-label">高</span>
              <span className="info-bar-value">{latestOHLC.high.toFixed(2)}</span>
              <div className="info-bar-divider" />
              <span className="info-bar-label">低</span>
              <span className="info-bar-value">{latestOHLC.low.toFixed(2)}</span>
              <div className="info-bar-divider" />
              <span className="info-bar-label">量</span>
              <span className="info-bar-value">{(latestOHLC.volume / 10000).toFixed(0)}万</span>
            </>
          )}
        </div>
      )}

      <div className="dashboard-main">
        <div className="chart-area">
          <div className="chart-container">
            {ohlcQuery.isFetching ? (
              <Flex align="center" justify="center" style={{ height: '100%' }}>
                <Spin />
              </Flex>
            ) : ohlcQuery.data && ohlcQuery.data.length > 0 ? (
              <ChartPanel
                rows={ohlcQuery.data}
                range={range}
                onRangeChange={setRange}
                isLoading={ohlcQuery.isFetching}
              />
            ) : (
              <Empty description="暂无数据" />
            )}
          </div>
        </div>

        <div className={`ai-drawer ${aiDrawerOpen ? 'open' : ''}`}>
          <div className="ai-drawer-content">
            <div className="ai-drawer-header">
              <span className="ai-drawer-title">AI 分析</span>
              <Space>
                <Select
                  value={provider}
                  onChange={setProvider}
                  size="small"
                  style={{ width: 140 }}
                  options={[
                    { label: '自动识别', value: 'auto' },
                    ...providerOptions,
                  ]}
                />
                <Button
                  type="text"
                  size="small"
                  icon={<CloseOutlined />}
                  onClick={() => setAiDrawerOpen(false)}
                />
              </Space>
            </div>
            <div className="ai-drawer-body">
              <CopilotPanel
                symbol={selectedSymbol}
                range={range}
                provider={provider}
                providerApiKey={providerApiKey}
                providerEnabled={Boolean(providerApiKey.trim())}
              />
            </div>
            <div className="ai-drawer-footer">
              <Input.Password
                value={providerApiKey}
                onChange={(e) => setProviderApiKey(e.target.value)}
                placeholder="API Key"
                size="small"
                autoComplete="off"
              />
            </div>
          </div>
        </div>

        <button
          className={`ai-fab ${aiDrawerOpen ? 'hidden' : ''}`}
          onClick={() => setAiDrawerOpen(true)}
        >
          <MessageOutlined />
        </button>
      </div>
    </div>
  )
}
