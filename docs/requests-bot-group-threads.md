# Bot Group Threads

- [x] Hand a group's new posts to a member while it works on its turn, instead of dropping the ones that arrive mid-turn.
- [x] Leave a member's pending question to the user when group posts arrive.
- [x] Merge the durable bot delivery and restart recovery work (0097c081).
- [x] Run each bot's group turns in a thread of its own per group, so group work neither blocks nor reroutes the bot's own chat and nothing lives only in memory.
- [x] Pick up after a restart: resume interrupted bot, worker and group threads, and keep queued group turns.

Measured on the installed Claude adapter: Jaz's prompt queueing already delivers a mid-turn message at the next tool boundary, while `_session/steering` interrupted the running command and the model dropped the message, so Claude stays on prompt queueing.

A bot's group thread is a hidden `bot_member` thread on the bot's agent, model and home, created before its first turn in the group and recorded in `bot_memberships` with the seq of the last group message shown to it. Its identity prompt says it speaks to the group; `send_message` there posts to the group as the bot, and a reply turn still answers the bot that asked. Model and agent changes reach a bot's group threads; deleting a bot or group archives them. Routines created from a group thread belong to the bot, and its usage counts as the bot's.

Restart: startup recovery resumed only the first of a worker and its parent, because a resumed worker draws its parent's attention and the recovery loop skipped sessions whose update time had changed. The status check already rejects sessions that moved on, so that guard is gone.

Verification: full Go tests, bot tests under race, 306 frontend tests, typecheck and lint pass. Negative controls for each behaviour (turns in the main chat, unsaved read position, waking on unaddressed posts, unsynced models, unmapped routine owner, apps shown from a group thread, missing group identity, missing bot tools, the restart guard) fail on the expected test.

## Requested Strict Review

- [x] Audit the group-thread change for structure, durability and duplicated mechanisms.
- [x] Fix confirmed findings, verify and commit.

The review replaced the in-memory turn scheduler (taking/owed flags, a wake loop and a startup pass that rebuilt lost wakes) with the server's durable per-thread queue, which already runs queued turns when a thread is free and survives restarts. A post now goes to every member it addresses and every member whose group thread is taking a turn as it is posted: delivery steers it into that turn, or queues a turn on the thread when it takes none or cannot take the messages, and only then marks them seen. Deciding at post time closed a gap where a message posted in a turn's last moments was judged after the turn ended and never delivered. The follow-up cap is spent only when something is delivered. Net change: 367 lines removed, 187 added.

Verification: full Go tests, bot tests 50 times plain and 10 times under race, and controls for turns in the main chat, mid-turn steering, the post-time turn check, a refused queue, the follow-up cap, unsynced models, apps shown from a group thread and missing group identity all fail on the expected test. The first-turn fallback to a bot's own last post is no longer reachable by any test, because deliveries now keep the read position current; it remains for groups that predate group threads.
