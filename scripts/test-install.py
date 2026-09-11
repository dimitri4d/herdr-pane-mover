#!/usr/bin/env python3
"""Check source-install success and failure without touching the real executable."""
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parent.parent


class BuildTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='pane-mover-build-')
        self.addCleanup(temporary.cleanup)
        self.root = pathlib.Path(temporary.name)
        for folder in ['scripts', 'bin', 'mock']:
            (self.root / folder).mkdir()
        shutil.copy(ROOT / 'scripts/install.sh', self.root / 'scripts/install.sh')
        self.binary = self.root / 'bin/pane-mover'
        self.binary.write_bytes(b'previous executable')
        compiler = self.root / 'mock/go'
        compiler.write_text('''#!/bin/sh
set -eu
case "$1" in
 env)
  case "$2" in GOHOSTOS) echo linux ;; GOHOSTARCH) echo amd64 ;; GOVERSION) echo go1.26.8 ;; esac ;;
 build)
  [ "$GOTOOLCHAIN" = local ] && [ "$GOWORK" = off ] && [ "$CGO_ENABLED" = 0 ]
  [ "$GOOS" = linux ] && [ "$GOARCH" = amd64 ]
  [ "$2" = -mod=readonly ]
  [ "$TEST_FAIL" = 0 ] || exit 1
  while [ "$1" != -o ]; do shift; done
  printf 'new executable' > "$2" ;;
 *) exit 1 ;;
esac
''')
        compiler.chmod(0o755)
        self.env = dict(os.environ, PATH=str(self.root / 'mock') + os.pathsep + os.defpath,
                        TEST_FAIL='0', GOOS='windows', GOARCH='arm64')

    def install(self):
        return subprocess.run(['sh', str(self.root / 'scripts/install.sh')], cwd='/',
                              env=self.env, capture_output=True, text=True)

    def test_success(self):
        result = self.install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.binary.read_bytes(), b'new executable')
        self.assertTrue(os.access(self.binary, os.X_OK))
        self.assertEqual(list(self.binary.parent.glob('.pane-mover.*')), [])

    def test_build_failure_preserves_binary(self):
        self.env['TEST_FAIL'] = '1'
        self.assertNotEqual(self.install().returncode, 0)
        self.assertEqual(self.binary.read_bytes(), b'previous executable')
        self.assertEqual(list(self.binary.parent.glob('.pane-mover.*')), [])


if __name__ == '__main__':
    unittest.main()
