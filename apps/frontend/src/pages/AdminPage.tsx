import { useState } from 'react'
import { Alert, App, Button, Card, Descriptions, Drawer, Flex, Form, Input, InputNumber, Layout, Select, Space, Table, Tag, Typography } from 'antd'
import { ArrowLeftOutlined } from '@ant-design/icons'
import { useQuery } from '@tanstack/react-query'
import { Navigate, useNavigate } from 'react-router-dom'
import { buildQueryErrorAlert } from '../lib/http'
import { batchCreateAdminRedeemCodes, createAdminRedeemCode, disableAdminRedeemCode, fetchAdminActionLogs, fetchAdminRedeemCodeClaims, fetchAdminRedeemCodes, fetchAdminUserDetail, fetchAdminUsers, grantAdminUserBonusCredits, grantAdminUserMembership } from '../lib/api'
import { getStoredUser } from '../lib/auth'

const { Header, Content } = Layout
const { Text, Title, Paragraph } = Typography

type FormValues = {
  code?: string
  reward_type: 'membership' | 'bonus_credits'
  package_id?: 'starter' | 'active' | 'pro'
  bonus_credits?: number
  max_claims: number
  expires_at?: string
}

type BatchFormValues = {
  prefix?: string
  count: number
  reward_type: 'membership' | 'bonus_credits'
  package_id?: 'starter' | 'active' | 'pro'
  bonus_credits?: number
  max_claims: number
  expires_at?: string
}

type GrantCreditsFormValues = {
  amount: number
}

type GrantMembershipFormValues = {
  package_id: 'starter' | 'active' | 'pro'
}

