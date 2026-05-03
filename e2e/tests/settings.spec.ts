import { test, expect } from '@playwright/test'

function uniqueEmail(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}@example.com`
}

async function registerAndGoToSettings(page: import('@playwright/test').Page, email: string) {
  await page.goto('/')
  await page.waitForSelector('input[type="email"]')
  const createTab = page.locator('button[type="button"]').filter({ hasText: 'Create account' })
  if (await createTab.isVisible({ timeout: 2000 }).catch(() => false)) await createTab.click()
  await page.fill('input[type="email"]', email)
  const pwFields = page.locator('input[type="password"]')
  await pwFields.nth(0).fill('password123')
  const count = await pwFields.count()
  if (count > 1) await pwFields.nth(1).fill('password123')
  await page.locator('button[type="submit"]').click()
  await page.locator('button[title="Sign out"]').waitFor({ timeout: 10000 })

  // Navigate to settings via gear icon
  await page.locator('button[title="Settings"]').click()
  await expect(page.getByRole('heading', { name: /settings/i })).toBeVisible()
}

test.describe('settings', () => {
  test.beforeEach(async ({ page }) => {
    const email = uniqueEmail('settings')
    await registerAndGoToSettings(page, email)
  })

  test('settings page has Telegram section', async ({ page }) => {
    // Telegram is shown in the sidebar nav
    await expect(page.getByRole('button', { name: /telegram/i })).toBeVisible()
  })

  test('settings page has Email section', async ({ page }) => {
    await expect(page.getByRole('button', { name: /^email$/i })).toBeVisible()
  })

  test('settings page has Webhooks section', async ({ page }) => {
    await expect(page.getByRole('button', { name: /webhooks/i })).toBeVisible()
  })

  test('settings page has Speed Test section', async ({ page }) => {
    await expect(page.getByRole('button', { name: /speed test/i })).toBeVisible()
  })

  test('can navigate to Email settings', async ({ page }) => {
    await page.getByRole('button', { name: /^email$/i }).click()
    // Email settings form should show SMTP host field
    await expect(page.locator('input[placeholder="smtp.gmail.com"]')).toBeVisible({ timeout: 5000 })
  })

  test('can navigate to Speed Test settings', async ({ page }) => {
    await page.getByRole('button', { name: /speed test/i }).click()
    // Speed test page has a "Run Speed Test" button
    await expect(page.getByRole('button', { name: /run speed test/i })).toBeVisible({ timeout: 5000 })
  })

  test('telegram form has token and chat id fields', async ({ page }) => {
    // Telegram is the default active section
    // Token input placeholder is "123456789:ABCdef…"
    await expect(
      page.locator('input[placeholder*="ABCdef"], button:has-text("Change"), input[type="password"]').first()
    ).toBeVisible({ timeout: 5000 })
    // Chat ID field placeholder is "987654321"
    await expect(
      page.locator('input[placeholder="987654321"]')
    ).toBeVisible({ timeout: 5000 })
  })

  test('email form has host and port fields', async ({ page }) => {
    await page.getByRole('button', { name: /^email$/i }).click()
    // Host field placeholder is "smtp.gmail.com"
    await expect(page.locator('input[placeholder="smtp.gmail.com"]')).toBeVisible({ timeout: 5000 })
    // Port field is a number input
    await expect(page.locator('input[type="number"]')).toBeVisible({ timeout: 5000 })
  })

  test('email form switches to Self-hosted mode', async ({ page }) => {
    await page.getByRole('button', { name: /^email$/i }).click()

    // Click the Self-hosted toggle/tab
    const selfHostedBtn = page.getByRole('button', { name: /self.?host/i })
    await expect(selfHostedBtn).toBeVisible({ timeout: 5000 })
    await selfHostedBtn.click()

    // After switching, host defaults to "localhost"
    await expect(page.locator('input[placeholder="localhost"]')).toBeVisible({ timeout: 5000 })
  })
})
