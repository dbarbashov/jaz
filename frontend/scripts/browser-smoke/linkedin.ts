import type { SideBrowser } from '@/lib/sideBrowser'

export async function exerciseLinkedInNavigation(browser: SideBrowser, evaluate: (expression: string) => Promise<unknown>): Promise<void> {
  const origin = await fetch('/password-origin').then(response => response.text())
  const port = new URL(origin).port
  for (const hostname of ['linkedin.com', 'www.linkedin.com']) {
    const home = `https://${hostname}:${port}/`
    await browser.call({ method: 'Jaz.open', params: { url: home } })
    if (await evaluate('location.pathname === "/login" && document.body.innerText.includes("LinkedIn sign in")') !== true) {
      throw new Error('A blocked LinkedIn homepage did not reach the sign-in page')
    }
  }
  const home = `https://www.linkedin.com:${port}/`
  for (const [url, text] of [[home + '?allowed', 'LinkedIn feed'], [home + 'restricted', 'Blocked'], [home + 'login?blocked', 'Blocked'], [origin + '/?blocked', 'Blocked']]) {
    await browser.call({ method: 'Jaz.open', params: { url } })
    if (await evaluate(`location.href === ${JSON.stringify(url)} && document.body.innerText.includes(${JSON.stringify(text)})`) !== true) {
      throw new Error('LinkedIn recovery replaced a successful page, a blocked sign-in/content page or another site')
    }
  }
  await browser.call({ method: 'Jaz.open', params: { url: home + 'restricted' } })
  if (await evaluate('fetch("/").then(response => response.status)') !== 403) {
    throw new Error('LinkedIn recovery redirected a background request')
  }
}
