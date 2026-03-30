import { useEffect, useMemo, useState } from 'react'
import {
  App,
  Alert,
  Button,
  Card,
  DatePicker,
  Empty,
  Flex,
  Input,
  Layout,
  Select,
  Space,
  Spin,
  Typography,
} from 'antd'
import { DeleteOutlined, LogoutOutlined, ReloadOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { type Dayjs } from 'dayjs'
import { useNavigate } from 'react-router-dom'
import { ChartPanel } from '../components/ChartPanel'
import { CopilotPanel } from '../components/CopilotPanel'
import { fetchAIProviders } from '../lib/copilotApi'
import { buildQueryErrorAlert } from '../lib/http'
import { deleteSymbol, fetchBillingSummary, fetchOHLC, fetchSymbols, logout } from '../lib/api'
import { clearAuth, getStoredUser } from '../lib/auth'

const { Header, Content, Sider } = Layout
const { RangePicker } = DatePicker
const { Search } = Input
const { Paragraph, Text, Title } = Typography

export function DashboardPage() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const user = getStoredUser()
  const [selectedSymbol, setSelectedSymbol] = useState<string>()
  const [range, setRange] = useState<[Dayjs | null, Dayjs | null] | null>(null)
  const [refreshSeed, setRefreshSeed] = useState(0)
  const [symbolInput, setSymbolInput] = useState('')
  const [provider, setProvider] = useState<string>('deepseek')

  const symbolsQuery = useQuery({
    queryKey: ['symbols', refreshSeed],
    queryFn: () => fetchSymbols(refreshSeed > 0),
  })

  const providersQuery = useQuery({
    queryKey: ['ai-providers'],
    queryFn: fetchAIProviders,
  })

  const billingQuery = useQuery({
    queryKey: ['billing-summary'],
    queryFn: fetchBillingSummary,
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
    if (defaultProvider) {
      setProvider((current) => current || defaultProvider.id)
    }
  }, [providerList])

  const ohlcQuery = useQuery({
    queryKey: ['ohlc', selectedSymbol],
    enabled: Boolean(selectedSymbol),
    queryFn: () => fetchOHLC(selectedSymbol!),
  })

  const currentSymbol = useMemo(
    () => symbolList.find((item) => item.symbol === selectedSymbol),
    [selectedSymbol, symbolList],
  )
  const currentProvider = useMemo(
    () => providerList.find((item) => item.id === provider),
    [provider, providerList],
  )
  const providerOptions = providerList.map((item) => ({
    label: `${item.name} · ${item.model}`,
    value: item.id,
    disabled: !item.enabled,
  }))
  const dashboardError = buildQueryErrorAlert(symbolsQuery.error ?? ohlcQuery.error, {
    dataSourceName: '工作台数据',
  })

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
    message.info('正在同步东方财富数据')
  }

  async function handleAddSymbol(rawValue: string) {
    const normalized = rawValue.trim().replace(/^sh|^sz/i, '')
    if (!/^\d{6}$/.test(normalized)) {
      message.error('请输入 6 位股票代码，例如 000001 或 600519')
      return
    }

    if (symbolList.some((item) => item.symbol === normalized)) {
      setSelectedSymbol(normalized)
      setSymbolInput('')
      message.info(`${normalized} 已在列表中，已为你切换`)
      return
    }

    try {
      await fetchOHLC(normalized)
      setRefreshSeed((value) => value + 1)
      setSelectedSymbol(normalized)
      setSymbolInput('')
      message.success(`已添加 ${normalized}`)
    } catch (error) {
      console.error(error)
      message.error('添加股票失败，请确认代码是否有效')
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
    } catch (error) {
      console.error(error)
      message.error('删除股票失败')
    }
  }

  return (
    <Layout className="dashboard-shell">
      <Sider
        width={280}
        theme="light"
        className="dashboard-sider"
        style={{
          borderRight: '1px solid rgba(15, 23, 42, 0.08)',
          background: 'rgba(255,255,255,0.72)',
          backdropFilter: 'blur(14px)',
        }}
      >
        <Flex vertical style={{ height: '100%', padding: 20 }} gap={18}>
          <Flex justify="space-between" align="center">
            <div className="brand-mark">
              <strong>市场助手</strong>
              <span>东方财富自选集</span>
            </div>
            <Button type="text" icon={<ReloadOutlined />} onClick={handleRefresh} />
          </Flex>

          <Card size="small" style={{ borderRadius: 20 }}>
            <Text type="secondary">当前登录</Text>
            <Title level={5} style={{ marginTop: 6, marginBottom: 0 }}>
              {user?.email ?? 'anonymous'}
            </Title>
          </Card>

          <Card size="small" style={{ flex: 1, borderRadius: 20 }}>
            <Flex justify="space-between" align="center" style={{ marginBottom: 12 }}>
              <Text strong>股票列表</Text>
              {symbolsQuery.isFetching ? <Spin size="small" /> : null}
            </Flex>
            <Search
              value={symbolInput}
              onChange={(event) => setSymbolInput(event.target.value)}
              onSearch={handleAddSymbol}
              placeholder="输入 6 位股票代码添加"
              enterButton="添加"
              style={{ marginBottom: 12 }}
            />
            {symbolList.length ? (
              <Flex vertical gap={8}>
                {symbolList.map((item) => (
                  <div
                    key={item.symbol}
                    style={{
                      cursor: 'pointer',
                      padding: '12px 14px',
                      borderRadius: 16,
                      background:
                        item.symbol === selectedSymbol ? '#ecfdf5' : 'rgba(248,250,252,0.85)',
                      border:
                        item.symbol === selectedSymbol
                          ? '1px solid #99f6e4'
                          : '1px solid rgba(148, 163, 184, 0.16)',
                    }}
                    onClick={() => setSelectedSymbol(item.symbol)}
                  >
                    <Flex justify="space-between" align="flex-start" gap={12}>
                      <Flex vertical>
                        <Text strong>{item.symbol}</Text>
                        <Text type="secondary">
                          {item.name} · {item.market}
                        </Text>
                      </Flex>
                      <Button
                        type="text"
                        size="small"
                        icon={<DeleteOutlined />}
                        onClick={(event) => {
                          event.stopPropagation()
                          void handleDeleteSymbol(item.symbol)
                        }}
                      />
                    </Flex>
                  </div>
                ))}
              </Flex>
            ) : (
              <Empty
                description="还没有股票数据。点击顶部刷新按钮会从东方财富同步默认股票。"
                image={Empty.PRESENTED_IMAGE_SIMPLE}
              />
            )}
          </Card>

          <Card size="small" style={{ borderRadius: 20 }}>
            <Flex justify="space-between" align="center">
              <div>
                <Text type="secondary">今日会员额度</Text>
                <Title level={5} style={{ margin: '6px 0 0' }}>
                  {(billingQuery.data?.today_quota?.remaining ?? 0).toLocaleString()} / {(billingQuery.data?.today_quota?.total ?? 0).toLocaleString()}
                </Title>
                <Text type="secondary">免费 credits: {billingQuery.data?.credit_balance?.toLocaleString() ?? 0}</Text>
              </div>
            </Flex>
            <Paragraph type="secondary" style={{ margin: '10px 0 0' }}>
              当前阶段通过兑换码激活会员档位，AI 会优先消耗日额度，再补扣免费 credits。
            </Paragraph>
            <Button style={{ marginTop: 10 }} block onClick={() => navigate('/billing')}>
              兑换码中心
            </Button>
            {user?.is_admin ? (
              <Button style={{ marginTop: 10 }} block onClick={() => navigate('/admin')}>
                管理后台
              </Button>
            ) : null}
          </Card>

          <Button icon={<LogoutOutlined />} onClick={handleLogout}>
            退出登录
          </Button>
        </Flex>
      </Sider>

      <Layout className="dashboard-main">
        <Header
          className="dashboard-header"
          style={{
            background: 'transparent',
            padding: '18px 24px 0',
            height: 'auto',
          }}
        >
          <Flex justify="space-between" align="center" wrap="wrap" gap={16}>
            <div>
              <Text type="secondary">盘中分析工作台</Text>
              <Title level={2} style={{ margin: 0 }}>
                {currentSymbol ? `${currentSymbol.name} (${currentSymbol.symbol})` : '选择一个股票'}
              </Title>
            </div>
            <Space wrap>
              <Select
                value={provider}
                onChange={setProvider}
                style={{ minWidth: 180 }}
                options={providerOptions}
              />
              <RangePicker value={range} onChange={(value) => setRange(value)} />
            </Space>
          </Flex>
        </Header>

        <Content className="dashboard-content" style={{ padding: 24 }}>
          {symbolsQuery.error || ohlcQuery.error ? (
            <Alert
              type="error"
              showIcon
              style={{ marginBottom: 16 }}
              title={dashboardError.title}
              description={dashboardError.description}
            />
          ) : null}
          <div className="content-grid">
            <div className="dashboard-left-column">
              <ChartPanel
                rows={ohlcQuery.data ?? []}
                range={range}
                onRangeChange={setRange}
                isLoading={ohlcQuery.isFetching}
              />
            </div>
            <div>
              <CopilotPanel
                symbol={selectedSymbol}
                range={range}
                provider={provider}
                providerEnabled={currentProvider?.enabled ?? false}
                providerLabel={currentProvider ? `${currentProvider.name} · ${currentProvider.model}` : provider}
              />
            </div>
          </div>
        </Content>
      </Layout>

    </Layout>
  )
}
