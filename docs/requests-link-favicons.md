# Website icons in chat links

- [x] Diagnose the globe on CRM links: Google favicon discovery returns 404; CRM's SVG works.
- [x] Try the site's favicon directly, preserving browser-supplied icons and Google/globe fallbacks.
- [x] Keep the source's origin and port; update the real Markdown rendering regression.
- [x] Verify rendered source selection; run full frontend/backend checks and strict review.
- [x] Prepare the verified fix for commit; retain the existing app release/version.

CRM's companion change adds a public conventional ICO endpoint from its existing glyph and corrects record-icon baseline alignment. No new dependency or release/version change.

301 frontend tests, typecheck, lint, web build and full Go build/vet/test pass. Browser-supplied URLs keep priority; failed sources advance through the site icon, Google and globe without retrying the failed URL. The side browser connection became unavailable before a live loading/fallback check, which remains a verification limitation. This frontend change takes effect in the next Jaz app build.
