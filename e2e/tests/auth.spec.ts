import { test, expect } from '@playwright/test'

// Helper: generate unique email for each test to avoid conflicts
function uniqueEmail(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}@example.com`
}

// Helper: register a user via the form
async function registerUser(page: import('@playwright/test').Page, email: string, password: string) {
  await page.goto('/')
  // Wait for the login page to appear
  await page.waitForSelector('input[type="email"]')

  await page.fill('input[type="email"]', email)
  await page.fill('input[type="password"]', password)

  // First user → form is in register mode; subsequent users need to switch to register
  const registerBtn = page.getByRole('button', { name: /create account/i })
  const tabRegister = page.getByRole('button', { name: /create account/i })

  if (await tabRegister.isVisible({ timeout: 1000 }).catch(() => false)) {
    await tabRegister.click()
    // After switching, re-fill because a confirm field may appear
    await page.fill('input[type="email"]', email)
    const pwFields = page.locator('input[type="password"]')
    await pwFields.nth(0).fill(password)
    const count = await pwFields.count()
    if (count > 1) await pwFields.nth(1).fill(password)
  }

  await registerBtn.click()
  await page.waitForURL('/')
}

test.describe('authentication', () => {
  test('redirects to login when not authenticated', async ({ page }) => {
    await page.goto('/')
    // Login form should be visible
    await expect(page.locator('input[type="email"]')).toBeVisible()
    await expect(page.locator('input[type="password"]').first()).toBeVisible()
  })

  test('can register a new account', async ({ page }) => {
    const email = uniqueEmail('register')
    await page.goto('/')
    await page.waitForSelector('input[type="email"]')

    await page.fill('input[type="email"]', email)

    // Fill password fields — first run shows register form immediately
    const pwFields = page.locator('input[type="password"]')
    await pwFields.nth(0).fill('password123')
    const count = await pwFields.count()
    if (count > 1) await pwFields.nth(1).fill('password123')

    // Submit
    const btn = page.getByRole('button', { name: /create account|sign in/i }).first()
    await btn.click()
    await page.waitForURL('/')

    // Dashboard should show Ding title
    await expect(page.getByRole('heading', { name: /ding/i })).toBeVisible()
  })

  test('shows error on duplicate email', async ({ page }) => {
    const email = uniqueEmail('dup')

    // First registration
    await page.goto('/')
    await page.waitForSelector('input[type="email"]')
    const regTab1 = page.getByRole('button', { name: /^create account$/i })
    if (await regTab1.isVisible({ timeout: 2000 }).catch(() => false)) await regTab1.click()
    await page.fill('input[type="email"]', email)
    const pwFields1 = page.locator('input[type="password"]')
    await pwFields1.nth(0).fill('password123')
    const c1 = await pwFields1.count()
    if (c1 > 1) await pwFields1.nth(1).fill('password123')
    await page.locator('button[type="submit"]').click()
    await page.locator('button[title="Sign out"]').waitFor({ timeout: 10000 })

    // Log out
    await page.getByRole('button', { name: /sign out/i }).click()
    await page.waitForSelector('input[type="email"]')

    // Switch to register tab (users exist now)
    const createTab = page.getByRole('button', { name: /^create account$/i })
    if (await createTab.isVisible({ timeout: 2000 }).catch(() => false)) await createTab.click()

    // Try to register with same email
    await page.fill('input[type="email"]', email)
    const pwFields2 = page.locator('input[type="password"]')
    await pwFields2.nth(0).fill('password123')
    const c2 = await pwFields2.count()
    if (c2 > 1) await pwFields2.nth(1).fill('password123')
    await page.locator('button[type="submit"]').click()

    // Expect an error message
    await expect(page.locator('p.text-red-400, [class*="text-red"]')).toBeVisible({ timeout: 5000 })
  })

  test('can log in after registering', async ({ page }) => {
    const email = uniqueEmail('login-after')

    // Register
    await page.goto('/')
    await page.waitForSelector('input[type="email"]')
    const regTab = page.getByRole('button', { name: /^create account$/i })
    if (await regTab.isVisible({ timeout: 2000 }).catch(() => false)) await regTab.click()
    await page.fill('input[type="email"]', email)
    const pwFields = page.locator('input[type="password"]')
    await pwFields.nth(0).fill('password123')
    const count = await pwFields.count()
    if (count > 1) await pwFields.nth(1).fill('password123')
    await page.locator('button[type="submit"]').click()
    await page.locator('button[title="Sign out"]').waitFor({ timeout: 10000 })

    // Log out
    await page.getByRole('button', { name: /sign out/i }).click()
    await page.waitForSelector('input[type="email"]')

    // Log in again
    await page.fill('input[type="email"]', email)
    await page.fill('input[type="password"]', 'password123')
    await page.locator('button[type="submit"]').click()
    await page.locator('button[title="Sign out"]').waitFor({ timeout: 10000 })

    // Dashboard visible
    await expect(page.locator('button[title="Settings"]')).toBeVisible()
  })

  test('can log out', async ({ page }) => {
    const email = uniqueEmail('logout')

    // Register and land on dashboard
    await page.goto('/')
    await page.waitForSelector('input[type="email"]')
    const regTab = page.getByRole('button', { name: /^create account$/i })
    if (await regTab.isVisible({ timeout: 2000 }).catch(() => false)) await regTab.click()
    await page.fill('input[type="email"]', email)
    const pwFields = page.locator('input[type="password"]')
    await pwFields.nth(0).fill('password123')
    const count = await pwFields.count()
    if (count > 1) await pwFields.nth(1).fill('password123')
    await page.locator('button[type="submit"]').click()
    await page.locator('button[title="Sign out"]').waitFor({ timeout: 10000 })

    // Sign out
    await page.getByRole('button', { name: /sign out/i }).click()

    // Login form should reappear
    await expect(page.locator('input[type="email"]')).toBeVisible()
  })
})
