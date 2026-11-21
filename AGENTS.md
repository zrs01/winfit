# winfit — Agent Guide

Go CLI that sets the position and size of desktop application windows on
Windows 10/11 (x64, ARM64). Target by window title, application name, or
exact handle; with no target flag it moves the CLI's own console window.
Usage examples: [README.md](README.md). This file is the authoritative
record of semantics and decisions.

Non-goals: animations, virtual-desktop management, hotkeys, always-on-top
or any z-order changes, non-Windows platforms.

## Layout & boundaries

- `main.go` / `flags.go` / `run.go` / `get.go` / `pick.go` / `logging.go` —
  CLI assembly (urfave/cli/v3, flags use `Aliases:` for shorts) and
  orchestration: parse → resolve target (incl. the ambiguity prompt) →
  compute rect → apply → corrective re-check. `selectorFlags()` is shared by
  the root move and `get`; `get.go` is the read-only half (no geometry
  flags).
- `internal/place` — the ONLY place placement geometry is encoded (px/%
  parsing, derived sizes, keep-aspect, clamping/containment). Pure numbers;
  never imports syscall.
- `internal/target` — pure selection rules (title/app matching, multi-process
  rooting, ambiguity policy) as pure functions over injected inputs
  (z-ordered window list, `Image`/`Parent`/`ForegroundPid` funcs). No
  syscalls here either.
- `internal/winapi` — all Win32 calls (`winapi_windows.go` via
  golang.org/x/sys/windows LazyProc; `winapi_other.go` stubs return
  "unsupported"). Policy-free: testable logic belongs in place/target.
- `internal/picker` — the bubbletea selection list for ambiguous matches.
  Renders rows handed to it; no Win32, no geometry. Key handling lives in
  `model.Update` so it is testable without a terminal.
- Diagnostics and errors are standard library only: `log/slog` on stderr
  so stdout stays parseable, and `errors`/`fmt.Errorf` with `%w` — `main`
  walks the cause chain at debug level instead of printing stack traces.
  `logging.go` picks the handler: `consoleHandler` for an attached console
  (no timestamps, `level:` tag colored red/yellow/cyan/dim, attributes as
  trailing `key=value`), slog's text format with `time=` when stderr is
  piped or redirected. Colors only after `winapi.EnableVT` turns on
  virtual-terminal processing for the handle (off on pre-Win10 conhost).
  Level comes from `LOG_LEVEL`: `error|warn|info|debug`, `trace` ≡ debug.

Invariant: `go build ./...` and `go test ./...` pass on Linux CI — keep the
non-Windows stubs in sync with the windows implementations.

## Commands

- `make` — build `bin/winfit.exe` (host arch; version from `git describe`).
- `make test` / `make fmt` / `make clean` / `make windows` (cross-build the
  release `windows_amd64` binary into `build/`).
- Releases: push a `v*` tag → `.github/workflows/release.yml` runs GoReleaser
  (`.goreleaser.yaml`) on ubuntu-latest, publishing zipped
  `windows_amd64`/`windows_arm64` binaries + `checksums.txt` + changelog to
  the GitHub Release. `main.version` comes from the tag
  (`-ldflags -X main.version=<tag>`), same stamp as the Makefile — the
  version is derived from the tag, never hand-edited into a source file.
  `CHANGELOG.md` carries the curated, consumer-facing notes (write the entry
  in the same change as the feature; the release body itself is generated
  from the commit list, with `^docs:`/`^test:`/`^chore:` filtered). Check the
  packaging locally with `goreleaser release --snapshot --clean` (output in
  gitignored `dist/`).
- CI: `.github/workflows/ci.yml` — `go build ./...`, `go test ./...`, gofmt
  check on Linux, plus `make windows` and a `GOARCH=arm64` build.
- `modd` (`modd.conf`) re-runs `go test ./internal/...` on Go changes.
- Never introduce sh-only syntax (inline `VAR=x cmd`, `||`, `2>/dev/null`,
  `rm`, grep/awk): the Makefile pins `SHELL=cmd.exe` on Windows because
  Trend Micro blocks Git's `sh.exe` from forking (see global AGENTS.md).

## Value semantics

- All position/size flags accept `<px>` or `<n>%`; `%` suffix required,
  fractions allowed (`33.3%`), negatives allowed on position flags.
  No keyword values on any flag (`100%` is how you fill).
- Positions are absolute: pixels (or % of the reference work-area extent on
  that axis) measured inward from the named edge of the work area (taskbar
  excluded). Flush-to-edge is `0`. There is no relative-nudge syntax
  (`+100`) anywhere.
