package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func setupPicker(t *testing.T) (*picker, *fake) {
	t.Helper()
	s := fixture()
	f := &fake{s: fixture(), move: successfulMove(s.Panes[0])}
	p := newPicker(f.client(), s.Panes[0].ID)
	p.launch = func(fn func()) { fn() }
	p.dispatch = func(fn func()) { fn() }
	t.Cleanup(p.cancel)
	p.applySnapshot(s, s.Panes[0])
	return p, f
}
func selectChoice(t *testing.T, p *picker, kind, id string) int {
	t.Helper()
	for i, c := range p.rows {
		if c.dest.Kind == kind && (id == "" || c.dest.Tab == id || c.dest.Target.ID == id || c.dest.Workspace == id) {
			p.list.Select(i, 0)
			return i
		}
	}
	t.Fatalf("missing %s %s", kind, id)
	return -1
}
func key(p *picker, k tcell.Key) {
	e := p.app.GetInputCapture()(tcell.NewEventKey(k, 0, 0))
	if e != nil && p.app.GetFocus() != nil {
		p.app.GetFocus().InputHandler()(e, func(v tview.Primitive) { p.app.SetFocus(v) })
	}
}
func click(p *picker, x, y int) {
	p.root.MouseHandler()(tview.MouseLeftClick, tcell.NewEventMouse(x, y, tcell.Button1, 0), func(v tview.Primitive) { p.app.SetFocus(v) })
}
func TestTreeExpansionAndCurrentTab(t *testing.T) {
	p, f := setupPicker(t)
	if !p.list.HasFocus() {
		t.Fatal("tree not focused")
	}
	if len(p.rows) != 7 {
		t.Fatal(p.rows)
	}
	r := selectChoice(t, p, "tab", "w1:t1")
	if !p.rows[r].disabled {
		t.Fatal("current tab enabled")
	}
	key(p, tcell.KeyEnter)
	if len(f.calls) != 0 {
		t.Fatal("current tab moved")
	}
	key(p, tcell.KeyRight)
	r = selectChoice(t, p, "pane", "w1:p1")
	if !p.rows[r].disabled {
		t.Fatal("current pane enabled")
	}
	key(p, tcell.KeyEnter)
	if len(f.calls) != 0 {
		t.Fatal("current pane moved")
	}
	selectChoice(t, p, "tab", "w1:t2")
	key(p, tcell.KeyRight)
	selectChoice(t, p, "pane", "w1:p3")
	key(p, tcell.KeyLeft)
	c, _ := p.selected()
	if c.dest.Tab != "w1:t2" || c.dest.Kind != "tab" {
		t.Fatal("Left should reach parent")
	}
	key(p, tcell.KeyLeft)
	if p.expanded["tab:w1:t2"] {
		t.Fatal("tab did not collapse")
	}
	if len(f.calls) != 0 {
		t.Fatal("expansion contacted Herdr")
	}
}
func TestTabActivationCapturesActivePane(t *testing.T) {
	p, f := setupPicker(t)
	l := f.s.Layouts["w1:t2"]
	l.Active = "w1:p3"
	f.s.Layouts["w1:t2"] = l
	selectChoice(t, p, "tab", "w1:t2")
	key(p, tcell.KeyEnter)
	key(p, tcell.KeyEnter)
	if len(f.moves()) != 1 || !strings.Contains(strings.Join(f.moves()[0], " "), "--target-pane w1:p3") {
		t.Fatal(f.calls)
	}
}
func TestMouseTabArrowThenExactPane(t *testing.T) {
	p, f := setupPicker(t)
	r := selectChoice(t, p, "tab", "w1:t2")
	drawPicker(t, p, 72, 18)
	x, y, _, _ := p.list.GetInnerRect()
	click(p, x+4, y+r)
	if len(f.calls) != 0 || !p.expanded["tab:w1:t2"] {
		t.Fatal("tab arrow should expand without moving")
	}
	r = selectChoice(t, p, "pane", "w1:p3")
	drawPicker(t, p, 72, 18)
	x, y, _, _ = p.list.GetInnerRect()
	click(p, x+9, y+r)
	if len(f.moves()) != 1 || !strings.Contains(strings.Join(f.moves()[0], " "), "--target-pane w1:p3") {
		t.Fatal("pane name did not move exact target")
	}
}
func TestMouseWorkspaceNameAndTabName(t *testing.T) {
	p, f := setupPicker(t)
	r := selectChoice(t, p, "branch", "w2")
	drawPicker(t, p, 72, 18)
	x, y, _, _ := p.list.GetInnerRect()
	click(p, x+4, y+r)
	if !p.expanded["workspace:w2"] || len(f.calls) != 0 {
		t.Fatal("workspace name should expand")
	}
	r = selectChoice(t, p, "tab", "w2:t1")
	drawPicker(t, p, 72, 18)
	x, y, _, _ = p.list.GetInnerRect()
	click(p, x+7, y+r)
	if len(f.moves()) != 1 {
		t.Fatal("tab name should move immediately")
	}
}
func TestPlacementAndCreation(t *testing.T) {
	for _, kind := range []string{"new-tab", "new-workspace"} {
		t.Run(kind, func(t *testing.T) {
			p, f := setupPicker(t)
			drawPicker(t, p, 72, 18)
			x, y, _, _ := p.below.GetRect()
			click(p, x+3, y)
			if p.direction != "down" {
				t.Fatal("Below click did not select")
			}
			r := selectChoice(t, p, kind, "")
			drawPicker(t, p, 72, 18)
			x, y, _, _ = p.list.GetInnerRect()
			offset, _ := p.list.GetOffset()
			click(p, x+4, y+r-offset)
			if kind == "new-tab" {
				if p.mode != "workspaces" || p.rows[0].dest.Workspace != "w1" {
					t.Fatal("workspace picker")
				}
				selectChoice(t, p, "workspace", "w2")
				key(p, tcell.KeyEnter)
			}
			if len(f.moves()) != 1 {
				t.Fatal("creation didn't move")
			}
		})
	}
}
func TestWorkspaceBackRestoresTree(t *testing.T) {
	p, _ := setupPicker(t)
	r := selectChoice(t, p, "tab", "w1:t2")
	p.expand(r)
	p.chooseWorkspace()
	key(p, tcell.KeyEscape)
	selected, _ := p.list.GetSelection()
	if p.mode != "tree" || !p.expanded["tab:w1:t2"] || selected != r {
		t.Fatal("Back lost expansion/selection")
	}
}
func TestErrorRefreshAndRepeatedSubmission(t *testing.T) {
	p, f := setupPicker(t)
	f.move = &MoveResult{Changed: boolp(false), Reason: "zoomed_tab"}
	var work func()
	p.launch = func(fn func()) { work = fn }
	selectChoice(t, p, "tab", "w1:t2")
	p.activate()
	p.activate()
	if !p.pending || work == nil {
		t.Fatal("pending guard")
	}
	work()
	if !p.failed || len(p.rows) != 0 || len(f.moves()) != 1 {
		t.Fatal("error recovery")
	}
	p.activate()
	p.launch = func(fn func()) { fn() }
	p.reload()
	if p.pending || p.failed || !p.list.HasFocus() || len(f.moves()) != 1 {
		t.Fatal("refresh should restore tree without moving")
	}
}
func TestDuplicateLabels(t *testing.T) {
	s := fixture()
	s.Tabs = append(s.Tabs, Tab{"w1:t3", "w1", "Tools"})
	rows := treeChoices(s, s.Panes[0], "tree", map[string]bool{"workspace:w1": true})
	if rows[2].label != "Tab Tools (w1:t2)" || rows[3].label != "Tab Tools (w1:t3)" {
		t.Fatal(rows)
	}
}
func TestThemeAndSeparation(t *testing.T) {
	for _, size := range [][2]int{{72, 18}, {40, 12}} {
		p, _ := setupPicker(t)
		p.expanded["tab:w1:t2"] = true
		p.render(0)
		p.list.Select(len(p.rows)-1, 0)
		s := drawPicker(t, p, size[0], size[1])
		text := screenText(s)
		for _, label := range []string{"Move pane", "Placement", "Right", "Below", "+ New tab", "+ New workspace", "Cancel", "─"} {
			if !strings.Contains(text, label) {
				t.Fatalf("missing %s\n%s", label, text)
			}
		}
		if strings.Contains(text, "Ctrl") || strings.Contains(text, "Search") || strings.Contains(text, "Choose pane") {
			t.Fatal("extra controls returned")
		}
		cells, _, _ := s.GetContents()
		for i, c := range cells {
			fg, bg, attr := c.Style.Decompose()
			if bg != tcell.ColorDefault || fg != tcell.ColorDefault || attr&tcell.AttrReverse != 0 {
				t.Fatalf("cell %d overrides terminal theme: fg=%v bg=%v attr=%v", i, fg, bg, attr)
			}
		}
		if p.rows[len(p.rows)-2].dest.Kind != "new-tab" || p.rows[len(p.rows)-1].dest.Kind != "new-workspace" {
			t.Fatal("creation actions must follow workspace destinations")
		}
		if dir := os.Getenv("PANE_MOVER_RENDER_DIR"); dir != "" {
			if e := os.MkdirAll(dir, 0755); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(filepath.Join(dir, fmt.Sprintf("minimal-tree-%d.txt", size[0])), []byte(text), 0644); e != nil {
				t.Fatal(e)
			}
		}
	}
}
func TestMouseControlsOutsideLongTree(t *testing.T) {
	p, f := setupPicker(t)
	for i := 0; i < 30; i++ {
		p.snap.Tabs = append(p.snap.Tabs, Tab{ID: fmt.Sprint(i), Workspace: "w1", Label: "Extra"})
	}
	p.render(0)
	drawPicker(t, p, 40, 12)
	x, y, _, _ := p.below.GetRect()
	click(p, x+3, y)
	if len(f.calls) != 0 || p.direction != "down" {
		t.Fatal("toolbar click activated offscreen row")
	}
	p.busy(true)
	x, y, _, _ = p.cancelButton.GetRect()
	click(p, x+3, y)
	if p.ctx.Err() == nil {
		t.Fatal("pending Cancel intercepted")
	}
}
func drawPicker(t *testing.T, p *picker, w, h int) tcell.SimulationScreen {
	t.Helper()
	s := tcell.NewSimulationScreen("UTF-8")
	if e := s.Init(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Fini)
	s.SetSize(w, h)
	p.root.SetRect(0, 0, w, h)
	p.app.GetBeforeDrawFunc()(s)
	p.root.Draw(s)
	s.Show()
	return s
}
func screenText(s tcell.SimulationScreen) string {
	cells, w, h := s.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := cells[y*w+x].Runes
			if len(r) == 0 {
				b.WriteByte(' ')
			} else {
				b.WriteRune(r[0])
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
func TestApplicationLoopMovesOnceAndCloses(t *testing.T) {
	s := fixture()
	f := &fake{s: s, move: successfulMove(s.Panes[0])}
	p := newPicker(f.client(), s.Panes[0].ID)
	defer p.cancel()
	p.applySnapshot(s, s.Panes[0])
	screen := tcell.NewSimulationScreen("UTF-8")
	if e := screen.Init(); e != nil {
		t.Fatal(e)
	}
	screen.SetSize(100, 30)
	p.app.SetScreen(screen)
	done := make(chan error, 1)
	go func() { done <- p.app.Run() }()
	p.app.QueueUpdateDraw(func() { p.submit(Destination{Kind: "new-workspace"}); p.submit(Destination{Kind: "new-workspace"}) })
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		p.stop()
		t.Fatal("successful move did not close application")
	}
	if len(f.moves()) != 1 {
		t.Fatal("repeated submission moved more than once")
	}
}
func TestApplicationCancelInterruptsRead(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})
	c := Client{run: func(ctx context.Context, _ ...string) ([]byte, []byte, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return nil, nil, ctx.Err()
	}}
	p := newPicker(c, "w1:p1")
	defer p.cancel()
	screen := tcell.NewSimulationScreen("UTF-8")
	if e := screen.Init(); e != nil {
		t.Fatal(e)
	}
	screen.SetSize(100, 30)
	p.app.SetScreen(screen)
	p.reload()
	done := make(chan error, 1)
	go func() { done <- p.app.Run() }()
	<-started
	p.app.QueueEvent(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		p.stop()
		t.Fatal("Escape did not close during read")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("read context was not cancelled")
	}
}

