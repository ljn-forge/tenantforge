import importlib.util
import io
from pathlib import Path
import subprocess
import sys
import unittest
from unittest.mock import patch

SCRIPTS = Path(__file__).resolve().parents[1] / 'scripts'
sys.path.insert(0, str(SCRIPTS))
spec = importlib.util.spec_from_file_location('recovery_check', SCRIPTS / 'recovery-check.py')
recovery = importlib.util.module_from_spec(spec)
spec.loader.exec_module(recovery)


class RecoveryTests(unittest.TestCase):
    def test_cancelled_start_reaps_child_before_handle_is_returned(self):
        # Use a real detached process, then interrupt while readiness is pending.
        process = subprocess.Popen([sys.executable, '-c', 'import time; time.sleep(60)'], start_new_session=True)
        try:
            with patch.object(recovery.subprocess, 'Popen', return_value=process), patch.object(recovery.urllib.request, 'build_opener', side_effect=KeyboardInterrupt):
                with self.assertRaises(KeyboardInterrupt):
                    recovery.start({'TF_LISTEN': '127.0.0.1:8092'}, io.StringIO())
            self.assertIsNotNone(process.poll())
        finally:
            recovery.stop(process)
