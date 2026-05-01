import { chromium } from 'playwright'

const baseUrl = process.env.UI_BASE_URL ?? 'http://127.0.0.1:5177'
const apiBaseUrl = process.env.API_BASE_URL ?? 'http://127.0.0.1:8082'
const loginEmail = process.env.LOGIN_EMAIL ?? 'trader@example.com'
const loginPassword = process.env.LOGIN_PASSWORD ?? 'market123456'
const prompt = process.env.COPILOT_PROMPT ?? '请用一句话判断这个区间的趋势，并给出支撑位、压力位和一条风险提示。'
const executablePath =
  process.env.PLAYWRIGHT_CHROME_PATH ??
  '/home/dd/.cache/ms-playwright/chromium-1208/chrome-linux64/chrome'

const browser = await chromium.launch({
  headless: true,
  executablePath,
})

const page = await browser.newPage({
  viewport: { width: 1600, height: 1100 },
})

const loginResponse = await fetch(`${apiBaseUrl}/auth/login`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    email: loginEmail,
    password: loginPassword,
  }),
})

if (!loginResponse.ok) {
  throw new Error(`Login failed with status ${loginResponse.status}`)
}

const auth = await loginResponse.json()

await page.addInitScript((payload) => {
  window.localStorage.setItem('market-copilot-token', payload.token)
  window.localStorage.setItem('market-copilot-user', JSON.stringify(payload.user))
}, auth)

await page.goto(baseUrl, { waitUntil: 'domcontentloaded' })
await page.waitForLoadState('networkidle')

await page.locator('.copilot-card').waitFor({ state: 'visible', timeout: 20000 })

const composer = page.getByPlaceholder('输入你想问的问题')

try {
  await composer.waitFor({ state: 'visible', timeout: 20000 })
await composer.fill(prompt)
  await page.getByRole('button', { name: '生成分析' }).click()
} catch (error) {
  await page.screenshot({
    path: '/mnt/d/project-go/market-project/output/playwright/copilot-check-failed.png',
    fullPage: true,
  })
  const bodyText = await page.locator('body').innerText().catch(() => '')
  console.log(JSON.stringify({
    error: String(error),
    bodyPreview: bodyText.slice(0, 1200),
    screenshot: 'output/playwright/copilot-check-failed.png',
  }, null, 2))
  throw error
}

await page.getByText('分析中').waitFor({ state: 'visible', timeout: 5000 }).catch(() => null)
await page.locator('.chat-bubble--assistant').last().waitFor({ state: 'visible', timeout: 40000 })
await page.waitForFunction(() => {
  const bubbles = Array.from(document.querySelectorAll('.chat-bubble--assistant'))
  const last = bubbles[bubbles.length - 1]
  return Boolean(last && last.textContent && last.textContent.trim().length > 24)
}, undefined, { timeout: 40000 })

const providerText = await page.locator('.copilot-toolbar .ant-tag').nth(1).innerText().catch(() => null)
const answerText = await page.locator('.chat-bubble--assistant').last().innerText().catch(() => null)

await page.screenshot({
  path: '/mnt/d/project-go/market-project/output/playwright/copilot-check.png',
  fullPage: true,
})

console.log(JSON.stringify({
  url: page.url(),
  providerText,
  answerPreview: answerText?.slice(0, 260) ?? null,
  screenshot: 'output/playwright/copilot-check.png',
}, null, 2))

await browser.close()
