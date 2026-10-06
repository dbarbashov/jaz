# Bot Group Threads

- [x] Hand a group's new posts to a member while it works on its turn, instead of dropping the ones that arrive mid-turn.
- [x] Leave a member's pending question to the user when group posts arrive.
- [x] Merge the durable bot delivery and restart recovery work (0097c081).
- [x] Run each bot's group turns in a thread of its own per group, so group work neither blocks nor reroutes the bot's own chat and nothing lives only in memory.
- [x] Pick up after a restart: resume interrupted bot, worker and group threads, and wake members with unanswered messages addressed to them.

Measured on the installed Claude adapter: Jaz's prompt queueing already delivers a mid-turn message at the next tool boundary, while `_session/steering` interrupted the running command and the model dropped the message, so Claude stays on prompt queueing.

A bot's group thread is a hidden `bot_member` thread on the bot's agent, model and home, created before its first turn in the group and recorded in `bot_memberships` with the seq of the last group message shown to it. Its identity prompt says it speaks to the group; `send_message` there posts to the group as the bot, and a reply turn still answers the bot that asked. Model and agent changes reach a bot's group threads; deleting a bot or group archives them. Routines created from a group thread belong to the bot, and its usage counts as the bot's.

Restart: startup recovery resumed only the first of a worker and its parent, because a resumed worker draws its parent's attention and the recovery loop skipped sessions whose update time had changed. The status check already rejects sessions that moved on, so that guard is gone.

Verification: full Go tests, bot tests under race, 306 frontend tests, typecheck and lint pass. Negative controls for each behaviour (turns in the main chat, unsaved read position, waking on unaddressed posts, unsynced models, unmapped routine owner, apps shown from a group thread, missing group identity, missing bot tools, the restart guard) fail on the expected test.
