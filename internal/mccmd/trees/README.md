# Vendored vanilla command trees

Each `<version>.json.gz` is Mojang's generated `commands.json` for that
Minecraft release: the full Brigadier command graph, the same data the vanilla
client uses to drive its in-game suggestions. `bundled.go` embeds them and picks
one by the server's detected `mc_version`.

Source: [`misode/mcmeta`](https://github.com/misode/mcmeta), tag
`<version>-summary`, path `commands/data.json`. That file is byte-identical in
shape to what `java -jar server.jar --reports` writes to
`generated/reports/commands.json`.

To refresh or add a version:

```sh
v=1.21.11
curl -sL "https://raw.githubusercontent.com/misode/mcmeta/${v}-summary/commands/data.json" \
  | python3 -c 'import json,sys; json.dump(json.load(sys.stdin), sys.stdout, separators=(",",":"), sort_keys=True)' \
  | gzip -9 -n > "${v}.json.gz"
```

Keep the set to a handful of recent releases plus the versions people actually
run modded (1.16.5, 1.20.1). Gzipped they are ~5-8 KB each.
