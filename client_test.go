package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func boolp(v bool) *bool { return &v }
func fixture() Snapshot {
	return Snapshot{
		Workspaces: []Workspace{{"w1", "Work"}, {"w2", "Other"}},
		Tabs:       []Tab{{"w1:t1", "w1", "Source"}, {"w1:t2", "w1", "Tools"}, {"w2:t1", "w2", "Remote"}},
		Panes:      []Pane{{ID: "w1:p1", Workspace: "w1", Tab: "w1:t1", Terminal: "term1", Label: "Agent"}, {ID: "w1:p2", Workspace: "w1", Tab: "w1:t2", Terminal: "term2", Label: "Logs"}, {ID: "w1:p3", Workspace: "w1", Tab: "w1:t2", Terminal: "term3", Label: "Shell"}, {ID: "w2:p1", Workspace: "w2", Tab: "w2:t1", Terminal: "term4", Label: "Remote shell"}},
		Layouts: map[string]Layout{
			"w1:t1": {Workspace: "w1", Tab: "w1:t1", Active: "w1:p1", Zoomed: boolp(false), Area: Rect{Width: 100, Height: 50}, Panes: []PaneRect{{"w1:p1", Rect{Width: 100, Height: 50}}}},
			"w1:t2": {Workspace: "w1", Tab: "w1:t2", Active: "w1:p2", Zoomed: boolp(false), Area: Rect{Width: 100, Height: 50}, Panes: []PaneRect{{"w1:p2", Rect{Width: 50, Height: 50}}, {"w1:p3", Rect{X: 50, Width: 50, Height: 50}}}},
			"w2:t1": {Workspace: "w2", Tab: "w2:t1", Active: "w2:p1", Zoomed: boolp(false), Area: Rect{Width: 100, Height: 50}, Panes: []PaneRect{{"w2:p1", Rect{Width: 100, Height: 50}}}},
		},
	}
}

type fake struct {
	s        Snapshot
	calls    [][]string
	move     *MoveResult
	swap     *SwapResult
	swapFail error
	failure  error
}

