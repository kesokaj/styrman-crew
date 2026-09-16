// Command sh-agent installs, resyncs, inspects, and removes the sh-agent
// harness — global context, skills, and subagents — for Claude Code and the
// Antigravity CLI.
//
//	sh-agent install [--dry-run]     deploy the repo into ~/.claude and ~/.gemini/config
//	sh-agent status                  show drift between the repo and what is installed
//	sh-agent diff                    unified diff of every drifted file
//	sh-agent update [--check]        git fetch; pull --ff-only and reinstall when behind
//	sh-agent uninstall [--dry-run]   remove exactly what install wrote
//	sh-agent list                    print the skill and agent inventory
//	sh-agent render                  print the assembled context file
//	sh-agent adopt <name|path>       copy a skill from elsewhere into the repo
//	sh-agent version
package main

import (
	"bytes"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/kesokaj/styrman-harness/internal/harness"
)

const usage = `sh-agent — lifecycle tool for the sh-agent harness

Commands:
  install [--vendor V] [--dry-run]  deploy context, skills, and agents into ~/.claude and/or ~/.gemini/config (alias: sync)
  status                  show what is installed and whether it drifted from the repo (exit 1 on drift)
  diff                    unified diff of every drifted file
  update [--check]        git fetch the harness repo; pull --ff-only and reinstall when behind (--check only reports, exit 2 when behind)
  uninstall [--dry-run]   remove exactly the files install wrote, nothing else
  list                    print the skill and agent inventory
  render                  print the assembled context file
  adopt <name|path>       copy a skill installed elsewhere into the repo's skills/ (--force to replace)
  version                 print the build version

Flags accepted by every command:
  --repo DIR        harness checkout (default: --repo, $SH_AGENT_REPO, the working directory or its parents, then the last install)
  --home DIR        install under DIR instead of $HOME (for testing)
  --vendor VENDOR   target vendor: all, antigravity (or agy), claude (default: from manifest if installed, else all)
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		var code exitCode
		if errors.As(err, &code) {
			os.Exit(int(code))
		}
		fmt.Fprintln(os.Stderr, "sh-agent:", err)
		os.Exit(1)
	}
}

// exitCode carries a non-error exit status such as "drift found".
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit %d", int(e)) }

func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(out, usage)
		return nil
	}
	cmd, rest := args[0], args[1:]

	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(out)
	repoFlag := fs.String("repo", "", "harness checkout")
	homeFlag := fs.String("home", "", "install under this directory instead of $HOME")
	vendorFlag := fs.String("vendor", "", "target vendor: all, antigravity (or agy), claude")
	dryRun := fs.Bool("dry-run", false, "print what would change, change nothing")
	check := fs.Bool("check", false, "update: only report whether the repo is behind")
	force := fs.Bool("force", false, "adopt: replace an existing skill")
	if err := fs.Parse(reorder(rest)); err != nil {
		return err
	}

	home := *homeFlag
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		home = h
	}

	switch cmd {
	case "version":
		fmt.Fprintln(out, "sh-agent", buildVersion())
		return nil
	case "uninstall":
		return harness.Uninstall(home, *dryRun, out)
	}

	repo, err := harness.FindRepo(*repoFlag, home)
	if err != nil {
		return err
	}

	switch cmd {
	case "install", "sync":
		vendor, err := resolveVendor(*vendorFlag, home)
		if err != nil {
			return err
		}
		if err := knownCheckout(*repoFlag, home, repo); err != nil {
			return err
		}
		// The deployed binary must be the repo's own build: rebuild and let
		// that build install, unless this process is that build (ChildEnv),
		// there is no toolchain to build with, the tree is a plain copy, or
		// this is a dry run — a dry run never builds or runs a fresh binary;
		// like every install it still reads the checkout's git state, which
		// is why the checkout must be one the operator named or installed
		// before. A plain copy (no .git: a mirror inside a project's image)
		// never rebuilds — its image or VM build step built the running binary
		// from that same tree, the tree is often read-only to the installing user,
		// and git cannot update it anyway (BUG-015).
		_, goErr := exec.LookPath("go")
		own := harness.IsCheckout(repo)
		rebuild := shouldRebuild(goErr == nil, os.Getenv(harness.ChildEnv) == "1", own)
		if *dryRun && rebuild {
			fmt.Fprintf(out, "[dry-run] the binary would be rebuilt from %s\n", repo)
		} else if rebuild {
			var pass []string
			if *homeFlag != "" {
				pass = append(pass, "--home", home)
			}
			if *vendorFlag != "" {
				pass = append(pass, "--vendor", vendor)
			}
			return harness.Reinstall(repo, pass, out)
		}
		if !own && *dryRun {
			fmt.Fprintln(out, "[dry-run] plain copy, not its own git checkout: the running binary would be installed as is")
		} else if !own {
			fmt.Fprintln(out, "plain copy, not its own git checkout: the running binary is installed as is")
		} else if goErr != nil {
			fmt.Fprintln(out, "warning: go is not on PATH; the running binary was copied, not rebuilt — install Go and run `sh-agent update`")
		}
		self, err := selfExecutable()
		if err != nil {
			return err
		}
		files, err := harness.Plan(repo, self, vendor)
		if err != nil {
			return err
		}
		if err := harness.Install(repo, home, files, vendor, *dryRun, out); err != nil {
			return err
		}
		if err := pathHint(home, out); err != nil {
			return err
		}
		if vendor == harness.VendorAll || vendor == harness.VendorClaude {
			attributionSet(home, *dryRun, out)
			return attributionHint(home, "note: ", out)
		}
		return nil

	case "status":
		return status(repo, home, *vendorFlag, out)

	case "diff":
		vendor, err := resolveVendor(*vendorFlag, home)
		if err != nil {
			return err
		}
		files, err := harness.Plan(repo, nil, vendor)
		if err != nil {
			return err
		}
		return harness.Diff(home, files, out)

	case "update":
		if err := knownCheckout(*repoFlag, home, repo); err != nil {
			return err
		}
		return update(repo, home, *check, *homeFlag, *vendorFlag, out)

	case "list":
		inv, err := harness.LoadInventory(repo)
		if err != nil {
			return err
		}
		fmt.Fprint(out, harness.RenderInventory(inv))
		return nil

	case "render":
		inv, err := harness.LoadInventory(repo)
		if err != nil {
			return err
		}
		data, err := harness.RenderContext(repo, inv)
		if err != nil {
			return err
		}
		_, err = out.Write(data)
		return err

	case "adopt":
		if fs.NArg() != 1 {
			return errors.New("usage: sh-agent adopt <skill-name-or-path> [--force]")
		}
		dest, err := harness.Adopt(repo, home, fs.Arg(0), *force)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "adopted into %s\nnext: review it, run `sh-agent install`, then commit and push the repo\n", dest)
		return nil
	}
	return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
}

func resolveVendor(flagVal, home string) (string, error) {
	if flagVal != "" {
		return harness.NormalizeVendor(flagVal)
	}
	st, found, err := harness.LoadState(home)
	if err == nil && found && st.Vendor != "" {
		return harness.NormalizeVendor(st.Vendor)
	}
	return harness.VendorAll, nil
}

func status(repo, home, vendorFlag string, out io.Writer) error {
	info := harness.Inspect(repo)
	dirty := "clean"
	if info.Dirty {
		dirty = "uncommitted changes"
	}
	if info.Commit == "" {
		fmt.Fprintf(out, "repo       %s (plain copy, not its own git checkout; update is refused)\n", repo)
	} else {
		fmt.Fprintf(out, "repo       %s (%s @ %s, %s)\n", repo, info.Branch, info.Commit, dirty)
	}

	st, found, err := harness.LoadState(home)
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintln(out, "installed  nothing — run `sh-agent install`")
		return exitCode(1)
	}
	vDisplay := st.Vendor
	if vDisplay == "" {
		vDisplay = harness.VendorAll
	}
	fmt.Fprintf(out, "installed  %s from commit %s (vendor: %s, %d files, manifest ~/.config/sh-agent/state.json)\n", st.InstalledAt, st.Commit, vDisplay, len(st.Files))
	// The line describes the installed file, not this process: status may
	// run from `go run` or from a build outside ~/.local/bin.
	binaryLine := "binary     unknown (installed file has no build info)"
	if bi, err := buildinfo.ReadFile(filepath.Join(home, harness.BinPath)); err == nil {
		rev := versionFromInfo(bi)
		binaryLine = "binary     " + rev
		switch {
		case !binaryOlder(rev, info.Commit):
		case rev == "(devel)" || rev == "unknown":
			binaryLine += " — no build stamp; run `sh-agent update`"
		default:
			binaryLine += fmt.Sprintf(" — older than the repo (%s); run `sh-agent update`", info.Commit)
		}
	}
	fmt.Fprintln(out, binaryLine)
	if last, ok := harness.LastRetro(home); ok {
		days := int(time.Since(last).Hours() / 24)
		hint := ""
		if days > 14 {
			hint = " — past 14 days, run /retro at the end of the next session"
		}
		fmt.Fprintf(out, "retro      last proposal %s (%d days ago)%s\n", last.Format("2006-01-02"), days, hint)
	} else {
		fmt.Fprintln(out, "retro      none yet — run /retro at the end of a session with corrections")
	}

	vendor, err := resolveVendor(vendorFlag, home)
	if err != nil {
		return err
	}

	inv, err := harness.LoadInventory(repo)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "repo has   %d skills, %d agents, %d commands, %d rules\n", len(inv.Skills), len(inv.Agents), len(inv.Commands), len(inv.Rules))
	if vendor == harness.VendorAll || vendor == harness.VendorClaude {
		fmt.Fprintf(out, "claude     ~/%s  (AGENTS.md, CLAUDE.md, skills/, agents/, commands/, rules/)\n", harness.ClaudeRoot)
	}
	if vendor == harness.VendorAll || vendor == harness.VendorAntigravity {
		fmt.Fprintf(out, "agy        ~/%s  (rules/AGENTS.md, skills/, agents/, workflows/, rules/)\n", harness.AgyRoot)
	}

	files, err := harness.Plan(repo, nil, vendor)
	if err != nil {
		return err
	}
	report, err := harness.Status(home, files)
	if err != nil {
		return err
	}
	for _, e := range report.Entries {
		if e.State != "ok" {
			fmt.Fprintf(out, "  %-8s ~/%s\n", e.State, e.Rel)
		}
	}
	if report.Counts["foreign"] > 0 {
		fmt.Fprintln(out, "note       ~/.claude/CLAUDE.md is yours; move only rules meant for every machine into a rule file under rules/ in the harness repo — rules/ ships to every install and mirror, never credentials or personal or host-specific notes — then remove it and run install to write the one-line import")
	}
	if vendor == harness.VendorAll || vendor == harness.VendorClaude {
		if err := attributionHint(home, "note       ", out); err != nil {
			return err
		}
	}
	if !report.Drift() {
		fmt.Fprintf(out, "drift      none — %d files match the repo\n", report.Counts["ok"])
		return nil
	}
	fmt.Fprintf(out, "drift      %d missing, %d differ, %d stale — run `sh-agent diff`, then `sh-agent install`\n",
		report.Counts["missing"], report.Counts["differs"], report.Counts["stale"])
	return exitCode(1)
}

func update(repo, home string, checkOnly bool, homeFlag, vendorFlag string, out io.Writer) error {
	behind, err := harness.Behind(repo)
	if err != nil {
		return err
	}
	if behind == 0 {
		fmt.Fprintf(out, "up to date with upstream (%s)\n", harness.Inspect(repo).Commit)
		if checkOnly {
			return nil
		}
	} else {
		fmt.Fprintf(out, "behind upstream by %d commit(s)\n", behind)
		if checkOnly {
			return exitCode(2)
		}
		if err := harness.Pull(repo, out); err != nil {
			return err
		}
		fmt.Fprintf(out, "pulled to %s\n", harness.Inspect(repo).Commit)
	}
	var args []string
	if homeFlag != "" {
		args = append(args, "--home", home)
	}
	vendor, err := resolveVendor(vendorFlag, home)
	if err != nil {
		return err
	}
	if vendor != harness.VendorAll {
		args = append(args, "--vendor", vendor)
	}
	return harness.Reinstall(repo, args, out)
}

// knownCheckout refuses to build and install from a checkout that was only
// discovered by walking up from the working directory when a different
// checkout is already installed. Building runs that tree's code and deploys
// its context everywhere; standing in a harness-shaped directory (a mirror,
// a clone under review) must not be enough. An explicit --repo or
// $SH_AGENT_REPO, or a first install, is.
func knownCheckout(repoFlag, home, repo string) error {
	if repoFlag != "" || os.Getenv("SH_AGENT_REPO") != "" {
		return nil
	}
	st, found, err := harness.LoadState(home)
	if err != nil {
		return fmt.Errorf("reading the manifest before building: %w", err)
	}
	if !found || st.Repo == "" {
		return nil
	}
	if samePath(st.Repo, repo) {
		return nil
	}
	return fmt.Errorf("checkout %s is not the installed one (%s); pass --repo %s to build and install from it", repo, st.Repo, repo)
}

func samePath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return ra == rb
}

// shouldRebuild decides whether install re-executes through a fresh build
// of the repo: yes whenever a toolchain is present and the repo is its own
// git checkout, unless this process is already the fresh build that
// Reinstall spawned. A plain copy is installed from the running binary.
func shouldRebuild(goOnPath, child, ownCheckout bool) bool {
	return goOnPath && !child && ownCheckout
}

// binaryOlder reports whether the running binary is not the repo's current
// build: unstamped (`go run`, or a build outside git) or stamped with a
// different revision. Prefixes are compared both ways because git's short
// hash length is not fixed.
func binaryOlder(rev, commit string) bool {
	rev = strings.TrimSuffix(rev, "+dirty")
	if rev == "" || commit == "" {
		return false
	}
	if rev == "(devel)" || rev == "unknown" {
		return true
	}
	return !strings.HasPrefix(rev, commit) && !strings.HasPrefix(commit, rev)
}

// reorder moves flags in front of positional arguments so that
// `sh-agent adopt my-skill --home /tmp/x` parses; the flag package stops at
// the first positional argument otherwise.
func reorder(args []string) []string {
	takesValue := map[string]bool{
		"-repo": true, "--repo": true,
		"-home": true, "--home": true,
		"-vendor": true, "--vendor": true,
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "-"):
			flags = append(flags, a)
			if takesValue[a] && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
		default:
			positional = append(positional, a)
		}
	}
	return append(flags, positional...)
}

// selfExecutable returns the running binary so install can copy it to
// ~/.local/bin. On the normal path this is the build Reinstall just made;
// without a toolchain it is whatever binary is running, and status says so.
func selfExecutable() ([]byte, error) {
	p, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if p, err = filepath.EvalSymlinks(p); err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

func pathHint(home string, out io.Writer) error {
	bin := filepath.Join(home, filepath.Dir(harness.BinPath))
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == bin || strings.TrimSuffix(p, "/") == bin {
			return nil
		}
	}
	fmt.Fprintf(out, "note: %s is not on PATH; add it so `sh-agent` is available everywhere\n", bin)
	return nil
}

// attributionSet writes the one vendor setting the harness owns (R4,
// ADR-025): Claude Code's attribution off, so no commit or PR carries the
// trailer the context's Git rule forbids. Only an absent key is written — a
// present value of any kind is the operator's and attributionHint says when
// it is on. The write is a textual insertion before the object's closing
// brace, re-parsed before it lands, because encoding/json would reorder the
// operator's keys; every byte up to the object's last value stays, and the
// file keeps its own line endings. No state of the file
// fails the install: the deployment already succeeded and this file is
// advisory. dryRun prints what would be written and touches nothing.
func attributionSet(home string, dryRun bool, out io.Writer) {
	const fragment = `"attribution": {"commit": "", "pr": ""}`
	path := filepath.Join(home, harness.ClaudeRoot, "settings.json")
	// Lstat, not Stat: a settings.json that is a symlink (a dotfiles checkout)
	// is never written through — a rename would replace the link with a file.
	info, err := os.Lstat(path)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		fmt.Fprintf(out, "settings: %s is a symlink; add the key to its target by hand\n", path)
		return
	case errors.Is(err, os.ErrNotExist):
		if dryRun {
			fmt.Fprintf(out, "[dry-run] would create %s with %s\n", path, fragment)
			return
		}
		content := []byte("{\n  " + fragment + "\n}\n")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(out, "settings: %s could not be written (%v); add the key by hand\n", path, err)
			return
		}
		// O_EXCL: the Lstat above is advisory; whatever appears at the path
		// between the two calls (a planted link, a FIFO) makes this fail
		// instead of being written through or blocked on.
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			fmt.Fprintf(out, "settings: %s could not be written (%v); add the key by hand\n", path, err)
			return
		}
		if _, err := f.Write(content); err != nil {
			f.Close()
			fmt.Fprintf(out, "settings: %s could not be written (%v); add the key by hand\n", path, err)
			return
		}
		if err := f.Sync(); err != nil {
			f.Close()
			fmt.Fprintf(out, "settings: %s could not be written (%v); add the key by hand\n", path, err)
			return
		}
		if err := f.Close(); err != nil {
			fmt.Fprintf(out, "settings: %s could not be written (%v); add the key by hand\n", path, err)
			return
		}
		fmt.Fprintf(out, "settings: %s created with %s — Claude Code's commit and PR trailers are off\n", path, fragment)
		return
	case err != nil, !info.Mode().IsRegular():
		return // attributionHint names it
	case info.Size() > 1<<20:
		fmt.Fprintf(out, "settings: %s is larger than 1 MiB, more than this tool will rewrite; add the key by hand\n", path)
		return
	}
	data, problem := readSettings(path)
	if problem != "" {
		return // attributionHint names it
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || top == nil {
		// malformed, not an object, or the bare value null (a nil map with no
		// error): attributionHint names each with one line
		return
	}
	if _, present := top["attribution"]; present {
		return
	}
	// Insert after the last byte of the object's body, keeping the operator's
	// indentation style (two spaces, the file's own newline before the brace)
	// and whatever follows the closing brace.
	trimmed := bytes.TrimRight(data, " \t\r\n")
	closing := bytes.LastIndexByte(trimmed, '}')
	opening := bytes.IndexByte(trimmed, '{')
	if closing <= opening {
		fmt.Fprintf(out, "settings: %s could not be edited safely (the result would not parse); add the key by hand\n", path)
		return
	}
	nl := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		nl = "\r\n" // keep the file's own line endings
	}
	body := bytes.TrimRight(trimmed[:closing], " \t\r\n")
	insert := "," + nl + "  " + fragment + nl
	if len(bytes.TrimSpace(trimmed[opening+1:closing])) == 0 {
		body = trimmed[:opening+1]
		insert = nl + "  " + fragment + nl
	}
	result := make([]byte, 0, len(data)+len(insert))
	result = append(result, body...)
	result = append(result, insert...)
	result = append(result, trimmed[closing:]...)
	result = append(result, data[len(trimmed):]...)
	if !json.Valid(result) {
		fmt.Fprintf(out, "settings: %s could not be edited safely (the result would not parse); add the key by hand\n", path)
		return
	}
	if dryRun {
		fmt.Fprintf(out, "[dry-run] would add %s to %s\n", fragment, path)
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.json")
	if err != nil {
		fmt.Fprintf(out, "settings: %s could not be written (%v); add the key by hand\n", path, err)
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(result); err != nil {
		tmp.Close() // the file is discarded on the next line; its close result changes nothing
		abandonTemp(tmpName, path, err, out)
		return
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		abandonTemp(tmpName, path, err, out)
		return
	}
	if err := tmp.Close(); err != nil {
		abandonTemp(tmpName, path, err, out)
		return
	}
	if err := os.Chmod(tmpName, info.Mode().Perm()); err != nil {
		abandonTemp(tmpName, path, err, out)
		return
	}
	if err := os.Rename(tmpName, path); err != nil {
		abandonTemp(tmpName, path, err, out)
		return
	}
	fmt.Fprintf(out, "settings: %s — %s added; other keys untouched\n", path, fragment)
}

// abandonTemp reports a failed settings write and removes its temp file,
// naming the temp file when even that fails so nothing is left behind unseen.
func abandonTemp(tmpName, path string, cause error, out io.Writer) {
	fmt.Fprintf(out, "settings: %s could not be written (%v); add the key by hand\n", path, cause)
	if err := os.Remove(tmpName); err != nil {
		fmt.Fprintf(out, "settings: temp file %s left behind (%v)\n", tmpName, err)
	}
}

// readSettings reads the operator's settings file, bounded to a regular file
// of at most one megabyte; problem names why it could not be read, without a
// byte of its contents.
func readSettings(path string) (data []byte, problem string) {
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return nil, "could not be read"
	case !info.Mode().IsRegular():
		return nil, "is not a regular file"
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "could not be read"
	}
	defer f.Close()
	data, err = io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return nil, "could not be read"
	}
	return data, ""
}

// attributionHint says when Claude Code will still add its Co-Authored-By
// trailer. The context's Git rule forbids the trailer, but in the sandbox the
// rule alone did not hold (three of three headless commits carried it); the
// CLI's own setting is what stops it, and settings.json is the operator's
// file, so install and status only say so. prefix is each command's note
// style ("note: " for install, "note       " for status). Advisory only: no
// state of that file may fail install or status, and nothing from it is
// echoed.
func attributionHint(home, prefix string, out io.Writer) error {
	path := filepath.Join(home, harness.ClaudeRoot, "settings.json")
	off, problem := attributionOff(path)
	switch {
	case problem != "":
		fmt.Fprintf(out, "%s%s %s, so the attribution setting could not be checked; Claude Code adds a Co-Authored-By trailer to every commit unless it sets \"attribution\": {\"commit\": \"\"} (context § Git)\n", prefix, path, problem)
	case !off:
		fmt.Fprintf(out, "%s%s does not set \"attribution\": {\"commit\": \"\"}; Claude Code adds a Co-Authored-By trailer to every commit until it does (context § Git)\n", prefix, path)
	}
	return nil
}

// attributionOff reads the operator's settings file, bounded to a regular
// file of at most one megabyte. off is true when attribution.commit is "" or
// false; problem names why the file could not be judged, without a byte of
// its contents (a JSON error string carries the input it choked on).
func attributionOff(path string) (off bool, problem string) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false, ""
	}
	data, problem := readSettings(path)
	if problem != "" {
		return false, problem
	}
	var settings struct {
		Attribution struct {
			Commit any `json:"commit"`
		} `json:"attribution"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		if json.Valid(data) {
			return false, "is not a JSON object"
		}
		return false, "is not valid JSON"
	}
	switch v := settings.Attribution.Commit.(type) {
	case string:
		return v == "", ""
	case bool:
		return !v, ""
	}
	return false, ""
}

// buildVersion describes the running process, for `sh-agent version`.
// `sh-agent status` reads the installed file instead (see status).
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return versionFromInfo(info)
}

// versionFromInfo renders a binary's VCS stamp as the short revision with a
// "+dirty" suffix for a modified tree, or the module version when the build
// carried no stamp (`go run`, `go test`, a build outside git).
func versionFromInfo(info *debug.BuildInfo) string {
	rev, modified := "", ""
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				modified = "+dirty"
			}
		}
	}
	if rev == "" {
		return info.Main.Version
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	return rev + modified
}