func TestShortTreeExpansionRevealsChild(t *testing.T) {
	p, _ := setupPicker(t)
	r := selectChoice(t, p, "tab", "w1:t2")
	drawPicker(t, p, 40, 12)
	p.expand(r)
	screen := drawPicker(t, p, 40, 12)
	if !strings.Contains(screenText(screen), "Logs") {
		t.Fatal("expansion left every child below viewport")
	}
}

func TestKeyboardSelectionIsVisibleAndControlsDoNotTrapArrows(t *testing.T) {
	p, f := setupPicker(t)
	screen := drawPicker(t, p, 72, 18)
	if !strings.Contains(screenText(screen), "> ▾ Work") {
		t.Fatal("initial workspace has no visible selection")
	}
	p.app.SetFocus(p.below)
	key(p, tcell.KeyDown)
	if !p.list.HasFocus() {
		t.Fatal("Down stayed trapped in placement controls")
	}
	screen = drawPicker(t, p, 72, 18)
	text := screenText(screen)
	if !strings.Contains(text, ">   ▸ Tab Source (current)") || strings.Contains(text, "> ▾ Work") {
		t.Fatal("selection marker did not move with Down")
	}
	key(p, tcell.KeyUp)
	key(p, tcell.KeyRight)
	c, _ := p.selected()
	if c.dest.Tab != "w1:t1" {
		t.Fatal("Right did not enter expanded workspace")
	}
	key(p, tcell.KeyDown)
	key(p, tcell.KeyRight)
	key(p, tcell.KeyRight)
	c, _ = p.selected()
	if c.dest.Target.ID != "w1:p2" {
		t.Fatal("Right did not enter expanded tab")
	}
	screen = drawPicker(t, p, 72, 18)
	text = screenText(screen)
	if !strings.Contains(text, ">       Logs") {
		t.Fatal("pane selection missing")
	}
	cells, _, _ := screen.GetContents()
	bold := false
	for _, cell := range cells {
		fg, bg, attr := cell.Style.Decompose()
		if fg != tcell.ColorDefault || bg != tcell.ColorDefault || attr&tcell.AttrReverse != 0 {
			t.Fatal("selection overrides theme")
		}
		if attr&tcell.AttrBold != 0 {
			bold = true
		}
	}
	if !bold {
		t.Fatal("selected row should be bold")
	}
	if len(f.calls) != 0 {
		t.Fatal("navigation made CLI calls")
	}
}

