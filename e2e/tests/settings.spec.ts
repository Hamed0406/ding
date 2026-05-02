import { test, expect } from '@playwright/test'

function uniqueEmail(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}@example.com`
}

async function registerAndGoToSettings(page: import('@playwright/test').Page, email: string) {
  await page.goto('/')
  await page.waitForSelector('input[type="email"]')
  await page.fill('input[type="email"]', email)
  const pwFields = page.locator('input[type="password"]')
  await pwFields.nth(0).fill('password123')
  const count = await pwFields.count()
  if (count > 1) await pwFields.nth(1).fill('password123')
  await page.getByRole('button', { name: /create account|sign in/i }).first().click()
  await page.waitForURL('/')

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
    // Email settings form should show host/port fields
    await expect(page.locator('input[placeholder*="host" i], input[placeholder*="smtp" i], label:has-text("host"), label:has-text("Host")')).toBeVisible({ timeout: 5000 })
  })

  test('can navigate to Speed Test settings', async ({ page }) => {
    await page.getByRole('button', { name: /speed test/i }).click()
    // Speed test page has a "Run Speed Test" button
    await expect(page.getByRole('button', { name: /run speed test/i })).toBeVisible({ timeout: 5000 })
  })

  test('telegram form has token and chat id fields', async ({ page }) => {
    // Telegram is the default active section
    // Look for token-related input or button
    await expect(
      page.locator('input[placeholder*="token" i], button:has-text("Change token"), button:has-text("Enter token")')
    ).toBeVisible({ timeout: 5000 })
    // Chat ID field
    await expect(
      page.locator('input[placeholder*="chat" i]')
    ).toBeVisible({ timeout: 5000 })
  })

  test('email form has host and port fields', async ({ page }) => {
    await page.getByRole('button', { name: /^email$/i }).click()
    // Host field
    await expect(page.locator('input[placeholder*="host" i], input[id*="host" i]')).toBeVisible({ timeout: 5000 })
    // Port field — look for number input or labeled port
    await expect(page.locator('input[type="number"], input[placeholder*="port" i], input[id*="port" i]')).toBeVisible({ timeout: 5000 })
  })

  test('email form switches to Self-hosted mode', async ({ page }) => {
    await page.getByRole('button', { name: /^email$/i }).click()

    // Click the Self-hosted toggle/tab
    const selfHostedBtn = page.getByRole('button', { name: /self.?host/i })
    await expect(selfHostedBtn).toBeVisible({ timeout: 5000 })
    await selfHostedBtn.click()

    // After switching, the host should default to localhost or port to 1025
    await expect(
      page.locator('input[value="localhost"], input[value="1025"]')
    ).toBeVisible({ timeout: 5000 })
  })
})
