import { chromium } from 'playwright'

const baseUrl = process.env.UI_BASE_URL ?? 'http://127.0.0.1:5173'
const apiBaseUrl = process.env.API_BASE_URL ?? 'http://127.0.0.1:8080'
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

const consoleLogs = []
page.on('console', (message) => {
  consoleLogs.push(`${message.type()}: ${message.text()}`)
})

const loginResponse = await fetch(`${apiBaseUrl}/auth/login`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    email: 'trader@example.com',
    password: 'market123456',
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

await page.waitForLoadState('domcontentloaded')
await page.getByText(/Market Copilot/).first().waitFor()

const chartSurface = page.locator('.chart-overlay')
let chartVisible = false
const timeframeSnapshots = []
let allButtons = []
let dragSelectionRange = null

try {
  await chartSurface.waitFor({ state: 'visible', timeout: 15000 })
  chartVisible = true
} catch {}

if (chartVisible) {
  allButtons = await page.getByRole('button').evaluateAll((nodes) =>
    nodes.map((node) => (node.textContent || '').replace(/\s+/g, ' ').trim()),
  )
  const chartCard = page.locator('.panel-card').filter({ hasText: 'Chart Surface' }).first()

  const captureTimeframe = async (name, buttonIndex) => {
    await chartCard.locator('.ant-card-extra button').nth(buttonIndex).click({ force: true })
    await page.waitForTimeout(500)
    const bodyText = await page.locator('body').innerText()
    timeframeSnapshots.push({
      timeframe: name,
      bodyText: bodyText.slice(0, 900),
    })
  }

  await captureTimeframe('日线', 0)
  await captureTimeframe('周线', 1)
  await captureTimeframe('月线', 2)
  await captureTimeframe('日线', 0)

  const box = await chartSurface.boundingBox()
  if (box) {
    await page.mouse.move(box.x + box.width * 0.18, box.y + box.height * 0.55)
    await page.mouse.down()
    await page.mouse.move(box.x + box.width * 0.72, box.y + box.height * 0.55, { steps: 16 })
    await page.mouse.up()
    await page.waitForTimeout(500)
    dragSelectionRange = await page
      .locator('.panel-card')
      .filter({ hasText: 'Selected Range' })
      .first()
      .innerText()
      .catch(() => null)
  }
}

await page.screenshot({
  path: '/mnt/d/project-go/market-project/output/playwright/dashboard-check.png',
  fullPage: true,
})

const title = await page.locator('.panel-card').first().textContent().catch(() => null)
const rangeText = await page.getByText(/Selected Range/).locator('..').textContent().catch(() => null)
const bodyText = await page.locator('body').innerText()

console.log(
  JSON.stringify(
    {
      url: page.url(),
      chartVisible,
      allButtons,
      title,
      rangeText,
      dragSelectionRange,
      bodyText: bodyText.slice(0, 1200),
      timeframeSnapshots,
      consoleLogs,
      screenshot: 'output/playwright/dashboard-check.png',
    },
    null,
    2,
  ),
)

await browser.close()
