// Command qomfy is a Go CLI that submits ComfyUI workflows and downloads their
// outputs. It is the 1:1 Go rewrite of the Python script cc.py.
package main

import (
	"os"

	"qomfy/internal/commands"
)

func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		// Cobra already prints the error; ensure a non-zero exit.
		os.Exit(1)
	}
}