func TestApplicationArrowKeysNavigateAndHighlight(t *testing.T) {
	s := fixture()
	f := &fake{s: s}
	p := newPicker(f.client(), s.Panes[0].ID)
	defer p.cancel()
	p.applySnapshot(s, s.Panes[0])
	screen := tcell.NewSimulationScreen("UTF-8")
	if e := screen.Init(); e != nil {
		t.Fatal(e)
	}
	screen.SetSize(72, 18)
	p.app.SetScreen(screen)
	observed := make(chan string, 1)
	p.app.SetAfterDrawFunc(func(tcell.Screen) {
		c, ok := p.selected()
		if ok && c.dest.Target.ID == "w1:p2" {
			select {
			case observed <- currentFrameText(screen):
			default:
			}
		}
	})
	done := make(chan error, 1)
	go func() { done <- p.app.Run() }()
	for _, k := range []tcell.Key{tcell.KeyDown, tcell.KeyDown, tcell.KeyRight, tcell.KeyRight} {
		p.app.QueueEvent(tcell.NewEventKey(k, 0, 0))
	}
	select {
	case text := <-observed:
		if !strings.Contains(text, ">       Logs") {
			p.stop()
			t.Fatal("event-loop navigation did not draw selection")
		}
	case <-time.After(5 * time.Second):
		p.stop()
		t.Fatal("keyboard events did not reach the pane")
	}
	p.app.QueueEvent(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		p.stop()
		t.Fatal("did not exit")
	}
	if len(f.calls) != 0 {
		t.Fatal("navigation moved a pane")
	}
}

