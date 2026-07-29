package platform

import (
	"os"
	"strings"
)

// ResolveWindowsCommandIn resolves a bare command name against PATH/PATHEXT the
// way cmd.exe would, minus the part of that behaviour we do not want.
//
// Two guards, both ported from the oracle (src/update/npm-invocation.mjs):
//
//  1. Relative PATH entries are skipped. A relative entry resolves against the
//     working directory, so `PATH=.;C:\...` makes an `npm.cmd` dropped in a
//     project directory win over the real one. The update path runs `npm` with
//     the user's privileges, so that is arbitrary code execution from opening a
//     repository.
//
//  2. The working directory itself is skipped even when listed absolutely. This
//     is deliberately an exact-directory test rather than a subtree test: npm's
//     default Windows global prefix is %AppData%\npm, so excluding everything
//     under the cwd would fail closed for anyone whose shell sits in their home
//     directory — a normal setup, not the untrusted-project case being guarded.
//
// Returning the command unchanged when nothing matches preserves the previous
// behaviour of letting the OS resolve it.
func ResolveWindowsCommandIn(command, pathValue, pathExtValue, workingDirectory string, exists func(string) bool) string {
	if windowsExt(command) != "" || strings.ContainsAny(command, `\/`) {
		return command
	}
	if exists == nil {
		exists = func(path string) bool { _, err := os.Stat(path); return err == nil }
	}
	if pathExtValue == "" {
		pathExtValue = ".COM;.EXE;.BAT;.CMD"
	}
	extensions := strings.FieldsFunc(pathExtValue, func(r rune) bool { return r == ';' || r == ':' })
	for _, entry := range strings.Split(pathValue, ";") {
		entry = strings.TrimSpace(entry)
		entry = strings.Trim(entry, `"`)
		if entry == "" || !isAbsoluteWindowsPath(entry) || sameWindowsDirectory(entry, workingDirectory) {
			continue
		}
		for _, extension := range extensions {
			candidate := joinWindowsPath(entry, command+strings.ToLower(strings.TrimSpace(extension)))
			if exists(candidate) {
				return candidate
			}
		}
	}
	return command
}

// isAbsoluteWindowsPath recognises drive-letter and UNC paths without relying on
// filepath.IsAbs, which answers for the host platform rather than Windows.
func isAbsoluteWindowsPath(entry string) bool {
	if strings.HasPrefix(entry, `\\`) || strings.HasPrefix(entry, "//") {
		return true
	}
	if len(entry) >= 3 && entry[1] == ':' && (entry[2] == '\\' || entry[2] == '/') {
		return true
	}
	return false
}

func sameWindowsDirectory(left, right string) bool {
	if strings.TrimSpace(right) == "" {
		return false
	}
	normalize := func(path string) string {
		path = strings.ReplaceAll(path, "/", `\`)
		path = strings.TrimRight(path, `\`)
		return strings.ToLower(path)
	}
	return normalize(left) == normalize(right)
}

// windowsExt reports a trailing extension using Windows separators, since
// path/filepath answers for the HOST platform and this logic is about Windows
// paths regardless of where the test runs.
func windowsExt(command string) string {
	for index := len(command) - 1; index >= 0; index-- {
		switch command[index] {
		case '.':
			return command[index:]
		case '\\', '/', ':':
			return ""
		}
	}
	return ""
}

func joinWindowsPath(directory, name string) string {
	return strings.TrimRight(strings.ReplaceAll(directory, "/", `\`), `\`) + `\` + name
}
