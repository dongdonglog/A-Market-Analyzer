import { useState } from 'react'
import { Alert, App, Button, Card, Empty, Flex, Input, Layout, Space, Table, Typography } from 'antd'
import { ArrowLeftOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { buildQueryErrorAlert } from '../lib/http'
import { fetchBillingSummary, redeemCode } from '../lib/api'

const { Header, Content } = Layout
const { Text, Title, Paragraph } = Typography

export function BillingPage() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const [redeemInput, setRedeemInput] = useState('STARTER30')
  const [redeemLoading, setRedeemLoading] = useState(false)
  const billingQuery = useQuery({
    queryKey: ['billing-summary'],
    queryFn: fetchBillingSummary,
  })
  const billingError = buildQueryErrorAlert(billingQuery.error, {
    dataSourceName: '账单信息',
  })

  async function handleRedeemCode() {
    const code = redeemInput.trim()
    if (!code) {
      message.error('请输入兑换码')
      return
    }

    setRedeemLoading(true)
    try {
      const response = await redeemCode({ code })
      message.success(response.message)
      setRedeemInput('')
      await billingQuery.refetch()
    } catch (error) {
      console.error(error)
      message.error('兑换失败，请检查兑换码是否有效或是否已领取')
    } finally {
      setRedeemLoading(false)
    }
  }

  return (
    <Layout className="billing-shell">
      <Header className="billing-header">
        <Flex justify="space-between" align="center" gap={12}>
          <div>
            <Text type="secondary">Billing</Text>
            <Title level={2} style={{ margin: 0 }}>会员与兑换码</Title>
          </div>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/')}>
            返回工作台
          </Button>
        </Flex>
      </Header>
      <Content className="billing-content">
        {billingQuery.error ? (
          <Alert
            type="error"
            title={billingError.title}
            description={billingError.description}
            showIcon
          />
        ) : null}

        <div className="billing-grid">
          <Card className="panel-card" title="余额与档位">
            <Space direction="vertical" size="middle" style={{ width: '100%' }}>
              <div>
                <Text type="secondary">免费 credits 余额</Text>
                <Title level={3} style={{ margin: '8px 0 0' }}>
                  {billingQuery.data?.credit_balance?.toLocaleString() ?? 0} credits
                </Title>
              </div>
              <div>
                <Text type="secondary">今日会员额度</Text>
                <Title level={4} style={{ margin: '8px 0 0' }}>
                  {(billingQuery.data?.today_quota?.remaining ?? 0).toLocaleString()} / {(billingQuery.data?.today_quota?.total ?? 0).toLocaleString()}
                </Title>
                <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                  今日已用 {(billingQuery.data?.today_quota?.used ?? 0).toLocaleString()}，AI 会优先消耗这里，再补扣免费 credits。
                </Paragraph>
              </div>
              <div>
                <Text type="secondary">当前会员</Text>
                <Paragraph style={{ marginTop: 8, marginBottom: 0 }}>
                  {billingQuery.data?.current_membership
                    ? `${billingQuery.data.current_membership.package_name} · 日额度 ${billingQuery.data.current_membership.daily_quota.toLocaleString()} · 到期 ${billingQuery.data.current_membership.ends_at}`
                    : '当前没有生效中的会员'}
                </Paragraph>
              </div>
              <div>
                <Text type="secondary">兑换会员或免费额度</Text>
                <Space.Compact style={{ width: '100%', marginTop: 8 }}>
                  <Input
                    value={redeemInput}
                    onChange={(event) => setRedeemInput(event.target.value)}
                    placeholder="输入兑换码"
                  />
                  <Button type="primary" loading={redeemLoading} onClick={() => void handleRedeemCode()}>
                    兑换
                  </Button>
                </Space.Compact>
                <Paragraph type="secondary" style={{ marginTop: 8, marginBottom: 0 }}>
                  当前开发环境预置了 demo 兑换码：`WELCOME100`、`STARTER30`、`ACTIVE30`、`PRO30`。
                </Paragraph>
              </div>
              {(billingQuery.data?.packages ?? []).map((pkg) => (
                <div key={pkg.id} className="billing-package">
                  <strong>{pkg.name}</strong>
                  <span>¥{pkg.amount_cny} · 每日 {pkg.daily_quota.toLocaleString()} · {pkg.duration_days} 天</span>
                  <small>{pkg.description}</small>
                </div>
              ))}
            </Space>
          </Card>

          <Card className="panel-card" title="兑换说明">
            <Space direction="vertical" size="small">
              <Paragraph style={{ marginBottom: 0 }}>
                `STARTER30`：激活 Starter，30 天，每日 10,000。
              </Paragraph>
              <Paragraph style={{ marginBottom: 0 }}>
                `ACTIVE30`：激活 Active，30 天，每日 40,000。
              </Paragraph>
              <Paragraph style={{ marginBottom: 0 }}>
                `PRO30`：激活 Pro，30 天，每日 140,000。
              </Paragraph>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                支付入口当前已隐藏，先按兑换码运营。后续要恢复支付时，后端 mock 支付链路还在。
              </Paragraph>
            </Space>
          </Card>

          <Card className="panel-card" title="模型用量记录">
            {billingQuery.data?.usage?.length ? (
              <Table
                rowKey="id"
                size="small"
                pagination={false}
                dataSource={billingQuery.data.usage}
                columns={[
                  { title: '时间', dataIndex: 'created_at' },
                  { title: 'Provider', dataIndex: 'provider' },
                  { title: '股票', dataIndex: 'symbol' },
                  { title: '扣费', dataIndex: 'cost_credits', render: (value: number) => `-${value}` },
                  { title: '会员额度', dataIndex: 'quota_used', render: (value: number) => value.toLocaleString() },
                  { title: '免费 credits', dataIndex: 'bonus_used', render: (value: number) => value.toLocaleString() },
                ]}
              />
            ) : (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有模型调用记录" />
            )}
          </Card>
        </div>
      </Content>
    </Layout>
  )
}
