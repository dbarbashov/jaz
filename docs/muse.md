# Muse Code

Jaz's `muse` agent runs Meta's native Muse Code through the
[BrokkAI Muse ACP adapter](https://github.com/BrokkAi/muse-acp).
The verified pair is adapter **0.9.0** and native Muse
**1.4.2-R4684.1**, on macOS ARM64.

Install the native CLI using [Meta's instructions](https://dev.meta.ai/docs/muse-code),
and install `muse-acp` on the backend host. Select Muse in Jaz's agent settings.
The adapter's command can be overridden using the existing ACP configuration.
`MUSE_CLI` selects a particular native executable; otherwise Jaz checks PATH and
Muse's installer directory. These executables run on the server.

Sign in through Jaz or run `muse login` on the backend host. Muse owns OAuth,
Keychain credentials, refresh, and its saved API keys. `META_API_KEY` retains its
native precedence; Jaz's optional `JAZ_ACP_MUSE_API_KEY` binds to it explicitly.
Disconnect invokes native `muse logout`; a host environment key must be removed
by the host operator. Login, readiness, and execution use the same native
executable, configured PATH, proxy, locale and XDG configuration directories.

Muse supplies model choices, permission modes, and session controls. Blank model
and effort settings retain its defaults. `none` is an explicit Muse reasoning
level. A context window remains unknown until the native host reports it.
Jaz supplies its rules and memory by appending to Muse's native user `AGENTS.md`
in a private configuration view for each process. Other configuration entries
are linked to the original files, including absent auth, trust and settings files
and their locks, so native first saves reach the original profile. The original
rules remain untouched. Native session data stays in the original data directory.
The view remains available until the process exits. Jaz waits for process exit
before completing Close, so an immediate reload can acquire the native session
lease. Creating this view requires filesystem symlink support.

The adapter bridges native MCP, streaming, tool calls, approvals, session
reload, and advertised slash commands. ACP v1 queues follow-ups; native
in-turn steering requires a future ACP v2 integration. Native `:auto-review`
requires the interactive runtime: Jaz rejects it instead of allowing the
adapter's automatic substitution of a different permission profile.
Compaction goes through the advertised `/compact` command; its availability
and failures remain owned by Muse.

## Verification

The standard test suite covers native account-state responses, login identity,
logout (including keyless endpoints), model and `none` effort selection, native
configuration writes, session reload, prompt/transcript separation, and graceful
shutdown with configuration-view cleanup. Removing the production instruction
delivery path makes the lifecycle test fail. An isolated official Linux native
CLI additionally verified first credential writes through the configuration view.

A real native binary and the real adapter were additionally exercised through
Jaz's manager against an isolated loopback model endpoint. First-turn and
resumed requests retained the native system instructions and original user
rules and contained Jaz context once. No Meta account login or paid model call
was performed. Live account authorization and hosted-model execution remain
separate acceptance checks.
