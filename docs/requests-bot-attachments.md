# Bot Chat Attachments

- [x] Verify short text paste, long text attachments and image attachments through the shared bot composer.
- [x] Preserve attachments in sent bot messages, including attachment-only messages and reloads.
- [x] Reuse the existing attachment renderer without adding composer controls.
- [x] Run regression checks, full verification and strict code review.
- [x] Commit the verified fix.
- [x] Merge main into this branch, preserving main's chat bubbles and attachment rendering; verify and commit the conflict resolution.

Scope: one-to-one bot chats. Their shared composer already handles clipboard text above 2,000 characters as a UTF-8 file and clipboard images as attachments. The bot transcript projection discarded the attachment blocks, leaving attachment-only messages empty. It now retains their metadata and owning session so the normal attachment renderer can display them, including images served by the backend attachment URL.

Verification: 304 frontend tests, typecheck, lint, web build and full Go tests pass. The added render regression exercises the real bot projection and attachment renderer for attachment-only and text-plus-attachment messages. Removing the attachment wiring makes it fail on the missing text file. Strict review found no duplicate paste/upload logic or provider-boundary changes.

An isolated side-browser fixture exercised the production composer, projection and chat renderer, with upload/send boundaries simulated: long Unicode text retained exact bytes, pasted PNG displayed, both sent without accompanying text, the draft cleared, and both remained visible after reload. A short paste remained unhandled for the browser's native insertion. This verifies renderer behavior; it does not claim a live model turn or OS clipboard check.

Group chats still use a text-only composer and API. Asked whether the report concerns groups; no answer yet. No group behavior or app release was changed.

Merged main at `0099dcd6`, keeping its rounded chat bubbles and the shared attachment renderer. Attachment-only messages avoid an empty text bubble. On the combined source, all 306 frontend tests, typecheck, lint, web build and full Go tests pass. Strict review found no structural issues. The isolated browser fixture confirms attachment-only, mixed and plain messages through the rendered accessibility tree; screenshot capture timed out.
