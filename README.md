# NX subscription page themes

Themes for the page an [NX](https://nx-panel.org) panel shows when a user
opens their subscription address in a browser: their usage, expiry, the QR
code and the buttons that add the subscription to their app.

A theme is a directory holding an `index.html` (a Go `html/template` the
panel fills in) and any files it uses. No build step and no restart.

**How to write one:** [docs.nx-panel.org/addons/sub-themes](https://docs.nx-panel.org/addons/sub-themes)

| Directory | What it is |
| --- | --- |
| [`starter/`](starter/) | The smallest complete page, commented to be copied: usage, expiry, QR code, app buttons, configurations, files, notices, live refresh. One HTML file and one stylesheet. |
| [`builtin/`](builtin/) | The panel's own two pages, violet and amber, copied from panel `v0.1.0`. |
| [`check/`](check/) | `go run ./check`: parses every theme the way the panel does. CI runs it on every pull request. |

## Install a theme

On the panel's server, copy the theme's directory into the themes directory:

| Install | Themes directory |
| --- | --- |
| Standard (systemd) | `/var/opt/nx-panel/sub-themes/` |
| Docker | `./sub-themes/` next to the compose file |

```sh
git clone https://github.com/nx-panel/sub-themes /tmp/sub-themes
cp -r /tmp/sub-themes/starter /var/opt/nx-panel/sub-themes/my-theme
```

Then on the panel: **Settings → Subscriptions → Subscription page**, click
**Rescan the themes directory** and pick the theme under **Theme**.

To have every theme here and update them with `git pull`, clone the
repository as the themes directory itself, while that directory is still
empty:

```sh
git clone https://github.com/nx-panel/sub-themes /var/opt/nx-panel/sub-themes
```

(In Docker, clone it to `./sub-themes` next to the compose file.) Each
top-level directory with an `index.html` becomes a theme; `builtin/`,
`check/` and hidden directories are skipped.

## Write a theme

1. Copy `starter/` (or a page from `builtin/`) to a new directory. The name
   becomes the theme's name: letters, digits, `.`, `_` and `-`.
2. Put it in a panel's themes directory and open a user's subscription
   address with `/page` added. Edits show within a few seconds.
3. Run `go run ./check` before you commit.

A theme that fails to load falls back to the panel's built-in page, so a
mistake never costs a user their subscription. The panel log says why.

## Add your theme here

Pull requests with new themes are welcome. A theme here:

- is a top-level directory with an `index.html` and a short `README.md`
  (what it looks like, with a screenshot, and who made it);
- loads nothing from another host: no web fonts, scripts or images from a
  CDN. The page is opened by people whose VPN is not connected yet;
- works in all four page languages (`en`, `fa`, `ru`, `zh`) and right to left
  for Persian. Use the panel's phrases (`.T`) or carry your own for all four;
- passes `go run ./check`;
- is released under this repository's MIT licence.

## Licence

MIT, see [LICENSE](LICENSE).
