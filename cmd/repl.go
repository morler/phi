package main

import (
	"github.com/glojurelang/glojure/pkg/repl"
	cli "github.com/pulseaiclub/pli"
)

var replCommand = cli.Command{
	Name: "repl",
	Desc: "start the embedded Glojure REPL",
}

func init() {
	replCommand.Run = func(args []string, _ cli.Flags) error {
		if len(args) != 0 {
			return replCommand.Usagef("unexpected arguments")
		}
		repl.Start()
		return nil
	}
}