func (f *fake) client() Client {
	return Client{run: func(ctx context.Context, a ...string) ([]byte, []byte, error) {
		f.calls = append(f.calls, append([]string{}, a...))
		if e := ctx.Err(); e != nil {
			return nil, nil, e
		}
		if f.failure != nil {
			return nil, []byte("host unavailable"), f.failure
		}
		r := Result{}
		switch strings.Join(a[:2], " ") {
		case "workspace list":
			r.Type = "workspace_list"
			r.Workspaces = f.s.Workspaces
		case "tab list":
			r.Type = "tab_list"
			r.Tabs = f.s.Tabs
		case "pane list":
			r.Type = "pane_list"
			r.Panes = []Pane{}
			for _, p := range f.s.Panes {
				if p.Workspace == a[3] {
					r.Panes = append(r.Panes, p)
				}
			}
		case "pane zoom":
			p, _ := f.s.pane(a[2])
			l := f.s.Layouts[p.Tab]
			l.Zoomed = boolp(false)
			f.s.Layouts[p.Tab] = l
			r.Type = "pane_zoom"
		case "pane layout":
			p, _ := f.s.pane(a[3])
			l := f.s.Layouts[p.Tab]
			r.Type = "pane_layout"
			r.Layout = &l
		case "pane get", "pane current":
			p := f.s.Panes[0]
			if a[1] == "get" {
				p, _ = f.s.pane(a[2])
			}
			r.Type = "pane_current"
			r.Pane = &p
		case "pane move":
			r.Type = "pane_move"
			r.Move = f.move
		case "pane swap":
			if f.swapFail != nil {
				return nil, []byte("swap refused"), f.swapFail
			}
			r.Type = "pane_swap"
			r.Swap = f.swap
			if r.Swap == nil {
				r.Swap = &SwapResult{Changed: boolp(true), Focused: a[3]}
			}
		case "plugin pane":
			r.Type = "ok"
		default:
			return nil, nil, errors.New("unexpected command")
		}
		b, _ := json.Marshal(struct {
			Result Result `json:"result"`
		}{r})
		return b, nil, nil
	}}
}
func (f *fake) moves() [][]string { return f.commands("move") }
func (f *fake) swaps() [][]string { return f.commands("swap") }
func (f *fake) commands(verb string) [][]string {
	out := [][]string{}
	for _, a := range f.calls {
		if len(a) > 1 && a[0] == "pane" && a[1] == verb {
			out = append(out, a)
		}
	}
	return out
}
func successfulMove(src Pane) *MoveResult {
	src.ID = "w2:p9"
	src.Workspace = "w2"
	src.Tab = "w2:t1"
	return &MoveResult{Changed: boolp(true), Pane: src, Focused: src.ID}
}
func TestDestinationsAndExactTargets(t *testing.T) {
	s := fixture()
	src := s.Panes[0]
	for _, tt := range []struct {
		kind, id string
		want     []string
	}{
		{"workspace", "w2", []string{"--new-tab", "--workspace", "w2"}},
		{"new-workspace", "", []string{"--new-workspace"}},
		{"tab", "w1:t2", []string{"--tab", "w1:t2", "--target-pane", "w1:p2", "--split", "down", "--ratio", "0.5"}},
		{"pane", "w1:p3", []string{"--tab", "w1:t2", "--target-pane", "w1:p3", "--split", "down", "--ratio", "0.5"}},
	} {
		t.Run(tt.kind, func(t *testing.T) {
			d, e := s.destination(tt.kind, tt.id)
			if e != nil {
				t.Fatal(e)
			}
			a, e := moveArgs(src, d, Below)
			if e != nil {
				t.Fatal(e)
			}
			want := append(append([]string{"pane", "move", src.ID}, tt.want...), "--focus")
			if !reflect.DeepEqual(a, want) {
				t.Fatalf("got %v, want %v", a, want)
			}
		})
	}
}
func TestTabCaptureSurvivesFocusChange(t *testing.T) {
	s := fixture()
	src := s.Panes[0]
	d, _ := s.destination("tab", "w1:t2")
	l := s.Layouts[d.Tab]
	l.Active = "w1:p3"
	s.Layouts[d.Tab] = l
	f := &fake{s: s, move: successfulMove(src)}
	if e := f.client().move(context.Background(), src, d, Right); e != nil {
		t.Fatal(e)
	}
	a := strings.Join(f.moves()[0], " ")
	if !strings.Contains(a, "--target-pane w1:p2") {
		t.Fatal(a)
	}
}
func TestRevalidationRefusesChangedState(t *testing.T) {
	for _, scenario := range []string{"source gone", "source moved", "terminal replaced", "target gone", "target moved", "workspace gone", "source zoom", "target zoom", "same tab"} {
		t.Run(scenario, func(t *testing.T) {
			s := fixture()
			src := s.Panes[0]
			d, _ := s.destination("pane", "w1:p2")
			switch scenario {
			case "source gone":
				s.Panes = s.Panes[1:]
			case "source moved":
				s.Panes[0].Tab = "w1:t2"
			case "terminal replaced":
				s.Panes[1].Terminal = "replacement"
			case "target gone":
				s.Panes = append(s.Panes[:1], s.Panes[2:]...)
			case "target moved":
				s.Panes[1].Tab = "w2:t1"
			case "workspace gone":
				d, _ = s.destination("workspace", "w2")
				s.Workspaces = s.Workspaces[:1]
			case "source zoom":
				l := s.Layouts[src.Tab]
				l.Zoomed = boolp(true)
				s.Layouts[src.Tab] = l
			case "target zoom":
				l := s.Layouts[d.Tab]
				l.Zoomed = boolp(true)
				s.Layouts[d.Tab] = l
			case "same tab":
				d, _ = s.destination("pane", src.ID)
			}
			f := &fake{s: s, move: successfulMove(src)}
			if e := f.client().move(context.Background(), src, d, Right); e == nil {
				t.Fatal("move should fail")
			}
			if len(f.moves()) != 0 {
				t.Fatal("mutated stale state")
			}
		})
	}
}
func TestMoveResponseValidation(t *testing.T) {
	for _, scenario := range []string{"success", "missing result", "missing changed", "zoomed_tab", "same_tab", "other no-op", "missing pane", "wrong focus", "wrong terminal"} {
		t.Run(scenario, func(t *testing.T) {
			s := fixture()
			src := s.Panes[0]
			m := successfulMove(src)
			switch scenario {
			case "missing result":
				m = nil
			case "missing changed":
				m.Changed = nil
			case "zoomed_tab", "same_tab", "other no-op":
				m.Changed = boolp(false)
				m.Reason = scenario
			case "missing pane":
				m.Pane = Pane{}
			case "wrong focus":
				m.Focused = "w1:p3"
			case "wrong terminal":
				m.Pane.Terminal = "other"
			}
			f := &fake{s: s, move: m}
			d, _ := s.destination("new-workspace", "")
			e := f.client().move(context.Background(), src, d, Right)
			if (e == nil) != (scenario == "success") {
				t.Fatalf("unexpected error: %v", e)
			}
			if len(f.moves()) != 1 {
				t.Fatal("move retried")
			}
		})
	}
}
func TestMalformedAndFailedResponses(t *testing.T) {
	for _, out := range []string{"not json", "{}", `{"result":{}}`, `{"error":{"code":"bad","message":"no"}}`, `{"result":{"type":"workspace_list","workspaces":null}}`} {
		c := Client{run: func(context.Context, ...string) ([]byte, []byte, error) { return []byte(out), nil, nil }}
		if _, e := c.load(context.Background()); e == nil {
			t.Fatalf("accepted %s", out)
		}
	}
	f := &fake{failure: errors.New("exit 1")}
	if _, e := f.client().call(context.Background(), "pane", "get", "p"); e == nil || !strings.Contains(e.Error(), "host unavailable") {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f = &fake{s: fixture()}
	if _, e := f.client().load(ctx); e == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestLabels(t *testing.T) {
	if got := name("same\n\x1bname", "w1"); got != "same  name (w1)" {
		t.Fatal(got)
	}
	if name("", "w1") != "w1" {
		t.Fatal("empty name")
	}
}

func TestActionSourceCaptureAndDirectMoves(t *testing.T) {
	for _, action := range []string{"move", "new-tab", "new-workspace"} {
		t.Run(action, func(t *testing.T) {
			s := fixture()
			f := &fake{s: s, move: successfulMove(s.Panes[0])}
			if e := performAction(context.Background(), f.client(), action); e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(f.calls[0], []string{"pane", "current", "--current"}) {
				t.Fatal("source not captured first")
			}
			if action == "move" {
				want := []string{"plugin", "pane", "open", "--plugin", "herdr-pane-mover", "--entrypoint", "picker", "--env", "PANE_MOVER_SOURCE=w1:p1", "--focus"}
				if !reflect.DeepEqual(f.calls[1], want) || len(f.calls) != 2 {
					t.Fatal(f.calls)
				}
			} else {
				if len(f.moves()) != 1 {
					t.Fatal("direct action did not submit once")
				}
				args := strings.Join(f.moves()[0], " ")
				flag := "--new-workspace"
				if action == "new-tab" {
					flag = "--new-tab --workspace w1"
				}
				if !strings.Contains(args, flag) {
					t.Fatal(args)
				}
			}
		})
	}
}

func TestExecutableArgArraysAndSocketEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake herdr")
	// The filename has spaces, and shell syntax is passed as a literal argument.
	script := `#!/bin/sh
[ "$HERDR_SOCKET_PATH" = '/tmp/pane mover socket' ] || exit 3
[ "$3" = 'literal $(touch should-not-exist); [red]' ] || exit 4
printf '%s' '{"result":{"type":"ok"}}'
`
	if e := os.WriteFile(path, []byte(script), 0755); e != nil {
		t.Fatal(e)
	}
	t.Setenv("HERDR_BIN_PATH", path)
	t.Setenv("HERDR_SOCKET_PATH", "/tmp/pane mover socket")
	if _, e := newClient().call(context.Background(), "pane", "get", "literal $(touch should-not-exist); [red]"); e != nil {
		t.Fatal(e)
	}
}

// destination builds explicit destinations for client fixtures. Runtime tab
// destinations are resolved at activation by Client.resolveTab.
func (s Snapshot) destination(kind, id string) (Destination, error) {
	switch kind {
	case "new-workspace":
		return Destination{Kind: kind}, nil
	case "workspace":
		for _, w := range s.Workspaces {
			if w.ID == id {
				return Destination{Kind: kind, Workspace: id}, nil
			}
		}
	case "tab":
		for _, t := range s.Tabs {
			if t.ID == id {
				p, ok := s.pane(s.Layouts[id].Active)
				if ok {
					return Destination{Kind: kind, Workspace: t.Workspace, Tab: id, Target: p}, nil
				}
			}
		}
	case "pane":
		if p, ok := s.pane(id); ok {
			return Destination{Kind: kind, Workspace: p.Workspace, Tab: p.Tab, Target: p}, nil
		}
	}
	return Destination{}, errors.New("destination disappeared; Refresh and select again")
}

func TestUnzoomMove(t *testing.T) {
	for _, tabs := range [][]string{{"w1:t1"}, {"w1:t2"}, {"w1:t1", "w1:t2"}} {
		t.Run(strings.Join(tabs, "+"), func(t *testing.T) {
			s := fixture()
			src := s.Panes[0]
			for _, tab := range tabs {
				l := s.Layouts[tab]
				l.Zoomed = boolp(true)
				s.Layouts[tab] = l
			}
			f := &fake{s: s, move: successfulMove(src)}
			d := Destination{Kind: "tab", Workspace: "w1", Tab: "w1:t2"}
			err := f.client().move(context.Background(), src, d, Right)
			var zoom *zoomMoveError
			if !errors.As(err, &zoom) || len(f.moves()) != 0 {
				t.Fatalf("expected zoom recovery, got %v", err)
			}
			for _, a := range f.calls {
				if a[1] == "zoom" {
					t.Fatal("unzoom without consent")
				}
			}
			l := f.s.Layouts["w1:t2"]
			l.Active = "w1:p3"
			f.s.Layouts["w1:t2"] = l
			if err := f.client().moveWithZoom(context.Background(), src, zoom.destination, Right, true); err != nil {
				t.Fatal(err)
			}
			if len(f.moves()) != 1 || !strings.Contains(strings.Join(f.moves()[0], " "), "--target-pane w1:p2") {
				t.Fatal(f.moves())
			}
			for _, tab := range tabs {
				if *f.s.Layouts[tab].Zoomed {
					t.Fatal("still zoomed")
				}
			}
		})
	}
}
func TestUnzoomRecoveryRefusesChangesAndFailures(t *testing.T) {
	for _, scenario := range []string{"source moved", "target moved", "zoom fails", "rezoom", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			s := fixture()
			src := s.Panes[0]
			d, _ := s.destination("pane", "w1:p2")
			l := s.Layouts[src.Tab]
			l.Zoomed = boolp(true)
			s.Layouts[src.Tab] = l
			f := &fake{s: s, move: successfulMove(src)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "source moved" {
				f.s.Panes[0].Tab = "other"
			}
			if scenario == "target moved" {
				f.s.Panes[1].Terminal = "replacement"
			}
			if scenario == "cancel" {
				cancel()
			}
			base := f.client()
			c := Client{run: func(ctx context.Context, a ...string) ([]byte, []byte, error) {
				if a[1] == "zoom" && scenario == "zoom fails" {
					return nil, nil, errors.New("failed")
				}
				out, stderr, err := base.run(ctx, a...)
				if a[1] == "zoom" && scenario == "rezoom" {
					l := f.s.Layouts[src.Tab]
					l.Zoomed = boolp(true)
					f.s.Layouts[src.Tab] = l
				}
				return out, stderr, err
			}}
			if err := c.moveWithZoom(ctx, src, d, Right, true); err == nil {
				t.Fatal("expected refusal")
			}
			if len(f.moves()) != 0 {
				t.Fatal("unexpected move")
			}
		})
	}
}

