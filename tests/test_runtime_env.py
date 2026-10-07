import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


class RuntimeEnvironmentTests(unittest.TestCase):
    def test_runtime_strips_owner_secrets_and_owner_mode_is_explicit(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / 'scripts').mkdir()
            source = Path(__file__).resolve().parents[1] / 'scripts/run-env.py'
            shutil.copyfile(source, root / 'scripts/run-env.py')
            names = ['TF_DB_OWNER_PASSWORD', 'TF_DB_APP_PASSWORD', 'TF_MIGRATION_DATABASE_URL', 'TF_TEST_DATABASE_URL']
            (root / '.env').write_text('\n'.join(name + '=fixture' for name in names) + '\nTF_MASTER_KEY=runtime-fixture\n')
            child = 'import json,os; print(json.dumps({key:key in os.environ for key in ' + repr(names + ['TF_MASTER_KEY']) + '}))'
            environment = {key: value for key, value in os.environ.items() if not key.startswith('TF_')}
            for owner in [False, True]:
                args = [sys.executable, str(root / 'scripts/run-env.py')]
                if owner:
                    args.append('--owner')
                result = subprocess.run(args + [sys.executable, '-c', child], env=environment, check=True, capture_output=True, text=True)
                observed = json.loads(result.stdout)
                self.assertTrue(observed['TF_MASTER_KEY'])
                self.assertTrue(all(observed[name] == owner for name in names))

    def test_explicit_restore_configuration_overrides_inherited_live_target(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / 'scripts').mkdir()
            shutil.copyfile(Path(__file__).resolve().parents[1] / 'scripts/run-env.py', root / 'scripts/run-env.py')
            config = root / 'restore.env'
            config.write_text('TF_DATABASE_URL=restored-fixture\nTF_SMTP_HOST=\n')
            environment = dict(os.environ, TF_DATABASE_URL='live-fixture', TF_SMTP_HOST='live-mail')
            child = 'import json,os; print(json.dumps([os.environ["TF_DATABASE_URL"],os.environ["TF_SMTP_HOST"]]))'
            result = subprocess.run([sys.executable, str(root / 'scripts/run-env.py'), '--env-file', str(config), sys.executable, '-c', child], env=environment, check=True, capture_output=True, text=True)
            self.assertEqual(json.loads(result.stdout), ['restored-fixture', ''])