export function AdminPage() {
  const { message } = App.useApp()
  const navigate = useNavigate()
  const user = getStoredUser()
  const [form] = Form.useForm<FormValues>()
  const [batchForm] = Form.useForm<BatchFormValues>()
  const [grantCreditsForm] = Form.useForm<GrantCreditsFormValues>()
  const [grantMembershipForm] = Form.useForm<GrantMembershipFormValues>()
  const [submitting, setSubmitting] = useState(false)
  const [batchSubmitting, setBatchSubmitting] = useState(false)
  const [grantCreditsSubmitting, setGrantCreditsSubmitting] = useState(false)
  const [grantMembershipSubmitting, setGrantMembershipSubmitting] = useState(false)
  const [batchResults, setBatchResults] = useState<string[]>([])
  const [codeSearch, setCodeSearch] = useState('')
  const [codeRewardType, setCodeRewardType] = useState<'all' | 'membership' | 'bonus_credits'>('all')
  const [codeStatus, setCodeStatus] = useState<'all' | 'active' | 'disabled'>('all')
  const [claimSearch, setClaimSearch] = useState('')
  const [actionSearch, setActionSearch] = useState('')
  const [userSearch, setUserSearch] = useState('')
  const [membershipStatus, setMembershipStatus] = useState<'all' | 'member' | 'non_member'>('all')
  const [selectedUserID, setSelectedUserID] = useState<string | null>(null)
  const rewardType = Form.useWatch('reward_type', form) ?? 'membership'
  const batchRewardType = Form.useWatch('reward_type', batchForm) ?? 'membership'

  async function handleCopyBatchResults() {
    if (!batchResults.length) {
      message.warning('当前没有可复制的兑换码')
      return
    }
    try {
      await navigator.clipboard.writeText(batchResults.join('\n'))
      message.success(`已复制 ${batchResults.length} 个兑换码`)
    } catch (error) {
      console.error(error)
      message.error('复制失败，请手动复制')
    }
  }

  function downloadBatchResults(filename: string, content: string, mimeType: string) {
    const blob = new Blob([content], { type: mimeType })
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = filename
    link.click()
    window.URL.revokeObjectURL(url)
  }

  function handleExportBatchResultsAsText() {
    if (!batchResults.length) {
      message.warning('当前没有可导出的兑换码')
      return
    }
    downloadBatchResults('redeem-codes.txt', `${batchResults.join('\n')}\n`, 'text/plain;charset=utf-8')
    message.success('已导出 TXT')
  }

  function handleExportBatchResultsAsCSV() {
    if (!batchResults.length) {
      message.warning('当前没有可导出的兑换码')
      return
    }
    const rows = ['code', ...batchResults]
    downloadBatchResults('redeem-codes.csv', `${rows.join('\n')}\n`, 'text/csv;charset=utf-8')
    message.success('已导出 CSV')
  }

  const codesQuery = useQuery({
    queryKey: ['admin-redeem-codes', codeSearch, codeRewardType, codeStatus],
    queryFn: () => fetchAdminRedeemCodes({
      search: codeSearch || undefined,
      reward_type: codeRewardType === 'all' ? undefined : codeRewardType,
      status: codeStatus === 'all' ? undefined : codeStatus,
    }),
  })
  const claimsQuery = useQuery({
    queryKey: ['admin-redeem-code-claims', claimSearch],
    queryFn: () => fetchAdminRedeemCodeClaims({
      search: claimSearch || undefined,
    }),
  })
  const actionLogsQuery = useQuery({
    queryKey: ['admin-action-logs', actionSearch],
    queryFn: () => fetchAdminActionLogs({
      search: actionSearch || undefined,
    }),
  })
  const usersQuery = useQuery({
    queryKey: ['admin-users', userSearch, membershipStatus],
    queryFn: () => fetchAdminUsers({
      search: userSearch || undefined,
      membership_status: membershipStatus === 'all' ? undefined : membershipStatus,
    }),
  })
  const userDetailQuery = useQuery({
    queryKey: ['admin-user-detail', selectedUserID],
    queryFn: () => fetchAdminUserDetail(selectedUserID ?? ''),
    enabled: !!selectedUserID,
  })
  const adminError = buildQueryErrorAlert(
    codesQuery.error ?? claimsQuery.error ?? actionLogsQuery.error ?? usersQuery.error,
    { dataSourceName: '后台数据' },
  )
  const userDetailError = buildQueryErrorAlert(userDetailQuery.error, {
    dataSourceName: '用户详情',
  })

  if (!user?.is_admin) {
    return <Navigate to="/" replace />
  }

  async function handleFinish(values: FormValues) {
    setSubmitting(true)
    try {
      const created = await createAdminRedeemCode({
        code: values.code,
        reward_type: values.reward_type,
        package_id: values.package_id,
        bonus_credits: values.bonus_credits,
        max_claims: values.max_claims,
        expires_at: values.expires_at,
      })
      message.success(`已生成兑换码 ${created.code}`)
      form.resetFields()
      form.setFieldsValue({ reward_type: 'membership', package_id: 'starter', max_claims: 1 })
      await Promise.all([codesQuery.refetch(), actionLogsQuery.refetch()])
    } catch (error) {
      console.error(error)
      message.error('生成兑换码失败')
    } finally {
      setSubmitting(false)
    }
  }

  async function handleDisable(code: string) {
    try {
      await disableAdminRedeemCode(code)
      message.success(`已禁用 ${code}`)
      await Promise.all([codesQuery.refetch(), claimsQuery.refetch(), actionLogsQuery.refetch()])
    } catch (error) {
      console.error(error)
      message.error('禁用兑换码失败')
    }
  }

  async function handleBatchFinish(values: BatchFormValues) {
    setBatchSubmitting(true)
    try {
      const created = await batchCreateAdminRedeemCodes({
        prefix: values.prefix,
        count: values.count,
        reward_type: values.reward_type,
        package_id: values.package_id,
        bonus_credits: values.bonus_credits,
        max_claims: values.max_claims,
        expires_at: values.expires_at,
      })
      setBatchResults(created.map((item) => item.code))
      message.success(`已批量生成 ${created.length} 个兑换码`)
      batchForm.resetFields()
      batchForm.setFieldsValue({ reward_type: 'membership', package_id: 'starter', max_claims: 1, count: 10, prefix: 'BATCH' })
      await Promise.all([codesQuery.refetch(), actionLogsQuery.refetch()])
    } catch (error) {
      console.error(error)
      message.error('批量生成兑换码失败')
    } finally {
      setBatchSubmitting(false)
    }
  }

  async function handleGrantCredits(values: GrantCreditsFormValues) {
    if (!selectedUserID) return
    setGrantCreditsSubmitting(true)
    try {
      const result = await grantAdminUserBonusCredits(selectedUserID, { amount: values.amount })
      message.success(`已补充免费 credits，当前余额 ${result.credit_balance.toLocaleString()}`)
      grantCreditsForm.resetFields()
      grantCreditsForm.setFieldsValue({ amount: 1000 })
      await Promise.all([userDetailQuery.refetch(), usersQuery.refetch(), actionLogsQuery.refetch()])
    } catch (error) {
      console.error(error)
      message.error('补充免费 credits 失败')
    } finally {
      setGrantCreditsSubmitting(false)
    }
  }

  async function handleGrantMembership(values: GrantMembershipFormValues) {
    if (!selectedUserID) return
    setGrantMembershipSubmitting(true)
    try {
      await grantAdminUserMembership(selectedUserID, { package_id: values.package_id })
      message.success('会员已发放')
      grantMembershipForm.resetFields()
      grantMembershipForm.setFieldsValue({ package_id: 'starter' })
      await Promise.all([userDetailQuery.refetch(), usersQuery.refetch(), actionLogsQuery.refetch()])
    } catch (error) {
      console.error(error)
      message.error('发放会员失败')
    } finally {
      setGrantMembershipSubmitting(false)
    }
  }

  return (
    <Layout className="billing-shell">
      <Header className="billing-header">
        <Flex justify="space-between" align="center" gap={12}>
          <div>
            <Text type="secondary">Admin</Text>
            <Title level={2} style={{ margin: 0 }}>兑换码后台</Title>
          </div>
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate('/')}>
            返回工作台
          </Button>
        </Flex>
      </Header>
      <Content className="billing-content">
        {codesQuery.error || claimsQuery.error || actionLogsQuery.error || usersQuery.error ? (
          <Alert
            type="error"
            title={adminError.title}
            description={adminError.description}
            showIcon
          />
        ) : null}
        <div className="billing-grid">
          <Card className="panel-card" title="生成兑换码">
            <Form<FormValues>
              form={form}
              layout="vertical"
              initialValues={{ reward_type: 'membership', package_id: 'starter', max_claims: 1 }}
              onFinish={(values) => void handleFinish(values)}
            >
              <Form.Item label="奖励类型" name="reward_type">
                <Select
                  options={[
                    { label: '会员档位', value: 'membership' },
                    { label: '免费 credits', value: 'bonus_credits' },
                  ]}
                />
              </Form.Item>
              <Form.Item label="自定义兑换码" name="code">
                <Input placeholder="留空则自动生成" />
              </Form.Item>
              {rewardType === 'membership' ? (
                <Form.Item label="会员档位" name="package_id" rules={[{ required: true, message: '请选择会员档位' }]}>
                  <Select
                    options={[
                      { label: 'Starter', value: 'starter' },
                      { label: 'Active', value: 'active' },
                      { label: 'Pro', value: 'pro' },
                    ]}
                  />
                </Form.Item>
              ) : (
                <Form.Item label="免费 credits" name="bonus_credits" rules={[{ required: true, message: '请输入点数' }]}>
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
              )}
              <Form.Item label="可领取次数" name="max_claims" rules={[{ required: true, message: '请输入次数' }]}>
                <InputNumber min={1} style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item label="过期时间" name="expires_at">
                <Input type="datetime-local" />
              </Form.Item>
              <Button htmlType="submit" type="primary" loading={submitting}>
                生成兑换码
              </Button>
            </Form>
            <Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
              这版后台先做最小运营能力，当前支持生成、禁用、查看最近领取记录。
            </Paragraph>
          </Card>

          <Card className="panel-card" title="批量生成兑换码">
            <Form<BatchFormValues>
              form={batchForm}
              layout="vertical"
              initialValues={{ reward_type: 'membership', package_id: 'starter', max_claims: 1, count: 10, prefix: 'BATCH' }}
              onFinish={(values) => void handleBatchFinish(values)}
            >
              <Form.Item label="奖励类型" name="reward_type">
                <Select
                  options={[
                    { label: '会员档位', value: 'membership' },
                    { label: '免费 credits', value: 'bonus_credits' },
                  ]}
                />
              </Form.Item>
              <Form.Item label="前缀" name="prefix">
                <Input placeholder="例如 BATCH / SPRING" />
              </Form.Item>
              <Form.Item label="数量" name="count" rules={[{ required: true, message: '请输入数量' }]}>
                <InputNumber min={1} max={100} style={{ width: '100%' }} />
              </Form.Item>
              {batchRewardType === 'membership' ? (
                <Form.Item label="会员档位" name="package_id" rules={[{ required: true, message: '请选择会员档位' }]}>
                  <Select
                    options={[
                      { label: 'Starter', value: 'starter' },
                      { label: 'Active', value: 'active' },
                      { label: 'Pro', value: 'pro' },
                    ]}
                  />
                </Form.Item>
              ) : (
                <Form.Item label="免费 credits" name="bonus_credits" rules={[{ required: true, message: '请输入点数' }]}>
                  <InputNumber min={1} style={{ width: '100%' }} />
                </Form.Item>
              )}
              <Form.Item label="每个码可领取次数" name="max_claims" rules={[{ required: true, message: '请输入次数' }]}>
                <InputNumber min={1} style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item label="统一过期时间" name="expires_at">
                <Input type="datetime-local" />
              </Form.Item>
              <Button htmlType="submit" type="primary" loading={batchSubmitting}>
                批量生成
              </Button>
            </Form>
            {batchResults.length ? (
              <div style={{ marginTop: 16 }}>
                <Flex justify="space-between" align="center" gap={12} wrap="wrap">
                  <Text strong>本次结果</Text>
                  <Space wrap>
                    <Button onClick={() => void handleCopyBatchResults()}>
                      复制全部
                    </Button>
                    <Button onClick={handleExportBatchResultsAsText}>
                      导出 TXT
                    </Button>
                    <Button onClick={handleExportBatchResultsAsCSV}>
                      导出 CSV
                    </Button>
                  </Space>
                </Flex>
                <Input.TextArea
                  readOnly
                  value={batchResults.join('\n')}
                  autoSize={{ minRows: 6, maxRows: 12 }}
                  style={{ marginTop: 8 }}
                />
              </div>
            ) : null}
          </Card>

          <Card className="panel-card" title="最近兑换码">
            <Space style={{ width: '100%', marginBottom: 12 }} wrap>
              <Input
                value={codeSearch}
                onChange={(event) => setCodeSearch(event.target.value)}
                placeholder="搜索兑换码"
                style={{ width: 220 }}
              />
              <Select
                value={codeRewardType}
                onChange={setCodeRewardType}
                style={{ width: 160 }}
                options={[
                  { label: '全部类型', value: 'all' },
                  { label: '会员档位', value: 'membership' },
                  { label: '免费 credits', value: 'bonus_credits' },
                ]}
              />
              <Select
                value={codeStatus}
                onChange={setCodeStatus}
                style={{ width: 140 }}
                options={[
                  { label: '全部状态', value: 'all' },
                  { label: '启用', value: 'active' },
                  { label: '已禁用', value: 'disabled' },
                ]}
              />
            </Space>
            <Table
              rowKey="code"
              size="small"
              pagination={false}
              dataSource={codesQuery.data ?? []}
              columns={[
                { title: 'Code', dataIndex: 'code' },
                { title: '类型', dataIndex: 'reward_type' },
                {
                  title: '奖励',
                  key: 'reward',
                  render: (_, record) => record.reward_type === 'membership'
                    ? `${record.package_name} / ${record.daily_quota} / ${record.duration_days}天`
                    : `${record.bonus_credits} credits`,
                },
                { title: '领取', key: 'claims', render: (_, record) => `${record.claimed_count}/${record.max_claims}` },
                { title: '状态', key: 'status', render: (_, record) => record.is_active ? '启用' : '已禁用' },
                { title: '过期时间', dataIndex: 'expires_at', render: (value: string) => value || '长期有效' },
                { title: '创建时间', dataIndex: 'created_at' },
                {
                  title: '操作',
                  key: 'action',
                  render: (_, record) => record.is_active ? (
                    <Button size="small" danger onClick={() => void handleDisable(record.code)}>
                      禁用
                    </Button>
                  ) : null,
                },
              ]}
            />
            {!codesQuery.data?.length ? (
              <Space style={{ marginTop: 12 }}>
                <Text type="secondary">还没有后台生成的兑换码。</Text>
              </Space>
            ) : null}
          </Card>

          <Card className="panel-card" title="最近领取记录">
            <Input
              value={claimSearch}
              onChange={(event) => setClaimSearch(event.target.value)}
              placeholder="搜索兑换码或用户邮箱"
              style={{ width: 260, marginBottom: 12 }}
            />
            <Table
              rowKey={(record) => `${record.code}-${record.user_email}-${record.created_at}`}
              size="small"
              pagination={false}
              dataSource={claimsQuery.data ?? []}
              columns={[
                { title: 'Code', dataIndex: 'code' },
                { title: '类型', dataIndex: 'reward_type' },
                { title: '用户', dataIndex: 'user_email' },
                {
                  title: '领取内容',
                  key: 'reward',
                  render: (_, record) => record.reward_type === 'membership'
                    ? record.package_name
                    : `${record.bonus_credits} credits`,
                },
                { title: '时间', dataIndex: 'created_at' },
              ]}
            />
          </Card>

          <Card className="panel-card" title="最近操作记录">
            <Input
              value={actionSearch}
              onChange={(event) => setActionSearch(event.target.value)}
              placeholder="搜索管理员、动作、目标或摘要"
              style={{ width: 320, marginBottom: 12 }}
            />
            <Table
              rowKey="id"
              size="small"
              pagination={false}
              dataSource={actionLogsQuery.data ?? []}
              columns={[
                { title: '时间', dataIndex: 'created_at' },
                { title: '管理员', dataIndex: 'admin_email' },
                { title: '动作', dataIndex: 'action_type' },
                { title: '目标类型', dataIndex: 'target_type' },
                { title: '目标', dataIndex: 'target_id', render: (value: string) => value || '-' },
                { title: '摘要', dataIndex: 'description' },
              ]}
            />
          </Card>

          <Card className="panel-card" title="最近用户">
            <Space style={{ width: '100%', marginBottom: 12 }} wrap>
              <Input
                value={userSearch}
                onChange={(event) => setUserSearch(event.target.value)}
                placeholder="搜索用户邮箱"
                style={{ width: 240 }}
              />
              <Select
                value={membershipStatus}
                onChange={setMembershipStatus}
                style={{ width: 160 }}
                options={[
                  { label: '全部用户', value: 'all' },
                  { label: '有会员', value: 'member' },
                  { label: '无会员', value: 'non_member' },
                ]}
              />
            </Space>
            <Table
              rowKey="user_id"
              size="small"
              pagination={false}
              dataSource={usersQuery.data ?? []}
              columns={[
                { title: '邮箱', dataIndex: 'email' },
                { title: '管理员', dataIndex: 'is_admin', render: (value: boolean) => value ? '是' : '否' },
                { title: '免费 credits', dataIndex: 'credit_balance', render: (value: number) => value.toLocaleString() },
                { title: '当前会员', dataIndex: 'current_package', render: (value: string) => value || '-' },
                { title: '日额度', dataIndex: 'daily_quota', render: (value: number) => value ? value.toLocaleString() : '-' },
                { title: '会员到期', dataIndex: 'membership_ends_at', render: (value: string) => value || '-' },
                { title: '注册时间', dataIndex: 'created_at' },
                {
                  title: '操作',
                  key: 'action',
                  render: (_, record) => (
                    <Button size="small" onClick={() => setSelectedUserID(record.user_id)}>
                      查看详情
                    </Button>
                  ),
                },
              ]}
            />
          </Card>
        </div>
        <Drawer
          title="用户详情"
          width={760}
          open={!!selectedUserID}
          onClose={() => setSelectedUserID(null)}
          destroyOnClose
        >
          {userDetailQuery.error ? (
            <Alert
              type="error"
              title={userDetailError.title}
              description={userDetailError.description}
              showIcon
              style={{ marginBottom: 16 }}
            />
          ) : null}
          {userDetailQuery.data ? (
            <Space direction="vertical" size={16} style={{ width: '100%' }}>
              <Card size="small" title="运营动作">
                <Space direction="vertical" size={16} style={{ width: '100%' }}>
                  <Form<GrantCreditsFormValues>
                    form={grantCreditsForm}
                    layout="inline"
                    initialValues={{ amount: 1000 }}
                    onFinish={(values) => void handleGrantCredits(values)}
                  >
                    <Form.Item
                      label="补免费 credits"
                      name="amount"
                      rules={[{ required: true, message: '请输入点数' }]}
                    >
                      <InputNumber min={1} />
                    </Form.Item>
                    <Form.Item>
                      <Button type="primary" htmlType="submit" loading={grantCreditsSubmitting}>
                        发放
                      </Button>
                    </Form.Item>
                  </Form>

                  <Form<GrantMembershipFormValues>
                    form={grantMembershipForm}
                    layout="inline"
                    initialValues={{ package_id: 'starter' }}
                    onFinish={(values) => void handleGrantMembership(values)}
                  >
                    <Form.Item
                      label="直接发会员"
                      name="package_id"
                      rules={[{ required: true, message: '请选择档位' }]}
                    >
                      <Select
                        style={{ width: 160 }}
                        options={[
                          { label: 'Starter', value: 'starter' },
                          { label: 'Active', value: 'active' },
                          { label: 'Pro', value: 'pro' },
                        ]}
                      />
                    </Form.Item>
                    <Form.Item>
                      <Button type="primary" htmlType="submit" loading={grantMembershipSubmitting}>
                        发放
                      </Button>
                    </Form.Item>
                  </Form>
                </Space>
              </Card>

              <Descriptions bordered column={1} size="small" title="基础信息">
                <Descriptions.Item label="邮箱">{userDetailQuery.data.email}</Descriptions.Item>
                <Descriptions.Item label="管理员">
                  {userDetailQuery.data.is_admin ? <Tag color="gold">Admin</Tag> : '否'}
                </Descriptions.Item>
                <Descriptions.Item label="免费 credits">
                  {userDetailQuery.data.credit_balance.toLocaleString()}
                </Descriptions.Item>
                <Descriptions.Item label="注册时间">{userDetailQuery.data.created_at}</Descriptions.Item>
                <Descriptions.Item label="当前会员">
                  {userDetailQuery.data.current_membership
                    ? `${userDetailQuery.data.current_membership.package_name} / ${userDetailQuery.data.current_membership.ends_at}`
                    : '无'}
                </Descriptions.Item>
                <Descriptions.Item label="今日额度">
                  {`${userDetailQuery.data.today_quota.used.toLocaleString()} / ${userDetailQuery.data.today_quota.total.toLocaleString()}，剩余 ${userDetailQuery.data.today_quota.remaining.toLocaleString()}`}
                </Descriptions.Item>
              </Descriptions>

              <Card size="small" title="最近会员记录">
                <Table
                  rowKey="id"
                  size="small"
                  pagination={false}
                  dataSource={userDetailQuery.data.memberships}
                  columns={[
                    { title: '档位', dataIndex: 'package_name' },
                    { title: '状态', dataIndex: 'status' },
                    { title: '日额度', dataIndex: 'daily_quota', render: (value: number) => value.toLocaleString() },
                    { title: '时长', dataIndex: 'duration_days', render: (value: number) => `${value}天` },
                    { title: '开始', dataIndex: 'starts_at' },
                    { title: '结束', dataIndex: 'ends_at' },
                  ]}
                />
              </Card>

              <Card size="small" title="最近兑换记录">
                <Table
                  rowKey={(record) => `${record.code}-${record.created_at}`}
                  size="small"
                  pagination={false}
                  dataSource={userDetailQuery.data.redeem_claims}
                  columns={[
                    { title: 'Code', dataIndex: 'code' },
                    { title: '类型', dataIndex: 'reward_type' },
                    {
                      title: '奖励',
                      key: 'reward',
                      render: (_, record) => record.reward_type === 'membership'
                        ? record.package_name
                        : `${record.bonus_credits} credits`,
                    },
                    { title: '时间', dataIndex: 'created_at' },
                  ]}
                />
              </Card>

              <Card size="small" title="最近 AI 用量">
                <Table
                  rowKey="id"
                  size="small"
                  pagination={false}
                  dataSource={userDetailQuery.data.recent_usage}
                  columns={[
                    { title: 'Provider', dataIndex: 'provider' },
                    { title: 'Symbol', dataIndex: 'symbol' },
                    { title: '总消耗', dataIndex: 'cost_credits', render: (value: number) => value.toLocaleString() },
                    { title: '会员额度', dataIndex: 'quota_used', render: (value: number) => value.toLocaleString() },
                    { title: '免费额度', dataIndex: 'bonus_used', render: (value: number) => value.toLocaleString() },
                    { title: '时间', dataIndex: 'created_at' },
                  ]}
                />
              </Card>
            </Space>
          ) : null}
        </Drawer>
      </Content>
    </Layout>
  )
}
