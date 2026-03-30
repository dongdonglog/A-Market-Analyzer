import { useState } from 'react'
import { Button, Card, Form, Input, Segmented, Space, Typography, message } from 'antd'
import { Navigate, useNavigate } from 'react-router-dom'
import { login, register } from '../lib/api'
import { getStoredToken, storeAuth } from '../lib/auth'

const { Paragraph, Text, Title } = Typography

export function LoginPage() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [messageApi, contextHolder] = message.useMessage()

  if (getStoredToken()) {
    return <Navigate to="/" replace />
  }

  async function handleFinish(values: { email: string; password: string }) {
    setLoading(true)
    try {
      const payload = mode === 'login' ? await login(values) : await register(values)
      storeAuth(payload)
      messageApi.success(mode === 'login' ? '登录成功，正在进入工作台' : '注册成功，正在进入工作台')
      navigate('/', { replace: true })
    } catch (error) {
      console.error(error)
      messageApi.error(mode === 'login' ? '登录失败，请确认账号或 backend 服务' : '注册失败，请确认邮箱是否已存在')
    } finally {
      setLoading(false)
    }
  }

  return (
    <>
      {contextHolder}
      <div className="login-shell">
        <section className="login-hero">
          <div>
            <span className="hero-kicker">Eastmoney Data + AI Outlook</span>
            <h1 className="hero-title">Market Copilot</h1>
            <p className="hero-subtitle">
              今晚的版本只做最短链路: 登录、选股、看图、选区间、AI 解释与预判。
              数据源固定东方财富，先用 2 到 3 个股票打通整条链路。
            </p>
          </div>
          <div className="hero-metrics">
            <div className="hero-metric">
              <span className="hero-metric-label">Source</span>
              <span className="hero-metric-value">Eastmoney</span>
            </div>
            <div className="hero-metric">
              <span className="hero-metric-label">Stack</span>
              <span className="hero-metric-value">Go + React</span>
            </div>
            <div className="hero-metric">
              <span className="hero-metric-label">Output</span>
              <span className="hero-metric-value">Explain + Bias</span>
            </div>
          </div>
        </section>

        <Card className="login-card">
          <Space orientation="vertical" size="large" style={{ width: '100%' }}>
            <div>
              <Text type="secondary">Protected Dashboard</Text>
              <Title level={2} style={{ marginTop: 8 }}>
                {mode === 'login' ? '登录工作台' : '注册账号'}
              </Title>
              <Paragraph type="secondary" style={{ marginBottom: 0 }}>
                支持 demo 登录，也支持直接注册新用户。新用户会获得初始 credits。
              </Paragraph>
            </div>
            <Segmented
              block
              value={mode}
              onChange={(value) => setMode(value as 'login' | 'register')}
              options={[
                { label: '登录', value: 'login' },
                { label: '注册', value: 'register' },
              ]}
            />

            <Form
              layout="vertical"
              initialValues={{
                email: 'demo@example.com',
                password: 'demo123456',
              }}
              onFinish={handleFinish}
            >
              <Form.Item
                label="邮箱"
                name="email"
                rules={[{ required: true, message: '请输入邮箱' }]}
              >
                <Input size="large" />
              </Form.Item>
              <Form.Item
                label="密码"
                name="password"
                rules={[{ required: true, message: '请输入密码' }]}
              >
                <Input.Password size="large" />
              </Form.Item>
              <Button type="primary" htmlType="submit" size="large" block loading={loading}>
                {mode === 'login' ? '进入工作台' : '注册并进入'}
              </Button>
            </Form>
          </Space>
        </Card>
      </div>
    </>
  )
}
