import { type Page } from '@playwright/test'

export async function register(page: Page, email = 'test@example.com', password = 'password123') {
  await page.goto('/')
  await page.fill('[placeholder*="email" i], input[type="email"]', email)
  await page.fill('[placeholder*="password" i], input[type="password"]', password)
  // Look for register/create account button
  const registerBtn = page.getByRole('button', { name: /create|register|sign up/i })
  if (await registerBtn.isVisible()) {
    await registerBtn.click()
  } else {
    await page.getByRole('button', { name: /submit|log in|sign in/i }).click()
  }
  await page.waitForURL('/')
}

export async function login(page: Page, email = 'test@example.com', password = 'password123') {
  await page.goto('/')
  await page.fill('[placeholder*="email" i], input[type="email"]', email)
  await page.fill('[placeholder*="password" i], input[type="password"]', password)
  await page.getByRole('button', { name: /log in|sign in/i }).click()
  await page.waitForURL('/')
}
