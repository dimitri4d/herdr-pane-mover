package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

type Workspace struct {
	ID    string `json:"workspace_id"`
	Label string `json:"label"`
}
type Tab struct {
	ID        string `json:"tab_id"`
	Workspace string `json:"workspace_id"`
	Label     string `json:"label"`
}
type Pane struct {
	ID        string `json:"pane_id"`
	Workspace string `json:"workspace_id"`
	Tab       string `json:"tab_id"`
	Terminal  string `json:"terminal_id"`
	Label     string `json:"label"`
	Title     string `json:"title"`
}
type Layout struct {
	Workspace string `json:"workspace_id"`
	Tab       string `json:"tab_id"`
	Active    string `json:"focused_pane_id"`
	Zoomed    *bool  `json:"zoomed"`
}
type MoveResult struct {
	Changed *bool  `json:"changed"`
	Reason  string `json:"reason"`
	Pane    Pane   `json:"pane"`
	Focused string `json:"focused_pane_id"`
}
type Result struct {
	Type       string      `json:"type"`
	Workspaces []Workspace `json:"workspaces"`
	Tabs       []Tab       `json:"tabs"`
	Panes      []Pane      `json:"panes"`
	Pane       *Pane       `json:"pane"`
	Layout     *Layout     `json:"layout"`
	Move       *MoveResult `json:"move_result"`
}
type runner func(context.Context, ...string) ([]byte, []byte, error)
type Client struct{ run runner }

func newClient() Client {
	bin := os.Getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = "herdr"
	}
	return Client{run: func(ctx context.Context, args ...string) ([]byte, []byte, error) {
		cmd := exec.CommandContext(ctx, bin, args...) // nil Env deliberately inherits the socket and caller context.
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err := cmd.Run()
		return out.Bytes(), stderr.Bytes(), err
	}}
}
func (c Client) call(ctx context.Context, args ...string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, stderr, err := c.run(ctx, args...)
	var envelope struct {
		Result *Result                         `json:"result"`
		Error  *struct{ Code, Message string } `json:"error"`
	}
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w: %s", strings.Join(args[:min(2, len(args))], " "), err, clean(string(stderr)))
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		return Result{}, fmt.Errorf("invalid Herdr JSON: %w", err)
	}
	if envelope.Error != nil {
		return Result{}, fmt.Errorf("%s: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if envelope.Result == nil || envelope.Result.Type == "" {
		return Result{}, errors.New("Herdr response is missing result/type")
	}
	return *envelope.Result, nil
}
func validPane(p Pane) bool {
	return p.ID != "" && p.Workspace != "" && p.Tab != "" && p.Terminal != ""
}
func (c Client) pane(ctx context.Context, args ...string) (Pane, error) {
	r, e := c.call(ctx, args...)
	if e != nil {
		return Pane{}, e
	}
	if r.Pane == nil || !validPane(*r.Pane) {
		return Pane{}, errors.New("Herdr response is missing pane identity/location")
	}
	return *r.Pane, nil
}
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
func name(label, id string) string {
	label = clean(strings.TrimSpace(label))
	if label == "" {
		return id
	}
	return label + " (" + id + ")"
}
func samePane(a, b Pane) bool {
	return a.ID == b.ID && a.Tab == b.Tab && a.Workspace == b.Workspace && a.Terminal == b.Terminal
}

type Snapshot struct {
	Workspaces []Workspace
	Tabs       []Tab
	Panes      []Pane
	Layouts    map[string]Layout
}

func (s Snapshot) pane(id string) (Pane, bool) {
	for _, p := range s.Panes {
		if p.ID == id {
			return p, true
		}
	}
	return Pane{}, false
}
func (c Client) load(ctx context.Context) (Snapshot, error) {
	s := Snapshot{Layouts: map[string]Layout{}}
	r, e := c.call(ctx, "workspace", "list")
	if e != nil {
		return s, e
	}
	if r.Type != "workspace_list" || r.Workspaces == nil {
		return s, errors.New("missing workspace list")
	}
	s.Workspaces = r.Workspaces
	r, e = c.call(ctx, "tab", "list")
	if e != nil {
		return s, e
	}
	if r.Type != "tab_list" || r.Tabs == nil {
		return s, errors.New("missing tab list")
	}
	s.Tabs = r.Tabs
	// Explicit workspace scope works on hosts where pane list defaults to the current workspace.
	for _, w := range s.Workspaces {
		if w.ID == "" {
			return s, errors.New("workspace has no ID")
		}
		r, e = c.call(ctx, "pane", "list", "--workspace", w.ID)
		if e != nil {
			return s, e
		}
		if r.Type != "pane_list" || r.Panes == nil {
			return s, errors.New("missing pane list")
		}
		for _, p := range r.Panes {
			if !validPane(p) || p.Workspace != w.ID {
				return s, errors.New("invalid pane location")
			}
			s.Panes = append(s.Panes, p)
		}
	}
	for _, t := range s.Tabs {
		if t.ID == "" || t.Workspace == "" {
			return s, errors.New("tab has no identity/location")
		}
	}

	return s, nil
}

