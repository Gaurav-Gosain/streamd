# vhs/

[VHS](https://github.com/charmbracelet/vhs) tape scripts that generate
every GIF embedded in the root README.

## Prerequisites

```sh
brew install vhs jq     # or go install github.com/charmbracelet/vhs@latest
```

A JetBrains Mono Nerd Font (or any monospace Nerd Font) gives the tapes
the glyph coverage they need. Any monospace font will still render; you
just lose a handful of decorative characters.

## Build

```sh
make -C vhs            # builds binary + every gif into vhs/out/
make -C vhs gif-hero   # one at a time
make -C vhs clean      # drop generated gifs
```

The Makefile compiles a fresh `streamd` binary into `../bin` and
prepends that directory to `$PATH` during each `vhs` run, so the gifs
always match the current source.

## Layout

```
vhs/
├── _common.tape       shared theme / font / size settings
├── fixtures/          shell scripts that simulate streaming responses
│   ├── stream-sse.sh       OpenAI Chat Completions SSE
│   ├── stream-long.sh      long response that overflows the viewport
│   ├── stream-thinking.sh  reasoning_content + final answer
│   └── stream-info.sh      response with usage info chunk
├── out/               generated .gif files (checked into the repo)
├── hero.tape          top-of-README headline demo
├── inline.tape        default inline mode
├── alt.tape           --alt scrollable viewport
├── thinking.tape      reasoning / thinking tokens
└── info.tape          --info flag with token usage
```

The fixtures emit valid SSE / NDJSON to stdout with small `sleep` gaps
between tokens, so the demos look like real streaming without needing
a running LLM.

## Adding a new tape

1. Copy an existing tape as a starting point.
2. Open it with `Source vhs/_common.tape` so theme/size/font stay in sync.
3. Keep the demo under ~20 seconds.
4. Add it to the README's asset table so readers can find it.
5. Run `make -C vhs gif-<name>` to verify.

## Notes on the settings

`_common.tape` fixes the window size at 1320x760 with 15pt text and the
Tokyo Night palette to match streamd's default `dark` glamour theme.
Bumping `FontSize` or `PlaybackSpeed` past 1.0 inflates the GIF sizes
quickly, so prefer lower values when editing.
