export interface LoginPayload {
  email: string
  password: string
}

export interface LoginResponse {
  token: string
  user: {
    id: string
    email: string
    is_admin: boolean
  }
}

export interface SymbolRecord {
  symbol: string
  name: string
  market: string
  source: string
}

export interface OHLCRow {
  symbol: string
  market: string
  date: string
  open: number
  high: number
  low: number
  close: number
  volume: number
  amount?: number
  change_rate?: number
}

export interface CopilotQueryPayload {
  symbol: string
  start_date?: string
  end_date?: string
  provider?: string
  question: string
  history: Array<{
    role: string
    content: string
  }>
}

export interface CopilotResponse {
  session_id?: string
  session_date?: string
  answer: string
  bias: 'bullish' | 'bearish' | 'neutral' | string
  key_points: string[]
  risk_points: string[]
  watch_items: string[]
  levels: {
    support: {
      value: number
      reason: string
    }
    pressure: {
      value: number
      reason: string
    }
    risk: {
      value: number
      reason: string
    }
  }
  news_context: {
    used: boolean
    count: number
    note?: string
    items: Array<{
      title: string
      source: string
      published_at: string
      url: string
      summary: string
      relevance_reason: string
    }>
  }
}

export interface ChatMessage {
  role: 'user' | 'assistant' | string
  content: string
}

export interface CopilotSessionSummary {
  id: string
  session_date: string
  symbol: string
  message_count: number
  updated_at: string
  title: string
  summary: string
  is_favorite: boolean
}

export interface CopilotSessionMessagesResponse {
  session: CopilotSessionSummary
  messages: ChatMessage[]
}

export interface AIProviderInfo {
  id: string
  name: string
  model: string
  enabled: boolean
  is_default: boolean
}

export interface BillingPackage {
  id: string
  name: string
  amount_cny: number
  daily_quota: number
  duration_days: number
  description: string
}

export interface MembershipStatus {
  package_id: string
  package_name: string
  status: string
  daily_quota: number
  duration_days: number
  starts_at: string
  ends_at: string
}

export interface DailyQuotaStatus {
  date: string
  total: number
  used: number
  remaining: number
}

export interface BillingSummary {
  credit_balance: number
  current_membership?: MembershipStatus
  today_quota: DailyQuotaStatus
  packages: BillingPackage[]
  orders: RechargeOrderResponse[]
  usage: Array<{
    id: string
    provider: string
    symbol: string
    cost_credits: number
    quota_used: number
    bonus_used: number
    created_at: string
  }>
}

export interface RechargeOrderPayload {
  package_id: string
  payment_method: 'alipay' | 'wechat'
}

export interface RedeemCodePayload {
  code: string
}

export interface RedeemCodeResponse {
  code: string
  reward_type: string
  bonus_credits: number
  credit_balance: number
  activated_membership?: MembershipStatus
  message: string
}

export interface RechargeOrderResponse {
  order_id: string
  status: string
  package_id: string
  payment_method: string
  amount_cny: number
  daily_quota: number
  duration_days: number
  payment_url?: string
  qr_code?: string
  mock_pay_ready: boolean
  pay_hint: string
}

export interface AdminCreateRedeemCodePayload {
  code?: string
  reward_type: 'membership' | 'bonus_credits'
  bonus_credits?: number
  package_id?: string
  max_claims: number
  expires_at?: string
}

export interface AdminBatchCreateRedeemCodePayload {
  prefix?: string
  count: number
  reward_type: 'membership' | 'bonus_credits'
  bonus_credits?: number
  package_id?: string
  max_claims: number
  expires_at?: string
}

export interface AdminRedeemCode {
  code: string
  reward_type: string
  bonus_credits: number
  package_id: string
  package_name: string
  daily_quota: number
  duration_days: number
  max_claims: number
  claimed_count: number
  is_active: boolean
  expires_at: string
  created_at: string
}

export interface AdminRedeemCodeClaim {
  code: string
  reward_type: string
  user_email: string
  bonus_credits: number
  package_name: string
  created_at: string
}

export interface AdminUserSummary {
  user_id: string
  email: string
  is_admin: boolean
  credit_balance: number
  current_package: string
  daily_quota: number
  membership_ends_at: string
  created_at: string
}

export interface AdminUserMembershipRecord {
  id: string
  package_id: string
  package_name: string
  status: string
  daily_quota: number
  duration_days: number
  starts_at: string
  ends_at: string
  created_at: string
}

export interface AdminUserRedeemClaim {
  code: string
  reward_type: string
  bonus_credits: number
  package_name: string
  created_at: string
}

export interface AdminUserDetail {
  user_id: string
  email: string
  is_admin: boolean
  credit_balance: number
  created_at: string
  current_membership?: MembershipStatus
  today_quota: DailyQuotaStatus
  recent_usage: Array<{
    id: string
    provider: string
    symbol: string
    cost_credits: number
    quota_used: number
    bonus_used: number
    created_at: string
  }>
  memberships: AdminUserMembershipRecord[]
  redeem_claims: AdminUserRedeemClaim[]
}

export interface AdminActionLog {
  id: string
  admin_email: string
  action_type: string
  target_type: string
  target_id: string
  description: string
  created_at: string
}

export interface AdminGrantCreditsPayload {
  amount: number
}

export interface AdminGrantMembershipPayload {
  package_id: 'starter' | 'active' | 'pro'
}
