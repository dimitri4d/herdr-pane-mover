package main

import (
	"context"
	"errors"
	"fmt"
	"os"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Pane Mover:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: pane-mover move|picker|new-tab|new-workspace|version")
	}
	if args[0] == "version" {
		fmt.Println(version)
		return nil
	}
	if os.Getenv("HERDR_ENV") != "1" {
		return errors.New("run this action inside Herdr")
	}
	c := newClient()
	ctx := context.Background()
	if args[0] == "picker" {
		return runPicker(c, os.Getenv("PANE_MOVER_SOURCE"))
	}
	if args[0] != "move" && args[0] != "new-tab" && args[0] != "new-workspace" {
		return errors.New("unknown action")
	}
	return performAction(ctx, c, args[0])
}

func performAction(ctx context.Context, c Client, action string) error {
	src, e := c.pane(ctx, "pane", "current", "--current")
	if e != nil {
		return e
	}
	if action == "move" {
		_, e = c.call(ctx, "plugin", "pane", "open", "--plugin", "herdr-pane-mover", "--entrypoint", "picker", "--env", "PANE_MOVER_SOURCE="+src.ID, "--focus")
		return e
	}
	d := Destination{Kind: "new-workspace"}
	if action == "new-tab" {
		d = Destination{Kind: "workspace", Workspace: src.Workspace}
	}
	return c.move(ctx, src, d, "right")
}
