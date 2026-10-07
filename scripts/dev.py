#!/usr/bin/env python3
"""Run native backend and Vite with graceful stop and no credential output."""
import os
from pathlib import Path
import signal
import subprocess
import sys
import time

root = Path(__file__).resolve().parent.parent
os.chdir(root)
subprocess.run([sys.executable, 'scripts/init-env.py'], check=True)
subprocess.run(['docker', 'compose', 'up', '-d', '--wait', 'postgres', 'mailpit'], check=True)
subprocess.run([sys.executable, 'scripts/run-env.py', '--owner', 'go', 'run', './cmd/tenantforge', 'migrate'], check=True)
processes = []
def stop():
    for process in processes:
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
    for process in processes:
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
def interrupted(*_):
    raise KeyboardInterrupt
signal.signal(signal.SIGINT, interrupted)
signal.signal(signal.SIGTERM, interrupted)
exit_code = 0
try:
    processes.append(subprocess.Popen([sys.executable, 'scripts/run-env.py', 'go', 'run', './cmd/tenantforge', 'serve'], start_new_session=True))
    processes.append(subprocess.Popen(['npm', 'run', 'dev'], cwd=root/'web', start_new_session=True))
    print('TenantForge: http://localhost:5178 ; development mailbox: http://localhost:8029', flush=True)
    while all(process.poll() is None for process in processes):
        time.sleep(0.3)
    exit_code = next((process.returncode or 1 for process in processes if process.poll() is not None), 1)
except KeyboardInterrupt:
    pass
finally:
    stop()
sys.exit(exit_code)