// AfterDraw runs before Show; read the current logical frame rather than the
// simulation screen's previously flushed contents.
func currentFrameText(s tcell.Screen) string {
	var text strings.Builder
	w, h := s.Size()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, _, _, _ := s.GetContent(x, y)
			if r == 0 {
				r = ' '
			}
			text.WriteRune(r)
		}
		text.WriteByte('\n')
	}
	return text.String()
}

func TestUnzoomRecoveryControls(t *testing.T) {
	for _, size := range [][2]int{{72, 18}, {40, 12}} {
		p, f := setupPicker(t)
		l := f.s.Layouts["w1:t2"]
		l.Zoomed = boolp(true)
		f.s.Layouts["w1:t2"] = l
		selectChoice(t, p, "tab", "w1:t2")
		p.activate()
		if p.retry == nil || !p.unzoom.HasFocus() || len(f.moves()) != 0 {
			t.Fatal("missing explicit recovery")
		}
		screen := drawPicker(t, p, size[0], size[1])
		rendered := screenText(screen)
		for _, label := range []string{"Unzoom & move", "Refresh", "Cancel"} {
			if !strings.Contains(rendered, label) {
				t.Fatalf("%v missing %s: %s", size, label, rendered)
			}
		}
		t.Logf("%dx%d recovery:\n%s", size[0], size[1], rendered)
		var work func()
		p.launch = func(fn func()) { work = fn }
		x, y, _, _ := p.unzoom.GetRect()
		click(p, x+2, y)
		p.retryUnzoom()
		if work == nil || !p.pending {
			t.Fatal("did not submit")
		}
		work()
		if len(f.moves()) != 1 {
			t.Fatal("duplicate or missing move")
		}
	}
}
func TestRefreshDiscardsUnzoomRecovery(t *testing.T) {
	p, f := setupPicker(t)
	l := f.s.Layouts["w1:t2"]
	l.Zoomed = boolp(true)
	f.s.Layouts["w1:t2"] = l
	selectChoice(t, p, "tab", "w1:t2")
	p.activate()
	p.reload()
	p.retryUnzoom()
	if p.retry != nil || len(f.moves()) != 0 {
		t.Fatal("stale recovery")
	}
	p.showError(errors.New("Destination changed. Refresh and select again."))
	if strings.Count(p.status.GetText(false), "Refresh") != 1 {
		t.Fatal(p.status.GetText(false))
	}
}

