# Beacon feature map

What a user can do in Beacon, and how to drive each one with `drive.py`. Every
file answers the same four questions: what the feature is, how a user reaches
it, how to drive it, and what goes wrong.

| Feature                                   | Covers                                          |
| ----------------------------------------- | ----------------------------------------------- |
| [server-list.md](server-list.md)          | the list, selection, filter, the detail column  |
| [console.md](console.md)                  | the log screen, tabs, noise filter, search, rail |
| [lifecycle.md](lifecycle.md)              | start, stop, force kill, mark stopped           |
| [adding-a-server.md](adding-a-server.md)  | the folder picker, import, the start-script fix |
| [config-editor.md](config-editor.md)     | the server.properties editor: sections, scrolling, validation |

Not yet mapped: launch settings, the self-update banner, the `?` help grid.
Write the file when you first need to verify one.

## Screens

Beacon has three screens and a handful of modals. `internal/ui/ui.go` holds the
`screen` enum; the modal is whichever of `m.pat`, `m.pick`, `m.launch`,
`m.console`, `m.logSearch`, `m.actions`, `m.config` is non-nil. `m.actions` is
the overlay that `a` opens on the console; `m.config` is the server.properties
editor it leads to.

```
screenList  --right/enter-->  screenMenu  --enter on "Open console"-->  screenConsole
            <--left/esc----               <--esc-----------------------
```
