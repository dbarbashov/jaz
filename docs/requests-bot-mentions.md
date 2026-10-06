# Bot Mentions

- [x] Render the reported `[@Business Opportunist]` shorthand as a clickable bot mention, including saved messages.
- [x] Wake the addressed group member for unambiguous named mentions; preserve explicit bot IDs and the existing follow-up limit.
- [x] Verify real rendering/navigation, routing, ambiguous names and code examples.
- [x] Run full checks and strict code review; prepare the verified fix for commit.

Evidence: the saved Researcher posts in “Digging deep” contain bracketed names without `(bot:<id>)`. Both the renderer and group router currently require that target. Business Opportunist's preceding post uses the complete linked form successfully.

The existing Markdown pipelines resolve exact, unique names within the group. Explicit linked IDs take precedence; repeated mentions wake a member once. Unknown or ambiguous tags stay plain text, and code, images and web-link labels do not ping bots. Historical text remains unchanged. The routing parser reuses the already-pinned Goldmark dependency.

Verification: all 306 frontend tests, typecheck, lint, web build, full Go tests and bot race tests pass. Routing regressions exercise actual group turns; the renderer regression uses the real bot projection, Markdown, pills and router. Disabling either shorthand resolution path fails its regression on the expected missing link/turn.

The isolated side-browser check used three saved messages from the reported conversation, production ChatLog and styles. Tags displayed correctly, survived a page reload and navigated to Business Opportunist's exact bot ID. The destination was a fixture route; no live bot messages were sent or old pings replayed.

Strict review preserved the complete avatar catalogue separately from the group mention scope, reused the existing pill and text-node traversal, and found no provider/runtime-boundary changes. No application release is part of this change.

## Requested Strict Review

- [x] Audit identity stability, routing/rendering agreement and feature ownership.
- [x] Fix confirmed findings and simplify the implementation.
- [x] Verify the final revision and review the diff; prepare the review fixes for commit.

The requested review found and fixed three issues:

- The shared Markdown renderer read a bot catalogue through global context and rebuilt its name index per message. ChatLog now builds the name-to-target map once and passes it explicitly; unrelated Markdown uses its original pipeline. Group member filtering is memoized.
- New name-only messages did not preserve the recipient chosen at send time. Group posting now adds the canonical bot target before persistence and publication, using that same resolution for waking members. A rename/reused-name regression failed before this fix. Existing historical messages remain unchanged and retain the name-based rendering fallback.
- Markdown entities and escaped punctuation rendered as valid mentions but failed group routing. Resolution now uses the existing Markdown writer's text decoding, including escaped entities and numeric references. The entity/escaped-name routing regressions failed before the fix. Goldmark's existing pinned version is marked as a direct dependency; no version was added or upgraded.

Final verification: 306 frontend tests, typecheck, lint, web build, full Go tests and bot race tests pass. Additional coverage protects unresolved user mentions from broadcasting while preserving ordinary user broadcasts. The browser's rendered controls and click navigation still resolve the saved messages to the correct bot ID. Screenshot capture timed out during this review; the earlier visual inspection remains the styling evidence, and this review changed no styles.

## Mention Appearance

- [x] Align the bot icon with the mention label.
- [x] Keep mentions free of link underlines, including hover and keyboard focus.
- [x] Verify the rendered result, run checks and review the scoped fix.

The reported alignment and underline share one cause: generic Markdown anchor styles override the pill's centered alignment and add hover/focus underlines. Ordinary Markdown links now opt into their own class, leaving mention styling owned by the existing pill.

Verified in the production ChatLog fixture with saved messages and a fresh dark-mode screenshot: icon/label centers differ by less than 0.01 CSS pixels; mention hover and focus-visible have no underline, while an ordinary link still underlines on hover. All 306 frontend tests, full Go tests, typecheck, lint and web build pass. Strict review confirms the four-line source change removes the broad selector without adding mention-specific overrides or changing routing.
