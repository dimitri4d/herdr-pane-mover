#!/usr/bin/env python3
"""Validate our manifest against the fields used by Herdr's v1 plugin schema."""
import pathlib, re, tomllib
root = pathlib.Path(__file__).resolve().parent.parent
m = tomllib.loads((root / 'herdr-plugin.toml').read_text())
assert m['id'] == 'herdr-pane-mover'
version = m['version']
assert re.fullmatch(r'\d+\.\d+\.\d+', version)
assert re.fullmatch(r'\d+\.\d+\.\d+', m['min_herdr_version']), 'Herdr requires an explicit minimum version'
assert m['platforms'] == ['macos', 'linux']
assert m['build'] == [{'command': ['sh', 'scripts/install.sh']}]
assert {a['id'] for a in m['actions']} == {'move', 'new-tab', 'new-workspace'}
for a in m['actions']:
    assert a['command'] == ['./bin/pane-mover', a['id']]
assert m['panes'] == [dict(id='picker', title='Move pane', command=['./bin/pane-mover', 'picker'], placement='popup', width=100, height=24)]
assert f'const version = "{version}"' in (root / 'main.go').read_text(), 'executable/manifest version mismatch'
print('Manifest and executable version validated')
