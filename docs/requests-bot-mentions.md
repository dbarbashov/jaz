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
