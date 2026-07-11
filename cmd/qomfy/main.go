// Command qomfy is a Go CLI that submits ComfyUI workflows and downloads their
// outputs. It is the 1:1 Go rewrite of the Python script cc.py.
package main

import (
	"os"

	"qomfy/internal/commands"
)

func main() {
	cmd, err := commands.RootCmd.ExecuteC()
	if err != nil {
		commands.PrintError(cmd, err)
		os.Exit(1)
	}
}
