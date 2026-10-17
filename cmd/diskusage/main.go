package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"diskusage/internal/ui"
)

var version = "dev"

func main() {
	var (
		cross    bool
		apparent bool
		workers  int
		trash    bool
		showVer  bool
	)
	flag.BoolVar(&cross, "cross", false, "descend into other filesystems (mount points)")
	flag.BoolVar(&apparent, "apparent", false, "use apparent file sizes instead of the space actually used on disk")
	flag.IntVar(&workers, "j", runtime.NumCPU()*4, "number of directories read in parallel")
	flag.BoolVar(&trash, "trash", false, "move deleted items to the Trash instead of deleting them permanently")
	flag.BoolVar(&showVer, "version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `diskusage %s — find what is eating your disk

usage: diskusage [flags] [path]

flags:
`, version)
		flag.PrintDefaults()
	}
	flag.Parse()

	if showVer {
		fmt.Println("diskusage", version)
		return
	}

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fatal(err)
	}
	abs = filepath.Clean(abs)
	if _, err := os.Lstat(abs); err != nil {
		fatal(err)
	}

	err = ui.Run(abs, ui.Options{
		Cross:    cross,
		Apparent: apparent,
		Workers:  workers,
		Trash:    trash,
	})
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "diskusage:", err)
	os.Exit(1)
}
