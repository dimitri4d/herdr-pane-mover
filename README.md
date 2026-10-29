# Herdr Pane Mover

Move a running pane with a minimal **Workspace → Tab → Pane** tree. Expand to the destination, then click its name to move. The process stays in its pane; the plugin calls Herdr's `pane move` API.

## UI preview

Example with a workspace and tab expanded, and Above selected (names are illustrative):

```text
Move pane
From _OPS / Tab 1

  ▾ briove                       Tab 1
    ▾ Tab 1                      ┌──────────┐┌────▲─────┐
        Shell                    │Shell     ││Server    │
>       Server                   │          ││          │
  ▸ herdr plugin                 │          ││          │
  ▸ _OPS                         └──────────┘└──────────┘
  ---
  + New tab…
  + New workspace

Placement   ( ) Right  ( ) Below  ( ) Left  (*) Above

────────────────────────────────────────────────────────
Click to move above Server.                       Cancel
```

## Install

```sh
herdr plugin install dimitri4d/herdr-pane-mover --ref v0.2.0
```

Installation builds the checked-out Go source into `bin/pane-mover`. It requires macOS or Linux, Git, and **Go 1.26.8 or newer** installed on your PATH. Go downloads pinned dependencies as needed and verifies them against `go.sum`; the installer does not download a compiler or prebuilt executable. Go is needed only when building or reinstalling.

The command above installs the 0.2.0 release tag. Use `--ref main` for the latest unreleased source, or a full commit SHA for an immutable reference. Installing or linking a plugin runs ordinary code with your user permissions.

## Bind actions

Add to `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+m"
type = "plugin_action"
command = "herdr-pane-mover.move"
description = "move pane (picker)"

[[keys.command]]
key = "prefix+shift+m"
type = "plugin_action"
command = "herdr-pane-mover.new-tab"
description = "move pane to new tab"
```

Then run `herdr server reload-config`.

| Public action | Behavior |
| --- | --- |
| `herdr-pane-mover.move` | Capture the calling pane and open the picker. |
| `herdr-pane-mover.new-tab` | Immediately move into a new tab in the source workspace. |
| `herdr-pane-mover.new-workspace` | Immediately move into a new workspace. |

You can also run `herdr plugin action invoke herdr-pane-mover.new-workspace` inside Herdr.

## Move a pane

The source workspace starts expanded; other workspaces and all tabs start collapsed. Click a workspace name or its arrow to expand/collapse it. Click a tab's arrow to reveal its panes. Expanding or navigating never moves anything. With the mini-map visible, highlighting a tab reads its layout once per Refresh; every move checks the source and target again first.

- **Tab name:** move immediately beside that tab's active pane, resolved at activation time.
- **Pane name:** move immediately beside that exact pane.
- **Placement (Right / Below / Left / Above):** choose where the pane lands relative to its target before moving. Splits default to Right at 50/50; the choice lasts for this popup only. Herdr only splits right or down, so Left and Above split that way and then swap the moved pane with its target. If that swap fails, the pane stays in the destination tab on the opposite side and the picker says so; nothing is rolled back.
- **Mini-map:** when the popup is at least 90 columns wide, the highlighted tab is drawn on the right, with the target in bold and an arrow on the edge where the pane will land. Click near a box edge to move against that edge, or click the middle of a box to use the selected placement.
- **Panes in the current tab:** click one to swap places with it. The split shape and both processes stay as they were, and placement doesn't apply. The source pane is marked `(current)`.
- **+ New tab…:** choose a workspace, with the source workspace first. Clicking a workspace creates a new tab there and moves the pane.
- **+ New workspace:** immediately create a workspace and move the pane into its first tab.

Herdr chooses new tab/workspace names. The current tab is marked `(current)`. Herdr can't move a pane within its own tab, so the current tab only offers swaps.

New tab and New workspace appear directly after the workspace tree and scroll with it. Use Up/Down to navigate, Right/Left to expand or collapse, Enter to activate, and Tab/Shift+Tab to cycle controls, including the four placement choices. Escape goes Back from the workspace chooser or closes the tree; Cancel closes the popup. All actions also support mouse clicks.

The popup requests 100×24 cells and scrolls the tree independently. Under 90 columns the mini-map is hidden. Controls are tested in a simulated terminal at 40×12, where Left and Above are clipped; extremely small terminals may clip more. Colors follow your terminal, with a `>` marker and bold, underlined selection.

## Limitations

If a source or destination tab is zoomed, the picker offers **Unzoom & move**. Selecting it unzooms the affected tabs, rechecks the original destination, and attempts the move once. Tabs remain unzoomed even if the move fails. Nothing is unzoomed without this explicit action; direct shortcuts require manual unzooming. Other errors offer Refresh/Cancel, and Refresh requires another explicit selection.

Preset layouts (side by side, stacked, grid) and moving a pane to an arbitrary spot in its own tab aren't offered. Herdr 0.9.5 can only swap existing panes; its `layout.apply` API rebuilds the tab with new processes, which would kill whatever was running.

Herdr can change between validation and submission. A timeout or cancellation during a move can leave its outcome uncertain: inspect the pane's location before retrying. Cancel cannot undo an accepted move.

## Troubleshooting

- **Install fails:** check `go version` (1.26.8+) and dependency-download access. A failed build preserves the previous local executable.
- **Popup won't open / direct action failed:** inspect `herdr plugin log`. Herdr may reject popups while another modal is open.
- **Destination changed or disappeared:** Refresh and select again. A source pane that closed requires reopening from another pane.
- **Zoomed tab / no move:** select Unzoom & move in the picker, or unzoom manually and Refresh. Same-tab moves are unsupported; swap with a pane in the current tab instead. Swaps also work in zoomed tabs.
- **"Moved, but landed … instead of …":** the move worked but the Left/Above swap didn't. Swap the two panes from the target tab, or move again.
- **Cannot see a pane:** expand its workspace and tab, then scroll the tree if needed. Click the arrow to expand; click the name to move.

## Build from source

From a local checkout:

```sh
sh scripts/install.sh
herdr plugin link "$PWD"
```

`plugin link` does not run the build hook; build before linking. See the [release checklist](release/README.md) for automated checks and live compatibility testing.

## Uninstall

```sh
herdr plugin uninstall herdr-pane-mover
```

Remove its keybindings and reload config. Herdr removes a managed checkout; a locally linked source directory remains yours. The plugin stores no persistent user settings.

## License

MIT; see [LICENSE](LICENSE) and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Dependency license texts are included under `licenses/`.