func TestUnzoomRecoveryFailureRequiresFreshSelection(t *testing.T) {
	p, f := setupPicker(t)
	l := f.s.Layouts["w1:t2"]
	l.Zoomed = boolp(true)
	f.s.Layouts["w1:t2"] = l
	selectChoice(t, p, "tab", "w1:t2")
	p.activate()
	f.move = &MoveResult{Changed: boolp(false), Reason: "zoomed_tab"}
	key(p, tcell.KeyEnter)
	if !p.failed || p.retry != nil || len(f.moves()) != 1 {
		t.Fatal("failed recovery allowed retry")
	}
	if !strings.Contains(p.status.GetText(false), "may remain unzoomed") {
		t.Fatal(p.status.GetText(false))
	}
	p.retryUnzoom()
	if len(f.moves()) != 1 {
		t.Fatal("automatic retry")
	}
}

func TestArrowNavigationReachesCreationActions(t *testing.T) {
	p, f := setupPicker(t)
	p.expanded = map[string]bool{}
	p.render(0)
	key(p, tcell.KeyDown)
	key(p, tcell.KeyDown)
	c, ok := p.selected()
	if !ok || c.dest.Kind != "new-tab" {
		t.Fatal("Down should reach New tab after workspaces")
	}
	key(p, tcell.KeyEnter)
	if p.mode != "workspaces" || len(f.moves()) != 0 {
		t.Fatal("New tab must open workspace chooser")
	}
	for _, row := range p.rows {
		if row.dest.Kind != "workspace" {
			t.Fatal("creation action inside chooser")
		}
	}
	key(p, tcell.KeyEscape)
	c, _ = p.selected()
	if c.dest.Kind != "new-tab" {
		t.Fatal("Back lost New tab selection")
	}
	key(p, tcell.KeyDown)
	c, _ = p.selected()
	if c.dest.Kind != "new-workspace" {
		t.Fatal("Down should reach New workspace")
	}
	key(p, tcell.KeyUp)
	c, _ = p.selected()
	if c.dest.Kind != "new-tab" {
		t.Fatal("Up should return to New tab")
	}
	key(p, tcell.KeyDown)
	key(p, tcell.KeyEnter)
	if len(f.moves()) != 1 {
		t.Fatal("Enter must move to new workspace once")
	}
}

func TestCreationSeparatorIsNotSelectable(t *testing.T) {
	p, f := setupPicker(t)
	p.expanded = map[string]bool{}
	p.render(0)
	row := len(p.rows) - 3
	if p.rows[row].dest.Kind != "separator" || !p.list.GetCell(row, 0).NotSelectable {
		t.Fatal("separator should not be selectable")
	}
	drawPicker(t, p, 40, 12)
	x, y, _, _ := p.list.GetInnerRect()
	before, _ := p.list.GetSelection()
	click(p, x+3, y+row)
	after, _ := p.list.GetSelection()
	if before != after || len(f.calls) != 0 {
		t.Fatal("separator click changed selection or called Herdr")
	}
	p.list.Select(row+1, 0)
	key(p, tcell.KeyUp)
	after, _ = p.list.GetSelection()
	if after != row-1 {
		t.Fatal("Up did not skip separator")
	}
	key(p, tcell.KeyDown)
	after, _ = p.list.GetSelection()
	if after != row+1 {
		t.Fatal("Down did not skip separator")
	}
}