- One horizontal + one vertical value fully determine a position; an axis
  with no flag keeps that coordinate (one-axis moves work).
- Given both `--left` and `--right` without `--width`, the pair is a
  **derived size**: `width = workarea_width − left − right` (resolved to px
  first, so `--left 100 --right 20%` mixes fine). Same for top+bottom.
  Explicit `--width` wins over a derived width (warning logged).
- Sizes are absolute px or % of the reference work-area dimension; >100% is
  accepted and clamped by containment. `100%` on both axes is a plain
  resize to the work area — never `SW_MAXIMIZE`; styles untouched; fills
  only its own monitor.
- Resize anchor (no position flag): top-left by default; the right/bottom
  edge is held (window grows leftward/downward) when requested via
  `--right`/`--bottom` in the same call or when already touching that
  work-area edge.
- `--keep-aspect` (-k): aspect = current w/h before any change. Both dims
  requested → largest size at that aspect fitting the requested box
  (contain-fit); one dim requested → the other scales by the aspect. A
  derived width counts as the width input. Applied before containment
  clamping, so it can only shrink relative to the request, never push the
  window off-screen.
- No position or size flags at all → help + exit 1 (nothing to do).
- Exit codes: 0 ok, 1 runtime error (target errors, API failure),
  2 usage error (cli/v3 convention).

## Reading (`get`)

- `winfit get` = `selectorFlags()` + `--json`, no geometry flags; it reads
  the window's own monitor (no `--monitor`) and does not refuse a
  cross-session target, since reading moves nothing.
- Reported numbers are the flag contract in reverse: `position` (left/top),
  `size` (width/height) and `from` (right/bottom) are logical px measured
  from the work-area edges — the same values `--left/--top/--right/--bottom/
  --width/--height` take — so `get` output pastes straight into a move.
  `rect` and `workArea` are the raw physical px from `GetWindowRect`;
  `percent` is scale-invariant. The conversions live in
  `place.EdgeOffsetsFrom`, `place.PercentagesFrom` and
  `place.PhysicalToLogical` (inverse of `Spec.ScaleLogical`), never inline
  in `get.go`.
- Monitor attribution (`monitorOf` in `get.go`): exact work-area match
  against `winapi.Monitors()`, then containment; index 0 = unknown.
- State notes come from `winapi.WindowState` (`IsIconic` + `WS_MAXIMIZE`):
  minimized ⇒ the rect is the restored geometry. `WS_MAXIMIZE` alone is not
  taken as evidence of a maximize — `SetWindowPos` can leave the bit set on
  a small window — so only a rect covering ≥ `maximizedFillPct` of the work
  area is called maximized (a true maximize overhangs the work area by the
  invisible resize border).

## Target resolution

Precedence: `--hwnd` → `--title` → `--app` → self.

- Candidate filter (all selectors, via `EnumWindows`): top-level, visible,
  **unowned** (`GetWindow(GW_OWNER)==NULL`), `WS_CAPTION`, non-empty title,
  not `WS_EX_TOOLWINDOW`/`WS_EX_NOACTIVATE`.
- `--title` / `--app` are both case-insensitive **substrings**. For `--app`
  the image basename is compared after trimming a trailing `.exe` from the
  pattern; candidates owned by the current PID are skipped.
- `--hwnd`: hex with or without `0x`, validated by `IsWindow` + the filter.
  Unique per live window but recyclable — stale handles error with a
  `winfit list` hint.
- Ambiguity (N>1 surviving windows): `target.Select` returns
  `*target.AmbiguousError` carrying the survivors in z-order, and
  `pick.go` renders them as a bubbletea list (`picker.Pick`): up/down or
  k/j move, typing filters over title/app/class/hex-HWND, esc clears the
  filter then cancels on the second press, ctrl-c cancels, enter picks.
  Cancel ⇒ error + exit 1 (nothing moved). `--topmost` (= first match in
  EnumWindows z-order, front→back, no extra API call) and a tightened
  pattern skip the prompt. Without a TTY on stdin AND stdout (scripts,
  CI) the prompt is skipped and the error keeps the `winfit list` →
  `--hwnd` / `--topmost` hint. Zero matches: error with selector echo +
  `list` suggestion. Never guess.
- Multi-process apps (`chrome.exe` × N), in order: (1) window-first — only
  PIDs owning filter-passing windows count, so helper/renderer children
  self-eliminate; (2) process-tree rooting via
  `CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS)` — prefer PIDs whose parent
  runs a different image (main processes are spawned by the shell; children
  by the same exe); (3) tie-break: PID owning the foreground window, then
  most candidate windows, then lowest PID; (4) still tied → ambiguity
  policy, never silent pick. Rooting narrows the PROCESS only: when the
  winning PID still owns several filter-passing windows, those windows are
  the ambiguity list (unless `--topmost`).
