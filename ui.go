package main

import (
	"context"
	"errors"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type picker struct {
	retry                                     *Destination
	unzoom                                    *tview.Button
	c                                         Client
	app                                       *tview.Application
	ctx                                       context.Context
	cancel                                    context.CancelFunc
	launch                                    func(func())
	dispatch                                  func(func())
	sourceID                                  string
	src                                       Pane
	snap                                      Snapshot
	mode, direction                           string
	savedRow                                  int
	expanded                                  map[string]bool
	rows                                      []choice
	pending, failed                           bool
	header, status, divider                   *tview.TextView
	list                                      *tview.Table
	right, below, back, refresh, cancelButton *tview.Button
	root, toolbar, footer                     *tview.Flex
}

func textView() *tview.TextView {
	v := tview.NewTextView().SetDynamicColors(false).SetTextColor(tcell.ColorDefault)
	v.SetBackgroundColor(tcell.ColorDefault)
	return v
}
func flex() *tview.Flex { f := tview.NewFlex(); f.SetBackgroundColor(tcell.ColorDefault); return f }
func button(label string, fn func()) *tview.Button {
	b := tview.NewButton(label).SetSelectedFunc(fn).SetStyle(tcell.StyleDefault).SetActivatedStyle(tcell.StyleDefault.Underline(true)).SetDisabledStyle(tcell.StyleDefault.Dim(true))
	b.SetBackgroundColor(tcell.ColorDefault)
	return b
}
func spacer() *tview.Box { return tview.NewBox().SetBackgroundColor(tcell.ColorDefault) }
func newPicker(c Client, id string) *picker {
	ctx, cancel := context.WithCancel(context.Background())
	p := &picker{c: c, sourceID: id, ctx: ctx, cancel: cancel, app: tview.NewApplication(), mode: "tree", direction: "right", expanded: map[string]bool{}}
	p.launch = func(fn func()) { go fn() }
	p.dispatch = func(fn func()) { p.app.QueueUpdateDraw(fn) }
	p.header = textView().SetWrap(false)
	p.status = textView().SetWrap(true)
	p.divider = textView().SetWrap(false)
	p.list = tview.NewTable().SetSelectable(true, false).SetSelectedStyle(tcell.StyleDefault.Bold(true).Underline(true))
	p.list.SetBackgroundColor(tcell.ColorDefault)
	p.right = button("(*) Right", func() { p.setDirection("right") })
	p.below = button("( ) Below", func() { p.setDirection("down") })
	p.back = button("Back", p.goBack)
	p.refresh = button("Refresh", p.reload)
	p.unzoom = button("Unzoom & move", p.retryUnzoom)
	p.cancelButton = button("Cancel", p.stop)
	p.toolbar = flex()
	p.footer = flex()
	p.root = flex().SetDirection(tview.FlexRow).AddItem(p.header, 2, 0, false).AddItem(spacer(), 1, 0, false).AddItem(p.list, 0, 1, true).AddItem(p.toolbar, 1, 0, false).AddItem(spacer(), 1, 0, false).AddItem(p.divider, 1, 0, false).AddItem(p.footer, 1, 0, false)
	p.list.SetSelectedFunc(func(int, int) { p.activate() })
	p.list.SetSelectionChangedFunc(func(int, int) {
		p.highlightSelection()
		if p.pending || p.failed {
			return
		}
		c, ok := p.selected()
		if ok && c.disabled {
			p.status.SetText("Current tab: unavailable")
		} else {
			p.status.SetText("Click a tab or pane to move.")
		}
	})
	p.list.SetMouseCapture(func(a tview.MouseAction, e *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		x, y := e.Position()
		if !p.list.InRect(x, y) {
			return a, e
		}
		if p.pending || p.failed {
			return 0, nil
		}
		if a == tview.MouseLeftClick {
			r, _ := p.list.CellAt(x, y)
			if r >= 0 && r < len(p.rows) {
				if p.rows[r].dest.Kind == "separator" {
					return 0, nil
				}
				p.list.Select(r, 0)
				c := p.rows[r]
				ix, _, _, _ := p.list.GetInnerRect()
				if c.expandable && (c.dest.Kind == "branch" || x >= ix && x <= ix+2+c.depth*2+1) {
					p.expand(r)
				} else {
					p.activate()
				}
			}
			return 0, nil
		}
		return a, e
	})
	p.app.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyEscape {
			if p.mode == "workspaces" && !p.pending && !p.failed {
				p.goBack()
			} else {
				p.stop()
			}
			return nil
		}
		if p.pending {
			return nil
		}
		if e.Key() == tcell.KeyTab || e.Key() == tcell.KeyBacktab {
			items := []tview.Primitive{p.list}
			if p.failed {
				items = []tview.Primitive{p.status}
				if p.retry != nil {
					items = append(items, p.unzoom)
				}
				items = append(items, p.refresh, p.cancelButton)
			} else if p.mode == "workspaces" {
				items = append(items, p.back, p.cancelButton)
			} else {
				items = append(items, p.right, p.below, p.cancelButton)
			}
			i := 0
			for j, v := range items {
				if v.HasFocus() {
					i = j
					break
				}
			}
			step := 1
			if e.Key() == tcell.KeyBacktab {
				step = -1
			}
			p.app.SetFocus(items[(i+step+len(items))%len(items)])
			return nil
		}
		if p.failed {
			return e
		}
		// Arrow navigation should work immediately after clicking a control too.
		if e.Key() == tcell.KeyUp || e.Key() == tcell.KeyDown {
			p.app.SetFocus(p.list)
			row, _ := p.list.GetSelection()
			if e.Key() == tcell.KeyUp {
				row--
			} else {
				row++
			}
			if row >= 0 && row < len(p.rows) && p.rows[row].dest.Kind == "separator" {
				if e.Key() == tcell.KeyUp {
					row--
				} else {
					row++
				}
			}
			if len(p.rows) > 0 {
				p.list.Select(max(0, min(row, len(p.rows)-1)), 0)
			}
			return nil
		}
		if p.list.HasFocus() {
			r, _ := p.list.GetSelection()
			switch e.Key() {
			case tcell.KeyRight:
				if r >= 0 && r < len(p.rows) && p.rows[r].expandable {
					if !p.rows[r].open {
						p.expand(r)
					} else if r+1 < len(p.rows) && p.rows[r+1].depth > p.rows[r].depth {
						p.list.Select(r+1, 0)
					}
				}
				return nil
			case tcell.KeyLeft:
				if r >= 0 && r < len(p.rows) {
					if p.rows[r].expandable && p.rows[r].open {
						p.expand(r)
					} else {
						for i := r - 1; i >= 0; i-- {
							if p.rows[i].depth < p.rows[r].depth {
								p.list.Select(i, 0)
								break
							}
						}
					}
				}
				return nil
			}
		}
		return e
	})
	p.app.SetBeforeDrawFunc(func(s tcell.Screen) bool {
		w, _ := s.Size()
		p.divider.SetText(strings.Repeat("─", max(0, w)))
		return false
	})
	p.app.SetRoot(p.root, true).EnableMouse(true).SetFocus(p.list)
	p.controls()
	return p
}
func (p *picker) stop() { p.cancel(); p.app.Stop() }
func (p *picker) selected() (choice, bool) {
	r, _ := p.list.GetSelection()
	if r < 0 || r >= len(p.rows) {
		return choice{}, false
	}
	return p.rows[r], true
}
func (p *picker) controls() {
	p.toolbar.Clear()
	p.footer.Clear()
	p.footer.SetDirection(tview.FlexColumn)
	p.unzoom.SetDisabled(p.pending)
	for _, b := range []*tview.Button{p.right, p.below, p.back, p.refresh} {
		b.SetDisabled(p.pending || p.failed)
	}
	if p.failed {
		p.refresh.SetDisabled(p.pending)
		if p.retry != nil {
			p.root.ResizeItem(p.footer, 5, 0)
			buttons := flex().AddItem(p.unzoom, 16, 0, false).AddItem(p.refresh, 10, 0, false).AddItem(p.cancelButton, 9, 0, false)
			p.footer.SetDirection(tview.FlexRow).AddItem(p.status, 0, 1, false).AddItem(buttons, 1, 0, false)
			return
		}
		p.root.ResizeItem(p.footer, 3, 0)
		p.footer.AddItem(p.status, 0, 1, false).AddItem(p.refresh, 10, 0, false).AddItem(p.cancelButton, 9, 0, false)
		return
	}
	p.root.ResizeItem(p.footer, 1, 0)
	if p.mode == "tree" {
		p.toolbar.AddItem(textView().SetText("Placement"), 11, 0, false).AddItem(p.right, 12, 0, false).AddItem(p.below, 12, 0, false)
	} else {
		p.toolbar.AddItem(p.back, 8, 0, false)
	}
	p.footer.AddItem(p.status, 0, 1, false).AddItem(p.cancelButton, 9, 0, false)
}
func (p *picker) render(index int) {
	p.rows = treeChoices(p.snap, p.src, p.mode, p.expanded)
	p.list.Clear()
	for i, c := range p.rows {
		style := tcell.StyleDefault
		if c.disabled {
			style = style.Dim(true)
		}
		p.list.SetCell(i, 0, tview.NewTableCell(tview.Escape(treeRowText(c, false))).SetStyle(style).SetSelectable(c.dest.Kind != "separator").SetExpansion(1))
	}
	if len(p.rows) == 0 {
		p.list.SetCell(0, 0, tview.NewTableCell("No destinations available").SetStyle(tcell.StyleDefault).SetSelectable(false))
	}
	p.status.SetText("Click a tab or pane to move.")
	p.list.Select(max(0, min(index, len(p.rows)-1)), 0)
	p.header.SetText("Move pane\nFrom " + p.snap.contextName(p.src))
	if p.mode == "workspaces" {
		p.header.SetText("New tab in…\nChoose a workspace")
		p.status.SetText("Click a workspace to move.")
	}
	p.controls()
}
func (p *picker) expand(r int) {
	if p.pending || p.failed || r < 0 || r >= len(p.rows) || !p.rows[r].expandable {
		return
	}
	c := p.rows[r]
	p.expanded[c.key()] = !p.expanded[c.key()]
	p.render(r)
	// Reveal a child when expanding a row at the bottom of a short viewport.
	if p.expanded[c.key()] && r+1 < len(p.rows) && p.rows[r+1].depth > c.depth {
		_, _, _, height := p.list.GetInnerRect()
		offset, _ := p.list.GetOffset()
		if height > 1 && r+1 >= offset+height {
			p.list.SetOffset(max(0, r+2-height), 0)
		}
	}
}
func (s Snapshot) contextName(src Pane) string {
	w, t := src.Workspace, src.Tab
	for _, v := range s.Workspaces {
		if v.ID == src.Workspace {
			w = plain(v.Label, v.ID)
		}
	}
	for _, v := range s.Tabs {
		if v.ID == src.Tab {
			t = "Tab " + plain(v.Label, v.ID)
		}
	}
	return w + " / " + t
}
func (p *picker) applySnapshot(s Snapshot, src Pane) {
	if p.src.ID == "" {
		p.expanded["workspace:"+src.Workspace] = true
	}
	p.snap = s
	p.src = src
	p.mode = "tree"
	p.failed = false
	p.render(0)
	p.app.SetFocus(p.list)
}
func (p *picker) busy(b bool) { p.pending = b; p.controls() }
func (p *picker) showError(err error) {
	p.failed = true
	p.pending = false
	p.rows = nil
	p.list.Clear()
	p.retry = nil
	var zoom *zoomMoveError
	if errors.As(err, &zoom) {
		d := zoom.destination
		p.retry = &d
	}
	p.controls()
	p.status.SetText(clean(err.Error()))
	if p.retry != nil {
		p.status.SetText("Source or destination tab is zoomed. Unzoom & move leaves affected tabs unzoomed, even if the move fails.")
	}
	p.status.ScrollToBeginning()
	p.app.SetFocus(p.refresh)
	if p.retry != nil {
		p.app.SetFocus(p.unzoom)
	}
}
func (p *picker) reload() {
	if p.pending {
		return
	}
	p.retry = nil
	p.busy(true)
	p.status.SetText("Loading destinations…")
	p.launch(func() {
		src, e := p.c.pane(p.ctx, "pane", "get", p.sourceID)
		var s Snapshot
		if e == nil {
			s, e = p.c.load(p.ctx)
		}
		if p.ctx.Err() != nil {
			return
		}
		p.dispatch(func() {
			if e != nil {
				p.showError(e)
				return
			}
			p.failed = false
			p.busy(false)
			p.applySnapshot(s, src)
		})
	})
}
func (p *picker) setDirection(dir string) {
	if p.pending || p.failed {
		return
	}
	p.direction = dir
	p.right.SetLabel("( ) Right")
	p.below.SetLabel("( ) Below")
	if dir == "right" {
		p.right.SetLabel("(*) Right")
	} else {
		p.below.SetLabel("(*) Below")
	}
}
func (p *picker) chooseWorkspace() {
	if p.pending || p.failed {
		return
	}
	p.savedRow, _ = p.list.GetSelection()
	p.mode = "workspaces"
	p.render(0)
	p.app.SetFocus(p.list)
}
func (p *picker) goBack() {
	if p.pending || p.failed || p.mode != "workspaces" {
		return
	}
	p.mode = "tree"
	p.render(p.savedRow)
	p.app.SetFocus(p.list)
}
func (p *picker) activate() {
	if p.pending || p.failed {
		return
	}
	c, ok := p.selected()
	if !ok {
		return
	}
	if c.dest.Kind == "new-tab" {
		p.chooseWorkspace()
		return
	}
	if c.dest.Kind == "branch" {
		r, _ := p.list.GetSelection()
		p.expand(r)
		return
	}
	if c.disabled {
		p.status.SetText("Current tab: unavailable")
		return
	}
	p.submit(c.dest)
}
func (p *picker) retryUnzoom() {
	if p.pending || !p.failed || p.retry == nil {
		return
	}
	d := *p.retry
	p.retry = nil
	p.failed = false
	p.submitMove(d, true)
}
func (p *picker) submit(d Destination) { p.submitMove(d, false) }
func (p *picker) submitMove(d Destination, unzoom bool) {
	if p.pending || p.failed {
		return
	}
	src, dir := p.src, p.direction
	p.busy(true)
	p.status.SetText("Moving…")
	p.launch(func() {
		e := p.c.moveWithZoom(p.ctx, src, d, dir, unzoom)
		if p.ctx.Err() != nil {
			return
		}
		p.dispatch(func() {
			if e != nil {
				if unzoom {
					e = errors.New("Unzoom & move did not complete. Tabs may remain unzoomed. " + e.Error())
				}
				p.showError(e)
			} else {
				p.stop()
			}
		})
	})
}
func runPicker(c Client, id string) error {
	p := newPicker(c, id)
	defer p.cancel()
	p.reload()
	return p.app.Run()
}

// Text marks selection even when the host terminal does not render underline.
func treeRowText(c choice, selected bool) string {
	if c.dest.Kind == "separator" {
		return "  ---"
	}
	cursor := "  "
	if selected {
		cursor = "> "
	}
	disclosure := " "
	if c.expandable {
		disclosure = "▸"
		if c.open {
			disclosure = "▾"
		}
	}
	if c.dest.Kind == "new-tab" || c.dest.Kind == "new-workspace" {
		disclosure = "+"
	}
	label := cursor + strings.Repeat("  ", c.depth) + disclosure + " " + c.label
	if c.disabled {
		label += " (current)"
	}
	return label
}
func (p *picker) highlightSelection() {
	row, _ := p.list.GetSelection()
	for i, c := range p.rows {
		p.list.GetCell(i, 0).SetText(tview.Escape(treeRowText(c, i == row)))
	}
}
