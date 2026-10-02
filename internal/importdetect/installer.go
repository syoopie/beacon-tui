package importdetect

import (
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// forgeInstall is a Forge or NeoForge server shipped as an installer jar. Packs
// such as AllTheMods ship only the installer and a start script that loops and
// runs Java as a child, so Beacon runs the installer itself on first start and
// then execs Java with the argument files the installer writes.
type forgeInstall struct {
	loader  string // "forge" or "neoforge"
	version string // as the installer names it: "1.20.1-47.4.20", "21.1.251"
	jar     string // the installer's file name
}

var installerRe = regexp.MustCompile(`^(neoforge|forge)-(.+)-installer\.jar$`)

// findInstaller returns the Forge or NeoForge installer in dir, if there is one.
func findInstaller(dir string) (forgeInstall, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return forgeInstall{}, false
	}
	for _, e := range entries {
		if m := installerRe.FindStringSubmatch(e.Name()); m != nil && !e.IsDir() {
			return forgeInstall{loader: m[1], version: m[2], jar: e.Name()}, true
		}
	}
	return forgeInstall{}, false
}

// argsFile is the JVM argument file the installer writes, relative to the
// server directory.
func (f forgeInstall) argsFile() string {
	if f.loader == "neoforge" {
		return path.Join("libraries/net/neoforged/neoforge", f.version, "unix_args.txt")
	}
	return path.Join("libraries/net/minecraftforge/forge", f.version, "unix_args.txt")
}

// command installs the loader when its argument file is missing, then execs
// Java, so the pane's process is the server once it is up. The trailing "beacon"
// is $0 for sh -c; arguments appended by LaunchOption.Command become "$@".
func (f forgeInstall) command() string {
	args := f.argsFile()
	return "sh -c 'test -f " + args + " || java -jar " + f.jar + " --installServer || exit 1; " +
		"exec java @user_jvm_args.txt @" + args + ` "$@"' beacon`
}

func (f forgeInstall) label() string {
	name := "Forge"
	if f.loader == "neoforge" {
		name = "NeoForge"
	}
	return name + " " + f.version + " (Beacon installs it on first start)"
}

// mcVersion is the Minecraft version the installer targets. Forge names it
// ("1.20.1-47.4.20"); NeoForge encodes it in its own version, where 21.1.x
// targets Minecraft 1.21.1 and, from 26 on, the year-based 26.1.x targets 26.1.
func (f forgeInstall) mcVersion() string {
	if f.loader == "forge" {
		return cleanVersion(f.version)
	}
	parts := strings.Split(f.version, ".")
	if len(parts) < 2 {
		return ""
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return ""
	}
	if major >= 26 {
		return parts[0] + "." + parts[1]
	}
	if parts[1] == "0" {
		return "1." + parts[0]
	}
	return "1." + parts[0] + "." + parts[1]
}

var managedRe = regexp.MustCompile(`^sh -c 'test -f libraries/net/(neoforged/neoforge|minecraftforge/forge)/([^/]+)/unix_args\.txt `)

// InstallerLabel names the loader a Beacon-managed installer launch runs, such
// as "NeoForge 21.1.251", or "" when start is some other command. The UI shows
// it in place of the shell line.
func InstallerLabel(start string) string {
	m := managedRe.FindStringSubmatch(start)
	if m == nil {
		return ""
	}
	if m[1] == "neoforged/neoforge" {
		return "NeoForge " + m[2]
	}
	return "Forge " + m[2]
}
