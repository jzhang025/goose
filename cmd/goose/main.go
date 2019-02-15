package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tchajed/goose"
)

func main() {
	var config goose.Config
	flag.BoolVar(&config.AddSourceFileComments, "source-comments", false,
		"add comments indicating Go source code location for each top-level declaration")

	var outFile string
	flag.StringVar(&outFile, "out", "-",
		"file to output to (use '-' for stdout)")

	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Usage: goose <path to source dir>")
		os.Exit(1)
	}
	srcDir := flag.Arg(0)

	f, err := config.TranslatePackage(srcDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if outFile == "-" {
		f.Write(os.Stdout)
	} else {
		out, err := os.Create(outFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			fmt.Fprintln(os.Stderr, "could not write output")
			os.Exit(1)
		}
		defer out.Close()
		f.Write(out)
	}
}