func TestPlacementSplitsAndSwaps(t *testing.T) {
	for _, tt := range []struct {
		placement Placement
		split     string
		swap      bool
	}{{Right, "right", false}, {Below, "down", false}, {Left, "right", true}, {Above, "down", true}} {
		t.Run(string(tt.placement), func(t *testing.T) {
			s := fixture()
			src := s.Panes[0]
			d, _ := s.destination("pane", "w1:p3")
			f := &fake{s: s, move: successfulMove(src)}
			if e := f.client().move(context.Background(), src, d, tt.placement); e != nil {
				t.Fatal(e)
			}
			if len(f.moves()) != 1 || !strings.Contains(strings.Join(f.moves()[0], " "), "--target-pane w1:p3 --split "+tt.split+" ") {
				t.Fatal(f.moves())
			}
			// The swap uses the moved pane's new ID and the exact target.
			want := [][]string{}
			if tt.swap {
				want = append(want, []string{"pane", "swap", "--source-pane", "w2:p9", "--target-pane", "w1:p3"})
			}
			if !reflect.DeepEqual(f.swaps(), want) {
				t.Fatal(f.swaps())
			}
		})
	}
}
func TestPlacementIgnoredForNewTabsAndWorkspaces(t *testing.T) {
	for _, kind := range []string{"workspace", "new-workspace"} {
		s := fixture()
		src := s.Panes[0]
		d, _ := s.destination(kind, "w2")
		f := &fake{s: s, move: successfulMove(src)}
		if e := f.client().move(context.Background(), src, d, Left); e != nil || len(f.swaps()) != 0 {
			t.Fatal(kind, e, f.swaps())
		}
	}
}
func TestFailedPlacementSwapReportsLandingSide(t *testing.T) {
	for _, scenario := range []string{"refused", "no change", "wrong focus"} {
		t.Run(scenario, func(t *testing.T) {
			s := fixture()
			src := s.Panes[0]
			d, _ := s.destination("pane", "w1:p3")
			f := &fake{s: s, move: successfulMove(src)}
			switch scenario {
			case "refused":
				f.swapFail = errors.New("exit 1")
			case "no change":
				f.swap = &SwapResult{Changed: boolp(false), Reason: "same_pane"}
			case "wrong focus":
				f.swap = &SwapResult{Changed: boolp(true), Focused: "w1:p3"}
			}
			err := f.client().move(context.Background(), src, d, Above)
			var partial *partialPlacementError
			if !errors.As(err, &partial) || !strings.Contains(err.Error(), "landed below the target instead of above it") {
				t.Fatal(err)
			}
			if len(f.moves()) != 1 || len(f.swaps()) != 1 {
				t.Fatal("move or swap retried")
			}
		})
	}
}
func sameTabFixture() Snapshot {
	s := fixture()
	s.Panes = append(s.Panes, Pane{ID: "w1:p4", Workspace: "w1", Tab: "w1:t1", Terminal: "term5", Label: "Editor"})
	l := s.Layouts["w1:t1"]
	l.Panes = []PaneRect{{"w1:p1", Rect{Width: 50, Height: 50}}, {"w1:p4", Rect{X: 50, Width: 50, Height: 50}}}
	s.Layouts["w1:t1"] = l
	return s
}
func TestSameTabSwap(t *testing.T) {
	for _, scenario := range []string{"swap", "zoomed", "other tab", "self", "target gone"} {
		t.Run(scenario, func(t *testing.T) {
			s := sameTabFixture()
			src := s.Panes[0]
			d, _ := s.destination("pane", "w1:p4")
			d.Kind = "swap"
			switch scenario {
			case "zoomed":
				l := s.Layouts[src.Tab]
				l.Zoomed = boolp(true)
				s.Layouts[src.Tab] = l
			case "other tab":
				d, _ = s.destination("pane", "w1:p3")
				d.Kind = "swap"
			case "self":
				d, _ = s.destination("pane", src.ID)
				d.Kind = "swap"
			case "target gone":
				s.Panes = s.Panes[:len(s.Panes)-1]
			}
			f := &fake{s: s}
			err := f.client().move(context.Background(), src, d, Left)
			ok := scenario == "swap" || scenario == "zoomed"
			if (err == nil) != ok {
				t.Fatal(err)
			}
			want := [][]string{}
			if ok {
				want = append(want, []string{"pane", "swap", "--source-pane", "w1:p1", "--target-pane", "w1:p4"})
			}
			if !reflect.DeepEqual(f.swaps(), want) || len(f.moves()) != 0 {
				t.Fatal(f.calls)
			}
			for _, a := range f.calls {
				if a[1] == "zoom" {
					t.Fatal("swap unzoomed a tab")
				}
			}
		})
	}
}
