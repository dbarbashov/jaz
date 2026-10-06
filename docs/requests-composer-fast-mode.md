# Composer Fast Mode correction

- [x] Restore the main composer's Fast Mode switch and picker toggle, saved per agent/provider and applied before the first prompt.
- [x] Remove the model picker from existing chat composers; retain their Fast Mode menu switch.
- [x] Restore the previously requested minimal Bot composer from the unmerged correction.
- [x] Verify main, existing-chat and Bot composers; run full checks and strict review.

The earlier merge included `31ba7f82` and `af35280b` but omitted `6756bd69` and `46492b7d`. This correction carries over the omitted work and removes the existing-chat picker and its redundant control hook.

Verified: 305 frontend tests, typecheck, lint, web/desktop builds, full Go suite and the desktop model-picker smoke test. The smoke test checks Fast Mode persistence across model, agent, provider and mount changes. Side-browser checks exercised the actual New route and ThreadView, including first-prompt request ordering and the existing chat's active-turn switch. Native Codex ACP 2.1.1-jaz.1 advertised and accepted Fast Mode before its first prompt. Strict review found no remaining blockers; existing-chat controls now use a single menu component and no picker-specific hook or disabled state.
