# Periodic desktop updates

- [x] Check for new updates while Jaz stays open, every five minutes as requested.
- [x] Check again when the computer wakes from sleep.
- [x] Verify periodic discovery, retry after a failed check and preservation of downloaded updates.
- [x] Run full checks and the strict code-quality review, then commit.

The existing desktop updater checked ten seconds after launch and every six hours thereafter. Keep the existing download and user-confirmed installation flow.

Verification: the isolated Electron regression runs through the production controller with accelerated timers. Restoring the six-hour interval or removing the wake listener each fails its specific assertion. All 301 frontend tests, typecheck, lint, desktop bundle build and the full Go 1.26 test suite and vet pass. Strict review found no additional changes needed: the existing updater owns both scheduling and download state, and its library already coalesces in-flight checks.

Status: committed on the task branch; not merged or released.
