# Release checklist

Releases distribute source. Herdr builds the selected checkout locally; no executable assets or binary checksums are published.

## Automated checks

Use Go 1.26.8+ and Python 3.11+:

```sh
sh scripts/install.sh
./bin/pane-mover version
go test -race ./...
go vet ./...
go mod verify
python3 scripts/validate.py
python3 scripts/test-install.py
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

CI runs these checks on macOS and Linux with read-only repository permissions. Keep Go, dependencies, and pinned GitHub Actions updated; review updates and run the checks before merging.

## Live compatibility

Run these in a scratch workspace before tagging, on each advertised platform. Record the tested commit, date, OS/architecture, Go version, Herdr version, and any gaps in the release notes, and limit compatibility claims accordingly.

- Move a running counter/process to a selected tab, exact pane, new tab, and new workspace. Confirm its PID/output survives, focus follows it, and Right/Below split at 50/50.
- Repeat with Left and Above: the pane lands on that side of the target, keeps its PID, and keeps focus. Make the follow-up swap fail (for example, close the target pane between move and swap) and confirm the "landed … instead of …" message.
- Swap the source with another pane in its own tab, from the tree and from the mini-map, including in a zoomed tab. Both PIDs survive and focus stays on the source.
- Mini-map: edge clicks pick that placement, centre clicks use the selected one, clicking the source does nothing, and the preview hides under 90 columns.
- Exercise mouse and keyboard navigation, expansion, Back/Cancel, and layout at 100×24, 72×18 and 40×12.
- Test changed/closed destinations, zoomed tabs, command failure, and repeated clicks. Confirm Unzoom & move requires explicit activation, preserves the exact target across focus changes, and sends only one move. Check partial unzoom failures leave tabs unzoomed and require Refresh/reselection. Direct actions must not unzoom automatically.
- Test cancellation before and during a move; inspect the resulting location before retrying.
- Verify both direct actions and their error logs.

### Herdr API notes (0.9.5, protocol 22)

These are why `min_herdr_version` is 0.9.5; `pane swap` and `pane layout` are untested on older releases.

- `pane swap --source-pane A --target-pane B` exchanges two panes anywhere in a tab, including a zoomed one, keeps both processes, and focuses A.
- `pane move --split right|down` followed by a swap with the target gives the Left and Above placements. The moved pane keeps its ID.
- `pane layout` returns the tab `area` and per-pane `rect` that the mini-map draws.
- `layout.apply` with existing pane IDs rebuilds the tab with new panes and processes, so it can't be used for retiling.

## Publish

1. Match the version in `herdr-plugin.toml` and `main.go`; update the README install example.
2. Complete the checks and put the live results in the release notes. Set `min_herdr_version` to a version actually tested successfully.
3. Commit and review the source, then create and push the matching version tag. Do not move an existing release tag.
4. Verify a fresh pinned Herdr installation and add the repository topic `herdr-plugin` for discovery.

The checklist is reviewed manually; there is no automatic publishing workflow.
