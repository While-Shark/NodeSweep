"""Exercise installer lifecycle against a fake systemctl and temporary filesystem."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


@unittest.skipUnless(os.geteuid() == 0, 'Installer needs root; CI runs this suite with sudo')
class InstallTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix='nodesweep-install-test-')
        self.base = Path(self.directory.name)
        self.bundle = self.base / 'bundle'
        self.bundle.mkdir()
        shutil.copy('install.sh', self.bundle / 'install.sh')
        (self.bundle / 'deploy').mkdir()
        shutil.copy('deploy/nodesweep.service', self.bundle / 'deploy/nodesweep.service')
        self.root = self.base / 'root'
        self.root.mkdir()
        shim = self.base / 'commands'
        shim.mkdir()
        systemctl = shim / 'systemctl'
        systemctl.write_text('''#!/usr/bin/env bash
case "$1" in
 is-active) test -f "$TEST_ROOT/active" ;;
 stop) rm -f "$TEST_ROOT/active" ;;
 restart)
   if grep -q 'BUILD=bad' "$TEST_ROOT/usr/local/bin/nodesweep"; then rm -f "$TEST_ROOT/active"; exit 1; fi
   touch "$TEST_ROOT/active" ;;
 *) exit 0 ;;
esac
''')
        systemctl.chmod(0o755)
        self.env = dict(os.environ, PATH=str(shim) + ':' + os.environ['PATH'], TEST_ROOT=str(self.root), NODESWEEP_INSTALL_TEST="1")
        self.binary('old')

    def tearDown(self):
        self.directory.cleanup()

    def binary(self, version):
        file = self.bundle / 'nodesweep'
        file.write_text(f'''#!/usr/bin/env bash
BUILD={version}
if [[ "$1" == -version ]]; then echo "NodeSweep $BUILD"; exit 0; fi
if [[ "$1" == -config && "$3" == -init ]]; then printf '{{"adminToken":"fixture-only"}}' > "$2"; chmod 600 "$2"; exit 0; fi
exit 1
''')
        file.chmod(0o755)

    def run_install(self, *args, success=True):
        result = subprocess.run(['bash', str(self.bundle / 'install.sh'), '--root', str(self.root), *args], env=self.env, text=True, capture_output=True)
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
        return result

    def test_upgrade_preserves_config_and_rollback_restores_binary(self):
        self.run_install('--start')
        config = self.root / 'etc/nodesweep/config.json'
        before = config.read_bytes()
        self.binary('new')
        self.run_install()
        binary = self.root / 'usr/local/bin/nodesweep'
        self.assertIn('BUILD=new', binary.read_text())
        self.assertEqual(config.read_bytes(), before)
        self.run_install('--rollback')
        self.assertIn('BUILD=old', binary.read_text())
        self.assertTrue((self.root / 'active').exists())
        self.assertEqual(config.stat().st_mode & 0o777, 0o600)

    def test_activation_failure_restores_running_binary(self):
        self.run_install('--start')
        self.binary('bad')
        self.run_install(success=False)
        self.assertIn('BUILD=old', (self.root / 'usr/local/bin/nodesweep').read_text())
        self.assertTrue((self.root / 'active').exists())
