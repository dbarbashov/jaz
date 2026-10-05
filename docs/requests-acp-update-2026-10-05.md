# Claude and Codex ACP Update

Request: update Jaz's Claude and Codex ACP adapters when newer versions exist.

- [x] Check published adapter and native runtime versions.
- [x] Merge released upstream changes into the existing Jaz adapter forks.
- [x] Pin and verify the updated native runtimes through both native and ACP paths.
- [x] Publish verified bundles and update Jaz's asset specification and manifest.
- [x] Run the full backend suite, review the final changes and commit.

Baseline: Claude ACP 0.84.0-jaz.1 with SDK 0.3.284; Codex ACP 2.0.0-jaz.2 with Codex 0.159.0.
Latest checked: Claude ACP 0.85.1, SDK 0.3.289 / Claude Code 2.1.289; Codex ACP 2.1.1, Codex 0.160.0.

Sources: [Claude ACP](https://github.com/agentclientprotocol/claude-agent-acp/releases/tag/v0.85.1), [Codex ACP](https://github.com/agentclientprotocol/codex-acp/releases/tag/v2.1.1), npm registry metadata checked on 2026-10-05.

## Native Comparison

Measured on macOS arm64 with subscription OAuth; all four paths send one unchanged 93-byte initial user prompt.

| Check | Claude Native | Claude ACP | Codex Native | Codex ACP |
| --- | --- | --- | --- | --- |
| Resolved model | claude-sonnet-5-5 | claude-sonnet-5-5 | gpt-6.1-sol | gpt-6.1-sol |
| Provider-reported context | 1,000,000 | 1,000,000 | 258,400 | 258,400 |
| First request input tokens, including cache | 22,306 | 20,733 | 15,834 | 16,135 |
| File tool reads fixture | Pass | Pass | Pass | Pass |
| Fresh-process reload recalls fixture | Pass | Pass | Pass | Pass |
| Compaction and subsequent recall | Pass | Pass | Pass | Pass |
| Subscription authentication | Pass | Pass | Pass | Pass |

Full native CLI and SDK requests differ with their tool surfaces; the totals above are measurements, not a claim that the full requests are identical. Claude retains the native tool preset, Default effort and unknown context until provider reporting. The upstream effort update also preserves a settings-requested Ultracode mode when clearing an unsupported effort override.

Validation: Claude build/lint/format and 2,090 tests; Codex typecheck/build and 1,075 tests; Codex 0.160.0 schema generation produces no drift. Both compiled macOS arm64 bundles complete a real prompt through Jaz's Go ACP client. Codex's release workflow verifies and builds all six platform bundles; Claude builds all five existing platform bundles. Other platform binaries were not executed locally.

Release source commits: Claude `2b2697d0b5d54a8ad869d7163a81e9a78952b976`; Codex `d9021c03668f576d468dba752bc94b8f654bd202`. Codex release workflow run: [37276993074](https://github.com/gluonfield/codex-acp-app-server/actions/runs/37276993074), successful and matching the release tag commit.

Local probe evidence: `/private/tmp/jaz-acp-parity-20261005/`; baseline and final backend-suite logs: `/private/tmp/jaz-acp-update-20261005-backend-tests.log`.

Published bundles: [Claude ACP 0.85.1-jaz.1](https://github.com/gluonfield/claude-agent-acp/releases/tag/v0.85.1-jaz.1), [Codex ACP 2.1.1-jaz.1](https://github.com/gluonfield/codex-acp-app-server/releases/tag/v2.1.1-jaz.1). All eleven updated archives have matching published SHA-256 digests and contain the adapter and required native executables. The unchanged adapters are preserved exactly.

The existing manifest generator stalled on its first streamed archive under local Node 26. Direct archive hashing and inspection generated the same manifest structure from the pinned specification and GitHub release metadata. No validator code was changed.

Strict review: no Jaz runtime logic, defaults, authentication or protocol contracts change; the diff updates two existing adapter pins, their eleven asset URLs/checksums and this request ledger. Existing unfinished adapter checkouts are preserved. The installed Jaz release remains pinned to its own manifest; these updates take effect in the next Jaz build.

Final Jaz `go test ./...` passes on Go 1.26.0 after the manifest update. All request items are complete; no Jaz application release was created.
