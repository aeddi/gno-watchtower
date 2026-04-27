import { test, expect } from '@playwright/test'

test('app loads with TopBar and three content rows', async ({ page }) => {
    // Capture console errors so test failure has actionable detail.
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))

    await page.goto('/')

    // TopBar shows the brand label.
    await expect(page.getByText('scribe').first()).toBeVisible({
        timeout: 5000,
    })

    // Three structural section regions exist.
    await expect(page.locator('section.top')).toBeVisible()
    await expect(page.locator('section.middle')).toBeVisible()
    await expect(page.locator('section.bottom')).toBeVisible()

    // No JS page errors during initial load.
    expect(errors).toEqual([])
})

test('URL stays clean in live mode (no at param)', async ({ page }) => {
    await page.goto('/')
    // Allow any settling watchers to fire.
    await page.waitForLoadState('networkidle').catch(() => {})
    const url = new URL(page.url())
    expect(url.searchParams.get('at')).toBeNull()
})
