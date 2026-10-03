# WhatsApp Business pairing

## Request ledger

- [x] Inspect current support and investigate the reported October 2 failure.
- [x] Update the WhatsApp protocol library.
- [x] Preserve pairing completion and specific connection errors in the QR status.
- [x] Report passkey requests clearly without logging credentials or implementing authentication.
- [x] Verify pairing status, error delivery, cancellation and expiry; complete code review.
- [x] Commit the verified change.
- [ ] Retry the Business account with phone approval (requires the account owner).

The user explicitly deferred passkey authentication on October 3. The existing
QR/phone approval flow remains the connection method; no browser handoff or
passkey response endpoint is included.

## Evidence

Jaz used whatsmeow's June 22, 2026 revision. Upstream added passkey pairing
on July 1 in https://github.com/tulir/whatsmeow/pull/1186. Affected Business
accounts are described in https://github.com/tulir/whatsmeow/discussions/1187.
The failed October 2 attempt was not captured in durable logs, so the missing
protocol support is a confirmed gap and an unconfirmed cause of that attempt.

## Implementation and verification

The pinned whatsmeow revision is now `v0.0.0-20260929112325-8b41cfe6d9c4`.
Its required updates replace existing dependencies; no new dependency is added.
Pairing completion and connection errors are owned by the client event handler.
The QR-channel watcher no longer overwrites those outcomes with delayed generic
events. Passkey requests and manual confirmation requests terminate with an
explicit unsupported-step error through the existing connection screen.

Verified on October 3:

- Full backend suite, `go vet ./...`, backend build, and WhatsApp race tests pass.
- All 300 frontend tests, typecheck and lint pass.
- Restoring the old watcher makes five regression cases fail as expected.
- An isolated live probe receives a QR from WhatsApp and cancellation disconnects
  its client and removes its session. No phone account was linked by this probe.
- Strict maintainability review: the change stays in the provider's existing QR
  lifecycle; no new endpoint, UI state, browser integration or authenticator.

The account owner must retry Business app → Linked devices → Link a device after
starting the updated backend. This is the remaining account-compatibility check.
