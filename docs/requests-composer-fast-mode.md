# Composer Fast Mode correction

- [x] Restore the main composer's Fast Mode switch and picker toggle, saved per agent/provider and applied before the first prompt.
- [x] Remove the model picker from existing chat composers; retain their Fast Mode menu switch.
- [x] Restore the previously requested minimal Bot composer from the unmerged correction.
- [x] Verify main, existing-chat and Bot composers; run full checks and strict review.
- [x] Follow-up thermo-nuclear review: replace agent-wide Fast Mode inference with native model evidence and keep pending changes alive across menu closure.

The earlier merge included `31ba7f82` and `af35280b` but omitted `6756bd69` and `46492b7d`. This correction carries over the omitted work and removes the existing-chat picker and its redundant control hook.

Verified: 305 frontend tests, typecheck, lint, web/desktop builds, full Go suite and the desktop model-picker smoke test. The smoke test checks Fast Mode persistence across model, agent, provider and mount changes. Side-browser checks exercised the actual New route and ThreadView, including first-prompt request ordering and the existing chat's active-turn switch. Native Codex ACP 2.1.1-jaz.1 advertised and accepted Fast Mode before its first prompt.

Follow-up review corrected two issues:

- New-chat Fast Mode now requires the selected model's native `additional_speed_tiers` metadata, read from Codex's cache in the saved authentication profile. Missing metadata remains unknown. HTTP coverage verifies profile changes, unsupported models and other providers; the picker smoke verifies unsupported models receive neither the toggle nor a startup setting. The real native cache matches the live adapter's advertisement.
- The thread owns setting mutations, so closing the menu cannot discard pending state. Pending lasts through refreshed native state. The desktop smoke exercises the actual ThreadView, closes and reopens the menu during a delayed save, delays the refresh separately and checks the final enabled switch. The existing-chat model picker stays absent.

No remaining code-quality blockers. Changes are committed on the task branch; merge and release remain separate.
