package platform

import "testing"

func fakeExists(paths ...string) func(string) bool {
	set := make(map[string]bool, len(paths))
	for _, path := range paths {
		set[path] = true
	}
	return func(path string) bool { return set[path] }
}

// `PATH=.;C:\...` makes cmd.exe resolve a bare `npm` out of the directory
// opencodex was launched from. The updater runs npm with the user's
// privileges, so an npm.cmd dropped in a repository would execute simply
// because someone opened that repository. The oracle skips relative entries for
// exactly this reason (src/update/npm-invocation.mjs:55).
func TestWindowsCommandSkipsRelativePathEntries(t *testing.T) {
	resolved := ResolveWindowsCommandIn(
		"npm",
		`.;C:\Program Files\nodejs`,
		".CMD",
		`C:\Users\dev\project`,
		fakeExists(`npm.cmd`, `C:\Program Files\nodejs\npm.cmd`),
	)
	if resolved != `C:\Program Files\nodejs\npm.cmd` {
		t.Fatalf("resolved %q — a relative PATH entry won", resolved)
	}
}

// The working directory is skipped even when PATH names it absolutely.
func TestWindowsCommandSkipsTheWorkingDirectory(t *testing.T) {
	resolved := ResolveWindowsCommandIn(
		"npm",
		`C:\Users\dev\project;C:\Program Files\nodejs`,
		".CMD",
		`C:\Users\dev\project`,
		fakeExists(`C:\Users\dev\project\npm.cmd`, `C:\Program Files\nodejs\npm.cmd`),
	)
	if resolved != `C:\Program Files\nodejs\npm.cmd` {
		t.Fatalf("resolved %q — the working directory won", resolved)
	}
}

// Exact-directory test, not a subtree test: npm's default global prefix is
// %AppData%\npm, so excluding everything under the cwd would fail closed for
// anyone whose shell sits in their home directory.
func TestWindowsCommandKeepsDirectoriesBeneathTheWorkingDirectory(t *testing.T) {
	resolved := ResolveWindowsCommandIn(
		"npm",
		`C:\Users\dev\AppData\Roaming\npm`,
		".CMD",
		`C:\Users\dev`,
		fakeExists(`C:\Users\dev\AppData\Roaming\npm\npm.cmd`),
	)
	if resolved != `C:\Users\dev\AppData\Roaming\npm\npm.cmd` {
		t.Fatalf("resolved %q — a legitimate npm prefix under the home directory was skipped", resolved)
	}
}

// Ordinary resolution still works, including UNC entries and quoted PATH
// segments, and an unresolvable command is handed back for the OS to resolve.
func TestWindowsCommandResolvesNormally(t *testing.T) {
	if got := ResolveWindowsCommandIn("npm", `"C:\tools"`, ".CMD", `C:\elsewhere`, fakeExists(`C:\tools\npm.cmd`)); got != `C:\tools\npm.cmd` {
		t.Fatalf("quoted PATH entry: %q", got)
	}
	if got := ResolveWindowsCommandIn("npm", `\\server\share\bin`, ".CMD", `C:\elsewhere`, fakeExists(`\\server\share\bin\npm.cmd`)); got != `\\server\share\bin\npm.cmd` {
		t.Fatalf("UNC PATH entry: %q", got)
	}
	if got := ResolveWindowsCommandIn("npm", `C:\tools`, ".CMD", `C:\elsewhere`, fakeExists()); got != "npm" {
		t.Fatalf("unresolvable command should be returned unchanged, got %q", got)
	}
	if got := ResolveWindowsCommandIn(`C:\explicit\npm.cmd`, `C:\tools`, ".CMD", `C:\elsewhere`, fakeExists()); got != `C:\explicit\npm.cmd` {
		t.Fatalf("an explicit path must not be re-resolved, got %q", got)
	}
}
