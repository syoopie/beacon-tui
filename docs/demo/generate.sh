#!/usr/bin/env bash
# Regenerate the README screenshots. Needs vhs (brew install vhs) and the
# JetBrainsMono Nerd Font (brew install --cask font-jetbrains-mono-nerd-font).
#
# It builds a throwaway registry with three fake servers, points one at a
# curated log, and runs each tape against the real binary. The PNGs land next
# to this script.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../.." && pwd)"

demo="$(mktemp -d)"
trap 'rm -rf "$demo"' EXIT
export BEACON_DEMO="$demo"

go build -o "$demo/beacon" "$repo/cmd/beacon"
mkdir -p "$demo/config/servers" "$demo/state/logs"
cp "$here/latest.log" "$demo/state/logs/bmc4_serverpack_v61.log"

printf 'scan_roots = []\nstop_timeout = "1m0s"\n' > "$demo/config/config.toml"

for id in bmc4_serverpack_v61 creative survival; do
	mkdir -p "$demo/mc/$id"
	printf 'eula=true\n' > "$demo/mc/$id/eula.txt"
done
cat > "$demo/mc/bmc4_serverpack_v61/server.properties" <<-'PROP'
	motd=BMC4 modpack server
	server-port=25565
	difficulty=normal
	max-players=20
	enable-rcon=true
	rcon.port=25575
	rcon.password=changeme
PROP

spec() {
	local id=$1 start=$2 script=$3 port=$4
	cat > "$demo/config/servers/$id.toml" <<-SPEC
		id = "$id"
		dir = "$demo/mc/$id"
		start = "$start"
		script = "$script"
		port = $port
		session = "beacon-$id"
		log_file = "$demo/state/logs/$id.log"
		exec_state = "ok"

		[state]
		  last_known = "stopped"
		  pid = 0
	SPEC
}
spec bmc4_serverpack_v61 "./run.sh nogui"          run.sh     25565
spec creative            "java -jar server.jar nogui" ""      25567
spec survival            "./run.sh nogui"          run.sh     25565

cat >> "$demo/config/servers/bmc4_serverpack_v61.toml" <<-RCON

	[rcon]
	  enabled = true
	  port = 25575
	  password = "changeme"
RCON

cd "$here"
for tape in list console actions config; do
	rm -f "$tape.png"
	# vhs drives a fresh headless Chrome per tape and a cold start sometimes
	# finishes after the tape does, so retry until the PNG lands.
	for attempt in 1 2 3; do
		vhs "$tape.tape" || true
		[ -f "$tape.png" ] && break
		echo "retry $tape ($attempt)" >&2
		pkill -f 'rod/browser' 2>/dev/null || true
		sleep 3
	done
	[ -f "$tape.png" ] || { echo "vhs did not produce $tape.png" >&2; exit 1; }
done
rm -f out.gif
echo "wrote $here/{list,console,actions,config}.png"
