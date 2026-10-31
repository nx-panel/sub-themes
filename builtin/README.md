# The panel's built-in pages

Copies of the two subscription pages every NX panel ships, to start a theme
from. They are not themes in this directory: the panel looks one level deep
only, so a clone of this repository used as the themes directory does not
list them twice. To start from one, copy its directory to the top level under
a new name:

```sh
cp -r builtin/violet my-theme
```

| Page | Look |
| --- | --- |
| [`violet`](violet/index.html) | The default. The panel's own colours, system fonts with Vazirmatn for Persian, tabs for account, setup and advanced, a usage ring and history. |
| [`amber`](amber/index.html) | The original warm serif page with an amber accent. |

Each is one self-contained `index.html`. The comments in them name files in
the panel's source, which is not public; the
[theme guide](https://docs.nx-panel.org/addons/sub-themes) documents every
value they read.

These files are copied unchanged from the panel release named in the
top-level README and are updated with each release that changes them. A pull
request against them is better sent as an issue: the panel's copy is the one
that ships.
