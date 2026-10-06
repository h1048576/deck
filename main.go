package main

import (
	"flag"
	"fmt"
	"os"

	"wide-pure/internal/gui"
)

var version = "0.1.0"

func main() {
	args := os.Args[1:]
	command := ""
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "serve", "web":
		addr := "127.0.0.1:3420"
		if len(args) > 1 {
			addr = args[1]
		}
		if err := gui.Serve(version, addr); err != nil {
			fatal(err)
		}
	case "version", "--version", "-v":
		fmt.Println("wide-pure", version)
	default:
		fs := flag.NewFlagSet("wide-pure", flag.ContinueOnError)
		serve := fs.String("serve", "", "serve the UI in a browser at the given address instead of opening a window")
		if err := fs.Parse(args); err != nil {
			fatal(err)
		}
		if *serve != "" {
			if err := gui.Serve(version, *serve); err != nil {
				fatal(err)
			}
			return
		}
		if err := runGUI(version); err != nil {
			fatal(err)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "wide-pure:", err)
	os.Exit(1)
}
