// Command nitpicker-tui browses a nitpicker findings store
// (docs/audit/findings/) in the terminal: search, filter by state, lens,
// severity and category, read a finding rendered as markdown, and rescan.
package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/ivuorinen/nitpicker-tui/internal/findings"
	"github.com/ivuorinen/nitpicker-tui/internal/ui"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage: nitpicker-tui [-version] [dir]

Browse the nitpicker findings store (docs/audit/findings) of the project
containing dir, the current directory by default. Press ? for every key.

`

// exit is os.Exit; a test replaces it to run main in-process.
var exit = os.Exit

func main() {
	exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole program; its result is the exit code: 0 on success, 1
// when the store cannot be found or read or the terminal UI fails, 2 for a
// usage error. opts reach the program ui.Run starts: tests pass its input
// and output, and main passes none, so bubbletea keeps its own terminal
// handling, the TTY it opens when stdin is piped included.
func run(args []string, stdout, stderr io.Writer, opts ...ui.Option) int {
	flags := flag.NewFlagSet("nitpicker-tui", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = io.WriteString(stderr, usage)
		flags.PrintDefaults()
	}
	showVersion := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}

		return 2
	}
	if *showVersion {
		_, _ = fmt.Fprintln(stdout, version)

		return 0
	}
	if flags.NArg() > 1 {
		flags.Usage()

		return 2
	}

	// A missing store, a failed first load and a failed UI leave through one
	// exit path.
	dir, err := findStore(cmp.Or(flags.Arg(0), "."))
	if err == nil {
		err = ui.Run(newLoader(dir), opts...)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "nitpicker-tui:", err)

		return 1
	}

	return 0
}

// findStore returns the findings store of start or of its nearest ancestor
// that has one, the way git finds .git. Only a path that does not exist moves
// the search up: any other error is returned, so it is neither misreported as
// a missing store nor answered with an ancestor's. That is a permission error
// anywhere, and outside Windows, which reports both as a missing path, a
// symlink loop or a name too long.
func findStore(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "docs", "audit", "findings")
		info, err := os.Stat(candidate)
		switch {
		case err == nil && info.IsDir():
			return candidate, refuseSymlinks(dir, "docs", "audit", "findings")
		case err != nil && !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR):
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no docs/audit/findings in %s or any directory above it", start)
		}
		dir = parent
	}
}

// refuseSymlinks fails when any of parts, joined one by one under base, is a
// symlink. findings.py refuses a store reached through a symlink; os.Stat and
// os.OpenRoot would follow one, letting a cloned repository point the browser
// at another directory on the machine. An Lstat can fail only when the path
// vanishes after findStore's Stat; that error leaves through the branch the
// refusal takes.
func refuseSymlinks(base string, parts ...string) error {
	p := base
	for _, part := range parts {
		p = filepath.Join(p, part)
		info, err := os.Lstat(p)
		if err == nil && info.Mode()&fs.ModeSymlink != 0 {
			err = fmt.Errorf("refusing to follow the symlink %s", p)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// newLoader returns the function that loads the store at dir, at startup and
// for every rescan. Each call opens the store through os.Root, which confines
// every read to dir, and closes it before returning, so no descriptor stays
// open between loads and a store directory replaced by git is picked up.
func newLoader(dir string) ui.Reload {
	return func() (findings.Store, error) {
		root, err := os.OpenRoot(dir)
		if err != nil {
			return findings.Store{}, err
		}
		defer func() { _ = root.Close() }()

		return findings.Load(root.FS())
	}
}
