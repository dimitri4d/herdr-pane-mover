package main

import "strings"

type choice struct {
	label                      string
	dest                       Destination
	depth                      int
	expandable, open, disabled bool
}

func (c choice) key() string {
	switch c.dest.Kind {
	case "branch", "workspace":
		return "workspace:" + c.dest.Workspace
	case "tab":
		return "tab:" + c.dest.Tab
	default:
		return "pane:" + c.dest.Target.ID
	}
}
func plain(label, id string) string {
	if s := clean(strings.TrimSpace(label)); s != "" {
		return s
	}
	return id
}
func paneLabel(p Pane) string {
	s := p.Label
	if s == "" {
		s = p.Title
	}
	return plain(s, p.ID)
}
func uniqueChoices(items []choice) []choice {
	counts := map[string]int{}
	for _, c := range items {
		counts[c.label]++
	}
	for i, c := range items {
		if counts[c.label] > 1 {
			id := c.dest.Tab
			if c.dest.Kind == "pane" || c.dest.Kind == "swap" {
				id = c.dest.Target.ID
			}
			if c.dest.Kind == "branch" || c.dest.Kind == "workspace" {
				id = c.dest.Workspace
			}
			items[i].label = name(c.label, id)
		}
	}
	return items
}
func treeChoices(s Snapshot, src Pane, mode string, expanded map[string]bool) []choice {
	workspaces := []choice{}
	for _, local := range []bool{true, false} {
		for _, w := range s.Workspaces {
			if (w.ID == src.Workspace) != local {
				continue
			}
			kind := "branch"
			if mode == "workspaces" {
				kind = "workspace"
			}
			workspaces = append(workspaces, choice{label: plain(w.Label, w.ID), dest: Destination{Kind: kind, Workspace: w.ID}, expandable: mode != "workspaces", open: expanded["workspace:"+w.ID]})
		}
	}
	workspaces = uniqueChoices(workspaces)
	out := []choice{}
	for _, w := range workspaces {
		out = append(out, w)
		if !w.open || mode == "workspaces" {
			continue
		}
		tabs := []choice{}
		for _, t := range s.Tabs {
			if t.Workspace != w.dest.Workspace {
				continue
			}
			label := plain(t.Label, t.ID)
			if label == t.Label {
				label = "Tab " + label
			}
			tabs = append(tabs, choice{label: label, dest: Destination{Kind: "tab", Workspace: t.Workspace, Tab: t.ID}, depth: 1, expandable: true, open: expanded["tab:"+t.ID], disabled: t.ID == src.Tab})
		}
		for _, t := range uniqueChoices(tabs) {
			out = append(out, t)
			if !t.open {
				continue
			}
			panes := []choice{}
			for _, p := range s.Panes {
				if p.Tab != t.dest.Tab {
					continue
				}
				// Herdr cannot move within a tab, but panes there can swap with the source.
				kind := "pane"
				if t.disabled {
					kind = "swap"
				}
				panes = append(panes, choice{label: paneLabel(p), dest: Destination{Kind: kind, Workspace: p.Workspace, Tab: p.Tab, Target: p}, depth: 2, disabled: p.ID == src.ID})
			}
			out = append(out, uniqueChoices(panes)...)
		}
	}
	if mode != "workspaces" {
		if len(out) > 0 {
			out = append(out, choice{dest: Destination{Kind: "separator"}})
		}
		out = append(out,
			choice{label: "New tab…", dest: Destination{Kind: "new-tab"}},
			choice{label: "New workspace", dest: Destination{Kind: "new-workspace"}},
		)
	}
	return out
}
