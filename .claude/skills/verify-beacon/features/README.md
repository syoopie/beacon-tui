# Beacon feature map

What a user can do in Beacon, and how to drive each one with `drive.py`. Every
file answers the same four questions: what the feature is, how a user reaches
it, how to drive it, and what goes wrong.

| Feature                                   | Covers                                          |
| ----------------------------------------- | ----------------------------------------------- |
| [server-list.md](server-list.md)          | the list, selection, the always-on filter        |
| [console.md](console.md)                  | the log screen, tabs, noise filter, search, rail, the input |
| [lifecycle.md](lifecycle.md)              | start, stop, force kill, mark stopped           |
| [adding-a-server.md](adding-a-server.md)  | the folder picker, import, the start-script fix |
| [config-editor.md](config-editor.md)     | the server.properties editor: sections, scrolling, validation |

Not yet mapped as its own file: the launch-settings dialog (`m.launch`,
`internal/ui/launch.go`) beyond its Java-runtime row, which
[lifecycle.md](lifecycle.md) covers, and the self-update banner
(`internal/selfupdate`, `m.update`). Write the file when you first need to
verify one.

## Screens

Beacon has two screens and a handful of modals. `internal/ui/menu.go` holds the
`screen` enum: `screenList` and `screenConsole`, nothing between them. The modal
is whichever of `m.pat`, `m.pick`, `m.launch`, `m.console`, `m.logSearch`,
`m.actions`, `m.config` is non-nil. `m.actions` is the settings overlay that `a`
opens on the console; `m.config` is the server.properties editor it leads to,
`m.launch` the launch-settings dialog.

```
screenList  --right/enter on a server-->  screenConsole
            <--esc--------------------- (left is a no-op on the console)
```

The list has no detail column and no per-server menu any more: `→` or `enter`
goes straight to the selected server's console, and every action lives there.