type Destination struct {
	Kind      string
	Workspace string
	Tab       string
	Target    Pane
}

func (s Snapshot) validateLocation(src Pane, d Destination) error {
	p, ok := s.pane(src.ID)
	if !ok || !samePane(p, src) {
		return errors.New("source disappeared or changed location; Refresh and select again")
	}
	if l, ok := s.Layouts[src.Tab]; !ok || l.Zoomed == nil {
		return errors.New("source layout unavailable; Refresh")

	}
	switch d.Kind {
	case "new-workspace":
		return nil
	case "workspace":
		for _, w := range s.Workspaces {
			if w.ID == d.Workspace {
				return nil
			}
		}
	case "tab", "pane":
		if d.Tab == src.Tab {
			return errors.New("current tab unavailable: Herdr does not support same-tab moves")
		}
		p, ok := s.pane(d.Target.ID)
		if !ok || !samePane(p, d.Target) {
			return errors.New("target disappeared or changed location; Refresh and select again")
		}
		l, ok := s.Layouts[d.Tab]
		if !ok || l.Workspace != d.Workspace || l.Zoomed == nil {
			return errors.New("target tab changed; Refresh and select again")
		}
		return nil
	}
	return errors.New("destination unavailable; Refresh and select again")
}

// A zoom error retains the exact destination resolved by the first activation.
// Retrying must never follow a later change in the destination tab's focus.
type zoomMoveError struct{ destination Destination }

func (e *zoomMoveError) Error() string {
	return "Source or destination tab is zoomed. Unzoom before moving."
}
func (s Snapshot) validate(src Pane, d Destination) error {
	if err := s.validateLocation(src, d); err != nil {
		return err
	}
	if *s.Layouts[src.Tab].Zoomed || ((d.Kind == "tab" || d.Kind == "pane") && *s.Layouts[d.Tab].Zoomed) {
		return &zoomMoveError{destination: d}
	}
	return nil
}

func moveArgs(src Pane, d Destination, dir string) ([]string, error) {
	a := []string{"pane", "move", src.ID}
	switch d.Kind {
	case "workspace":
		if d.Workspace == "" {
			return nil, errors.New("missing workspace")
		}
		a = append(a, "--new-tab", "--workspace", d.Workspace)
	case "new-workspace":
		a = append(a, "--new-workspace")
	case "tab", "pane":
		if d.Target.ID == "" || d.Tab == "" || (dir != "right" && dir != "down") {
			return nil, errors.New("invalid split target")
		}
		a = append(a, "--tab", d.Tab, "--target-pane", d.Target.ID, "--split", dir, "--ratio", "0.5")
	default:
		return nil, errors.New("select a destination")
	}
	return append(a, "--focus"), nil
}
func (c Client) move(ctx context.Context, src Pane, d Destination, dir string) error {
	return c.moveWithZoom(ctx, src, d, dir, false)
}

