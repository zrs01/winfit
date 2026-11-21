# winfit

[![Release](https://img.shields.io/github/v/release/zrs01/winfit?label=release)](https://github.com/zrs01/winfit/releases/latest)

Set application window position and size from the command line on
Windows 10/11.

```
winfit --app notepad --left 100 --top 50     # px from work-area edges
winfit --app notepad -w 50% -H 40%           # % of the work area
winfit --app notepad -l 0 -r 100             # left flush, 100 px from right
winfit -w 800 -k                             # resize, keep aspect
winfit --title firefox -r 0 -b 0             # anchor bottom-right
winfit --monitor 2 --left 0 --top 0          # move onto monitor 2
winfit list                                  # HWND PID APP CLASS TITLE
winfit get --app notepad                     # print its position and size
winfit get --hwnd 0x001206AA --json          # same, machine-readable
winfit --hwnd 0x001206AA -l 0 -t 0           # exact window, from list
winfit --app chrome -l 10% --topmost         # frontmost of several matches
winfit --title obsidian -l 0                 # several matches → pick one
winfit -t 0                                  # no target = own console window
```

Selectors `--title` and `--app` are case-insensitive substrings
(`--app chrome` also matches `chrome.exe`'s crashpad helper names, etc.).
When a selector matches more than one window, winfit shows a list of the
matches: up/down move, type to filter, enter picks, esc cancels (exit 1,
nothing moved). In a pipe or script there is nobody to ask, so it errors
with a pointer to `winfit list` (`--hwnd`) or `--topmost` instead.

Position/size values: `123` (logical px), `25%` (of the reference monitor's
work area; `100%` fills it). Flags: `-t/--top`, `-b/--bottom`, `-l/--left`,
`-r/--right`, `-w/--width`, `-H/--height` (`-h` is help), `-k/--keep-aspect`,
`--title`, `--app`, `--hwnd`, `--topmost`, `--monitor`.

`winfit get` reads a window without moving it: same selectors, plus
`--json`. The `pos`/`size`/`from` numbers are exactly what the flags take
(logical px from the work-area edges), so a measured window can be restored
verbatim:

```
"Untitled - Notepad" hwnd 0x000D0DFE pid 16636 app Notepad class Notepad
  pos     left 100 (3.9%)   top 200 (14.4%)
  size    width 800 (31.2%)  height 600 (43.1%)
  from    right 1660 (64.8%)  bottom 592 (42.5%)
  rect    150,300 1200x900 px on work area 0,0 3840x2088  monitor 1 \\.\DISPLAY11 @150%
```

`--json` emits one object with `hwnd`, `pid`, `app`, `class`, `title`,
`rect` and `workArea` (physical px), `position`/`size`/`from` (logical px,
flag-ready), `percent`, `monitor` (`index` + `name`), `scale`, `minimized`,
`maximized`.

Guarantees: a resized window is always fully inside the work area;
move-only keeps ≥ 64 px of the window reachable; windows owned by another
user are refused. Ambiguous matches are resolved by the selection list
above (or `winfit list` → `--hwnd`, or `--topmost`).

Exit codes: `0` success, `1` runtime error, `2` usage error.

## Download

Prebuilt binaries for Windows x64 and ARM64 are on the
[releases page](https://github.com/zrs01/winfit/releases/latest): one zip per
architecture (`winfit_<version>_windows_amd64.zip`), plus `checksums.txt`.
Unzip and put `winfit.exe` anywhere on `PATH`.

Cutting a release: `git tag vX.Y.Z && git push origin vX.Y.Z` — the
`release` workflow runs GoReleaser (`.goreleaser.yaml`) and publishes the
zips, checksums and changelog automatically.

## Build & test

```
make            # build bin/winfit.exe (version stamped from git describe)
make test       # go test ./...  (geometry + selection logic are unit tests)
make windows    # cross-build windows_amd64 binary into build/
```

`go build ./...` passes on any OS; window operations are Windows-only
(`internal/winapi` stubs elsewhere). Dependencies: `urfave/cli/v3` for the
CLI, `charmbracelet/bubbletea` for the ambiguous-match selection list.

Architecture, semantics and decisions: [AGENTS.md](AGENTS.md).
