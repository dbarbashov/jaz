# Native computer use

Computer Use adds two Jaztools, disabled by default:

- `computer_status` reports the desktop connection, platform, pinned driver version, OS permissions and the conversation holding the machine lease.
- `computer_js` runs persistent JavaScript with native app discovery, window observations, accessibility, screenshots and input.

Enable it in **Settings → Computer Use** on the desktop. Accessibility and Screen Recording grants are requested only by the permission button. Status queries and scripts never request them. macOS may require restarting the application after a grant. Web clients show that the desktop app is required.

## Agent API

An empty `computer_js` call returns documentation, including when permissions are missing.

```js
await computer.tools()
await computer.tools('list_apps')
const apps = await computer.call('list_apps', {})
nodeRepl.write(apps)
```

`computer.tools(name)` returns the driver's description and JSON input schema. `computer.call(name, args)` returns its structured data, emits its text, and forwards the last screenshot as MCP image content. Supported tools are selected from the driver's capability metadata; Jaz has no duplicate tool-schema catalog.

The surface includes native app/window, accessibility, input, screen, clipboard, menu, state-verification and cursor capabilities. Driver configuration, session administration, recording/installers and browser automation stay outside this API. Jaz's side browser retains its own controls and settings.

Observe at the start of each script and after actions. Use exact window IDs and fresh `snapshot_id`, `element_token` or `capture_id` values. Background delivery is the default; a foreground retry is an explicit agent decision. Unsupported native routes and stale identifiers remain driver errors. Numeric values outside JavaScript's safe range are refused instead of rounding native identities.

JavaScript declarations survive between calls. Each script owns a fresh native driver session, so native snapshot tokens expire after that script. Cancellation, disconnecting or leaving the conversation resets the interpreter. Await each action; parallel native actions inside a script are rejected.

## Ownership and transport

The authenticated session WebSocket runs independently of side-browser visibility:

```text
MCP → Go computercontrol → session WebSocket → renderer QuickJS → restricted IPC → Electron Cua runtime
```

Electron main owns the machine lease, including while the driver starts or shuts down. Other sessions receive a busy error; status remains available. Script IDs are unique and checked with the owning renderer. Cancellation travels over the existing socket as a separate message, aborts the native call and keeps the lease until native cleanup completes. Renderer destruction, navigation, socket closure, timeout and app shutdown cancel active work.

QuickJS exposes only the computer API and text output. It has no Node, Electron, host filesystem or shell bindings. Scripts have a 60-second budget, 64 MiB guest memory and a short CPU-interruption budget. Output is bounded to 12,000 UTF-8 bytes plus one image; native structured/text responses are bounded to 4 MiB and image base64 to 32 MiB.

Jaz pins `@trycua/cua-driver` to `0.30.1`, uses its same-process `CuaDriver.create(undefined)` standard permission mode, and awaits `shutdown()` before destroying the native handle. The optional platform package and Electron-compatible N-API runtime are unpacked from ASAR. No standalone daemon installation is needed. See [Cua SDK documentation](https://cua.ai/docs/reference/cua-driver/sdk-reference) and [upstream source](https://github.com/trycua/cua/tree/main/libs/cua-driver).

## Verification

Normal checks, from their respective backend/frontend directories:

```sh
go test ./...
go test -race ./internal/computercontrol ./internal/app ./internal/jaztools
bun test
bun run typecheck
bun run build:bundle
```

The opt-in live probe runs from `frontend`:

```sh
bun run test:computer:native
```

The probe opens Calculator and a drag fixture and sends native keyboard and
pointer input. Run from the regular desktop with existing Accessibility and
Screen Recording grants, leaving the mouse and keyboard untouched. It closes
its own fixture processes.

`JAZ_COMPUTER_DRIVER_MODULE` may point to the packaged SDK's `dist/index.js` file URL to check the shipped native library. This does not change the test host's macOS permission identity. Development Electron and installed Jaz can have different grants.
