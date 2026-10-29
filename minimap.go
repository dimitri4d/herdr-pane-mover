package main

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// minimap draws one tab's pane rectangles scaled into its area. Clicking near
// a box edge picks that side; clicking the centre keeps the current placement.
type minimap struct {
	*tview.Box
	layout   *Layout
	title    string
	message  string
	labels   map[string]string
	source   string
	target   string
	marker   Placement
	showSide bool
	pick     func(paneID string, side Placement, edge bool)
}

func newMinimap() *minimap {
	m := &minimap{Box: tview.NewBox(), labels: map[string]string{}}
	m.SetBackgroundColor(tcell.ColorDefault)
	return m
}

// show sets the tab to draw. A nil layout draws the message instead.
func (m *minimap) show(l *Layout, message string) {
	m.layout = l
	m.message = message
}

// box maps a Herdr pane rectangle to screen cells below the title row.
func (m *minimap) box(r Rect) (x0, y0, x1, y1 int) {
	ix, iy, iw, ih := m.GetInnerRect()
	iy, ih = iy+1, ih-1
	a := m.layout.Area
	if a.Width <= 0 || a.Height <= 0 {
		return 0, 0, -1, -1
	}
	x0 = ix + (r.X-a.X)*iw/a.Width
	y0 = iy + (r.Y-a.Y)*ih/a.Height
	x1 = ix + (r.X+r.Width-a.X)*iw/a.Width - 1
	y1 = iy + (r.Y+r.Height-a.Y)*ih/a.Height - 1
	return
}

func (m *minimap) Draw(screen tcell.Screen) {
	m.Box.DrawForSubclass(screen, m)
	ix, iy, iw, _ := m.GetInnerRect()
	if m.layout == nil {
		tview.Print(screen, m.message, ix, iy, iw, tview.AlignLeft, tcell.ColorDefault)
		return
	}
	tview.Print(screen, tview.Escape(m.title), ix, iy, iw, tview.AlignLeft, tcell.ColorDefault)
	for _, p := range m.layout.Panes {
		x0, y0, x1, y1 := m.box(p.Rect)
		if x1-x0 < 1 || y1-y0 < 1 {
			continue
		}
		style := tcell.StyleDefault
		if p.ID == m.source {
			style = style.Dim(true)
		}
		if p.ID == m.target {
			style = style.Bold(true)
		}
		for x := x0 + 1; x < x1; x++ {
			screen.SetContent(x, y0, tview.Borders.Horizontal, nil, style)
			screen.SetContent(x, y1, tview.Borders.Horizontal, nil, style)
		}
		for y := y0 + 1; y < y1; y++ {
			screen.SetContent(x0, y, tview.Borders.Vertical, nil, style)
			screen.SetContent(x1, y, tview.Borders.Vertical, nil, style)
		}
		screen.SetContent(x0, y0, tview.Borders.TopLeft, nil, style)
		screen.SetContent(x1, y0, tview.Borders.TopRight, nil, style)
		screen.SetContent(x0, y1, tview.Borders.BottomLeft, nil, style)
		screen.SetContent(x1, y1, tview.Borders.BottomRight, nil, style)
		label := m.labels[p.ID]
		if label == "" {
			label = p.ID
		}
		if p.ID == m.source {
			label = "(moving) " + label
		}
		if y1-y0 > 1 {
			tview.Print(screen, tview.Escape(label), x0+1, y0+1, x1-x0-1, tview.AlignLeft, tcell.ColorDefault)
		}
		if p.ID == m.target && m.showSide {
			// Mark the edge the moved pane will land against.
			x, y, r := (x0+x1)/2, (y0+y1)/2, '▶'
			switch m.marker {
			case Left:
				x, r = x0, '◀'
			case Right:
				x = x1
			case Above:
				y, r = y0, '▲'
			case Below:
				y, r = y1, '▼'
			}
			screen.SetContent(x, y, r, nil, style)
		}
	}
}

// at returns the pane under a cell and the side it is nearest to. edge is
// false when the cell is inside the box, more than one cell from every edge.
func (m *minimap) at(x, y int) (id string, side Placement, edge bool) {
	if m.layout == nil {
		return "", "", false
	}
	for _, p := range m.layout.Panes {
		x0, y0, x1, y1 := m.box(p.Rect)
		if x < x0 || x > x1 || y < y0 || y > y1 {
			continue
		}
		side, d := Left, x-x0
		for _, c := range []struct {
			side Placement
			d    int
		}{{Right, x1 - x}, {Above, y - y0}, {Below, y1 - y}} {
			if c.d < d {
				side, d = c.side, c.d
			}
		}
		return p.ID, side, d <= 1
	}
	return "", "", false
}

func (m *minimap) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return m.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		x, y := event.Position()
		if !m.InRect(x, y) {
			return false, nil
		}
		if action == tview.MouseLeftClick && m.pick != nil {
			if id, side, edge := m.at(x, y); id != "" {
				m.pick(id, side, edge)
			}
		}
		return true, nil
	})
}
