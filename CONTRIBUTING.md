# Adding a theme

A theme is accepted when its pull request is merged. Everything below is
what that takes: most of it `go run ./check` tests for you, the rest is what
the review reads.

## 1. Start from a copy

```sh
cp -r starter my-theme
```

or from one of the panel's own pages under `builtin/`. The directory's name
is the theme's name, for good: letters, digits, `.`, `_` and `-`, at most 64,
not starting with a dot. Pick something that says what it looks like
(`midnight`, `paper-rtl`), not who made it.

Work on it on a panel of your own: put the directory in the panel's themes
directory and open a user's subscription address with `/page` added. Edits
show within a few seconds. [The theme guide](https://docs.nx-panel.org/addons/sub-themes)
lists every value the page receives.

## 2. The files

| File | What |
| --- | --- |
| `index.html` | The page. |
| `theme.json` | Who made it and what it is called, below. |
| `README.md` | What it looks like and anything special about it. |
| `preview.png` | A screenshot at phone width (390 × 844 is a good size): 300 to 1600 pixels wide, 300 to 4000 tall, at most 300 KB. Take it with Persian or English, light or dark, whichever shows the theme best. |
| anything else | Stylesheets, scripts, images and fonts the page uses, under `{{ .AssetBase }}`. |

The whole directory stays under 2 MB and 200 files. Only `.html`, `.css`,
`.js`, `.json`, `.md`, `.png`, `.jpg`, `.webp`, `.svg` and `.woff2` files; no
links, no hidden files or directories.

### `theme.json`

```json
{
  "name": "my-theme",
  "version": "1.0.0",
  "title": { "en": "My theme", "fa": "…", "ru": "…", "zh": "…" },
  "description": { "en": "One sentence on how it looks.", "fa": "…", "ru": "…", "zh": "…" },
  "author": { "name": "Your name", "github": "your-github-account" },
  "panelMin": "0.1.0"
}
```

| Field | Rule |
| --- | --- |
| `name` | The directory's name. |
| `version` | `MAJOR.MINOR.PATCH`. Start at `1.0.0`; raise it in every pull request that changes the theme. |
| `title` | Up to 60 characters, in all four languages. |
| `description` | Up to 300 characters, in all four languages. |
| `author.github` | Your GitHub account. You are asked to review any change someone else proposes to your theme. |
| `panelMin` | The oldest panel release the theme works on: the release that added the newest value it reads. `0.1.0` when in doubt. |

Nothing else may be in the file.

## 3. The rules

**Load nothing from another host.** The page is opened by someone whose VPN
is not connected yet: a font, script or image from a CDN can leave them a
blank page. Ship what you use in the directory. `check` refuses a remote
`src`, `srcset`, `<link href>`, `<form action>`, `@import`, `url()`, and a
script that sends a request (`fetch`, `XMLHttpRequest`, `WebSocket`,
`EventSource`, `sendBeacon`) or sets a `src` to another host. Links the user
clicks (`<a href>`, `window.open`) may go anywhere: app stores, your support
channel.

**Send nothing anywhere.** The page holds the user's subscription address
and every configuration on it. A script may read `/info` (the address with
`/info` added) and nothing else. No analytics, no counters, no error
reporting.

**No code from text.** No `eval`, `new Function`, a string passed to
`setTimeout` / `setInterval`, a dynamic `import()` or `document.write`. An
SVG has no `<script>`, `<foreignObject>` or `on…` attributes.

**Four languages, right to left.** The page is shown in `en`, `fa`, `ru` and
`zh`, and Persian is right to left. Use the panel's phrases (`index .T "…"`)
or carry your own text for all four. Print numbers and dates through `num`,
`bytes`, `date` and `datetime`, which write Persian digits and the Solar
Hijri calendar on a Persian page.

**MIT.** Your theme is released under this repository's licence, and you
have the right to everything in it: fonts under a licence that allows
redistribution (OFL is fine), images you made or may share.

## 4. Before you open the pull request

```sh
go run ./check
go test ./...
```

The pull request's CI runs the same, and also checks that a changed theme
raised its version.

One theme per pull request. The review reads every line of script, opens the
page in each language, light and dark, at phone width, and checks the
preview matches. Expect questions about anything a script does that the
page does not obviously need.

## Changing a theme that is here

Raise `version` in its `theme.json`, every time. If it is not your theme,
say why in the pull request; its author is asked to review it.