- Self (no selectors): `GetConsoleWindow()`, else attach the parent's
  console; walk `GetAncestor(GA_ROOTOWNER)` — under Windows Terminal the
  console HWND is a hidden conhost pseudo-console and this lands on the
  visible `WindowsTerminal.exe` window, moving **all tabs** (accepted).
  `list` hides this process's own console.
- Cross-user guard: `ProcessIdToSessionId` target vs own PID; windows in
  another session (hence another user) are refused with a clear error.
  Same-user elevated targets simply fail at `SetWindowPos` with the raw
  API error.
- Reference monitor: the window's own (`MonitorFromWindow`) or `--monitor`
  (1-based index or device name like `\\.\DISPLAY2`, via
  `EnumDisplayMonitors`). All px/% values resolve against the reference
  work area; `--monitor` with no position flags moves the window carrying
  its current edge offsets.
- `winfit list [--title|--app]` prints `HWND PID APP CLASS TITLE` in
  z-order; APP is the exe basename with `.exe` trimmed. It is the discovery
  half of `--hwnd`.

## Safety & boundary rules

- Move-only: clamp so ≥ 64 px of the rect stays inside the work area on
  each axis (negative positions allowed within that floor).
- Any resize ⇒ **full containment overrides every position instruction**:
  size clamped to work area (≥ `SM_CXMIN/CYMIN`), then position nudged so
  no corner escapes — even contradicting a requested `--right/--bottom`.
- Apply is ONE `SetWindowPos` (`SWP_NOZORDER|SWP_NOACTIVATE`; `SWP_NOSIZE`
  dropped when resizing): atomic move+resize, never steals focus,
  preferred over `MoveWindow`/`ShowWindow`. Iconic windows:
  `ShowWindow(SW_RESTORE)` first; maximized must restore first or the move
  is a no-op (logged).
- After the call, re-read `GetWindowRect` (the window may refuse the size
  via `WM_GETMINMAXINFO`) and run a corrective containment pass; then exit
  0 reporting the actual rect.
- Work area, never full monitor bounds (taskbar/app bars respected).
- DPI: `winapi.Init()` sets per-monitor-v2 awareness at startup via
  `SetProcessDpiAwarenessContext(PER_MONITOR_AWARE_V2)` (runtime call, no
  manifest; falls back to `SetProcessDPIAware()` pre-1607). Flag pixels are
  logical, scaled by `GetDpiForWindow(target)/96`; percentages are
  scale-invariant.
- `ImageName` tries `PROCESS_QUERY_INFORMATION` then
  `PROCESS_QUERY_LIMITED_INFORMATION`: UWP-era processes (Notepad on Win10
  19045) refuse the LIMITED mask. `GetConsoleWindow` lives in kernel32, not
  user32. Negative `GWL_*` indices need a runtime `uptr(int)` helper
  (compile-time `uintptr(-20)` overflows).

## Testing

- `internal/place`, `internal/target` and the `internal/picker` key model:
  table-driven unit tests, pure functions, no build tags, run anywhere;
  plus a property-style invariant: any resize request ⇒ output rect
  contained in work area. These helpers also carry the read-back path used
  by `get`: `EdgeOffsetsFrom`, `PercentagesFrom`, `PhysicalToLogical`
  (round-trip pinned at 1.25/1.5/2× scaling).
- Root tests (`pick_test.go`) pin that
  `*target.AmbiguousError` survives `%w` wrapping (else the picker
  never runs) and that a piped run keeps the non-interactive hint.
  `get_test.go` pins the stable `--json` key set, the flag-ready numbers in
  the human report, the three state notes, and `monitorOf`.
- The picker draws ASCII only (`>` cursor, `^ n above`, `v n more`):
  legacy conhost fonts rasterize box-drawing glyphs as `?`/`u`.
- Anything needing real HWNDs stays manual/live (`winfit list`, moving
  Notepad); no Windows-gated test binaries currently.
- Manual matrix when relevant: Win10/11, single/dual monitor, 100/150/200%
  scaling, conhost vs Windows Terminal.

Updated: 2026-11-21 — added `winfit get` (read position/size, `--json`),
`place` read-back helpers, `winapi.WindowState`; earlier same pass added the
GoReleaser release pipeline. Sole semantics
source; ambiguous matches resolve
through the `internal/picker` selection list, `target.Select` reports them
as `*target.AmbiguousError`; logging/errors are stdlib `log/slog` +
`errors` with a human `consoleHandler`, `internal/logging` is gone).
