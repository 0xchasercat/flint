import fs from "node:fs"
import { expect, test } from "@playwright/test"

test("authenticated web interface loads without browser errors", async ({ page }) => {
  const passphrasePath = process.env.FLINT_E2E_PASSPHRASE_FILE
  if (!passphrasePath) {
    throw new Error("FLINT_E2E_PASSPHRASE_FILE is required")
  }
  const passphrase = fs.readFileSync(passphrasePath, "utf8").trimEnd()
  const browserErrors: string[] = []
  const authFailures: string[] = []
  const httpFailures: string[] = []

  page.on("console", (message) => {
    if (message.type() === "error") browserErrors.push(message.text())
  })
  page.on("pageerror", (error) => browserErrors.push(error.message))
  page.on("response", (response) => {
    if (response.status() >= 400) {
      httpFailures.push(`${response.request().method()} ${response.status()} ${response.url()}`)
    }
    if ([401, 403].includes(response.status())) {
      authFailures.push(`${response.status()} ${response.url()}`)
    }
  })

  await page.goto("/")
  await expect(page.locator('input[name="passphrase"]')).toBeVisible()
  await page.locator('input[name="passphrase"]').fill(passphrase)
  await Promise.all([
    page.waitForURL((url) => url.pathname === "/"),
    page.getByRole("button", { name: "Login" }).click(),
  ])
  await expect(page.getByRole("link", { name: "Flint", exact: true })).toBeVisible()
  await expect(page.getByRole("heading", { name: "Dashboard", exact: true })).toBeVisible()

  // Read-only API calls must not fail because of an over-strict CSRF check.
  // Browsers still prevent another origin from reading these responses because
  // Flint does not emit permissive CORS headers.
  const crossOriginRead = await page.request.get("/api/host/status", {
    headers: { Origin: "https://untrusted.example" },
  })
  expect(crossOriginRead.ok()).toBeTruthy()

  const crossOriginWrite = await page.request.post("/api/not-a-route", {
    headers: { Origin: "https://untrusted.example" },
    data: {},
  })
  expect(crossOriginWrite.status()).toBe(403)

  const manifestResponse = await page.request.get("/site.webmanifest")
  expect(manifestResponse.ok()).toBeTruthy()
  expect(manifestResponse.headers()["content-type"]).toContain("application/manifest+json")
  expect(await manifestResponse.json()).toMatchObject({ short_name: "Flint" })

  const routes = ["/", "/vms", "/storage", "/networking", "/images", "/firewall", "/analytics", "/settings"]
  for (const route of routes) {
    await test.step(`load ${route}`, async () => {
      await page.goto(route)
      await expect(page.getByRole("link", { name: "Flint", exact: true })).toBeVisible()
      await page.waitForTimeout(500)
    })
  }

  await page.goto("/images#repository")
  const ubuntuLogo = page.locator('img[src="/ubuntu.svg"]').first()
  await expect(ubuntuLogo).toBeVisible()
  await expect(ubuntuLogo).toHaveJSProperty("complete", true)
  expect(await ubuntuLogo.evaluate((image: HTMLImageElement) => image.naturalWidth)).toBeGreaterThan(0)

  expect(authFailures, `authorization failures:\n${authFailures.join("\n")}`).toEqual([])
  expect(httpFailures, `HTTP failures:\n${httpFailures.join("\n")}`).toEqual([])
  expect(browserErrors, `browser errors:\n${browserErrors.join("\n")}`).toEqual([])
})