func (c Client) moveWithZoom(ctx context.Context, src Pane, d Destination, dir string, unzoom bool) error {
	fresh := Snapshot{Layouts: map[string]Layout{}}
	current, e := c.pane(ctx, "pane", "get", src.ID)
	if e != nil {
		return e
	}
	fresh.Panes = append(fresh.Panes, current)
	layout, e := c.layout(ctx, current)
	if e != nil {
		return e
	}
	fresh.Layouts[current.Tab] = layout
	if d.Kind == "tab" && d.Target.ID == "" {
		d, e = c.resolveTab(ctx, d)
		if e != nil {
			return e
		}
	}
	if d.Kind == "tab" || d.Kind == "pane" {
		target, err := c.pane(ctx, "pane", "get", d.Target.ID)
		if err != nil {
			return err
		}
		fresh.Panes = append(fresh.Panes, target)
		targetLayout, err := c.layout(ctx, target)
		if err != nil {
			return err
		}
		fresh.Layouts[target.Tab] = targetLayout
	}
	if d.Kind == "workspace" {
		r, err := c.call(ctx, "workspace", "list")
		if err != nil {
			return err
		}
		if r.Type != "workspace_list" || r.Workspaces == nil {
			return errors.New("missing workspace list")
		}
		fresh.Workspaces = r.Workspaces
	}
	if e = fresh.validateLocation(src, d); e != nil {
		return e
	}
	if unzoom {
		panes := []Pane{src}
		if d.Kind == "tab" || d.Kind == "pane" {
			panes = append(panes, d.Target)
		}
		for _, pane := range panes {
			if !*fresh.Layouts[pane.Tab].Zoomed {
				continue
			}
			// Check identity again immediately before changing a tab's zoom state.
			current, err := c.pane(ctx, "pane", "get", pane.ID)
			if err != nil {
				return err
			}
			if !samePane(current, pane) {
				return errors.New("Pane changed location. Refresh and select again.")
			}
			result, err := c.call(ctx, "pane", "zoom", pane.ID, "--off")
			if err != nil {
				return fmt.Errorf("Could not confirm unzoom; inspect the tab before retrying: %w", err)
			}
			if result.Type != "pane_zoom" {
				return errors.New("Unzoom response incomplete. Inspect the tab before retrying.")
			}
			layout, err := c.layout(ctx, pane)
			if err != nil {
				return err
			}
			if *layout.Zoomed {
				return errors.New("Tab is still zoomed. Unzoom it manually, then Refresh.")
			}
		}
		// Revalidate both panes and zoom states after all unzoom operations.
		// This makes one move attempt and never restores zoom or retries it.
		return c.move(ctx, src, d, dir)
	}
	if e = fresh.validate(src, d); e != nil {
		return e
	}
	args, e := moveArgs(src, d, dir)
	if e != nil {
		return e
	}
	r, e := c.call(ctx, args...)
	if e != nil {
		return fmt.Errorf("move outcome uncertain; inspect pane location before trying again: %w", e)
	}
	if r.Type != "pane_move" || r.Move == nil || r.Move.Changed == nil {
		return errors.New("move response incomplete; inspect pane location before trying again")
	}
	if !*r.Move.Changed {
		switch r.Move.Reason {
		case "zoomed_tab":
			return &zoomMoveError{destination: d}
		case "same_tab":
			return errors.New("no move: Herdr does not support same-tab moves")
		}
		return fmt.Errorf("no move: %s", name(r.Move.Reason, "changed: false"))
	}
	if !validPane(r.Move.Pane) || r.Move.Pane.Terminal != src.Terminal || r.Move.Focused != r.Move.Pane.ID {
		return errors.New("move reported changed but pane/focus confirmation is incomplete; inspect Herdr before trying again")
	}
	return nil
}

func (c Client) layout(ctx context.Context, pane Pane) (Layout, error) {
	r, e := c.call(ctx, "pane", "layout", "--pane", pane.ID)
	if e != nil {
		return Layout{}, e
	}
	if r.Type != "pane_layout" || r.Layout == nil || r.Layout.Tab != pane.Tab || r.Layout.Workspace != pane.Workspace || r.Layout.Active == "" || r.Layout.Zoomed == nil {
		return Layout{}, errors.New("tab layout changed; Refresh and choose again")
	}
	return *r.Layout, nil
}

// Capture the active target only on activation. Highlighting never calls Herdr.
func (c Client) resolveTab(ctx context.Context, d Destination) (Destination, error) {
	r, e := c.call(ctx, "pane", "list", "--workspace", d.Workspace)
	if e != nil {
		return d, e
	}
	if r.Type != "pane_list" || r.Panes == nil {
		return d, errors.New("missing pane list")
	}
	for _, p := range r.Panes {
		if p.Tab != d.Tab || p.Workspace != d.Workspace {
			continue
		}
		l, err := c.layout(ctx, p)
		if err != nil {
			return d, err
		}
		target, err := c.pane(ctx, "pane", "get", l.Active)
		if err != nil {
			return d, err
		}
		if target.Tab != d.Tab || target.Workspace != d.Workspace {
			return d, errors.New("target changed location; Refresh and choose again")
		}
		d.Target = target
		return d, nil
	}
	return d, errors.New("destination disappeared; Refresh and choose again")
}
