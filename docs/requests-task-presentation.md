# Task presentation and duplicate bot replies

- [x] Preserve data-only task lookups and make rendered task results explicit to agents.
- [x] Present selected tasks once, in order, with optional reasons and links that open Tasks.
- [x] Show explicit app results in normal chats and user-facing bot/routine turns without a duplicate fallback reply.
- [x] Clarify the asynchronous routine handoff that caused the reported duplicate.
- [x] Review and run full checks in both repositories.
- [ ] Verify the rendered integration in the Jaz side browser: blocked because this conversation's browser is disconnected.
- [x] Commit both repositories and push Jaz Tasks (Tasks c96c45e on main; Jaz changes are committed on the task branch).

Keep lookup calls quiet. Use MCP resource links for explicit presentation and preserve remote tool metadata. Presentation feedback must follow successful event persistence. No task-specific logic belongs in Jaz's host.

Verification on 2026-10-03: both complete Go suites, Tasks auth tests, Tasks frontend check/build, and Jaz's 299 frontend tests/typecheck/lint pass. A real native Codex routine turn used the actual Tasks server through Jaz's proxy: it read tasks, called show_issues once and completed with no assistant text. Disabling resource presentation made the proxy regression fail. Browser inspection and click-through remain unverified; no Jaz release is part of this change.

A native handoff probe called the production loop_run handler against a synthetic service once, made no task calls and ended with no assistant text. The simulated run was labeled read-only because it schedules no work; the fixture’s send_message was rejected by native approval review, so this verifies the handoff rather than acknowledgement delivery.
