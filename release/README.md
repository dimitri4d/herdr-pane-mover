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

Version 0.1.0 remains unverified. The manifest minimum of Herdr 0.9.0 is provisional. Before tagging, record the tested source commit, date, OS/architecture, Go version, Herdr version, and observations below. Test each advertised platform; record gaps explicitly and limit compatibility claims accordingly.

- [ ] Move a running counter/process to a selected tab, exact pane, new tab, and new workspace. Confirm its PID/output survives, focus follows it, and Right/Below split at 50/50.
- [ ] Exercise mouse and keyboard navigation, expansion, Back/Cancel, and layout at 72×18 and 40×12.
- [ ] Test changed/closed destinations, zoomed tabs, command failure, and repeated clicks. Confirm Unzoom & move requires explicit activation, preserves the exact target across focus changes, and sends only one move. Check partial unzoom failures leave tabs unzoomed and require Refresh/reselection. Direct actions must not unzoom automatically.
- [ ] Test cancellation before and during a move; inspect the resulting location before retrying.
- [ ] Verify both direct actions and their error logs.

Results: **not yet tested**.

## Publish

1. Match the version in `herdr-plugin.toml` and `main.go`; update the README install example.
2. Complete the checks and record live results. Set `min_herdr_version` to a version actually tested successfully.
3. Commit and review the source and results, then create and push the matching version tag. Do not move an existing release tag.
4. Verify a fresh pinned Herdr installation and add the repository topic `herdr-plugin` for discovery.

The checklist is reviewed manually; there is no automatic publishing workflow.
