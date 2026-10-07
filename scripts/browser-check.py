#!/usr/bin/env python3
"""Run persisted browser acceptance against a fresh, isolated local database."""
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import time
import urllib.request

root = Path(__file__).resolve().parent.parent
os.chdir(root)
for port in [8091, 5178]:
    try:
        connection = socket.create_connection(('127.0.0.1', port), timeout=0.2)
    except OSError:
        continue
    connection.close()
    sys.exit(f'Port {port} is already in use; stop the existing browser verification services.')
probe = subprocess.run(['docker', 'compose', 'exec', '-T', 'postgres', 'psql', '-U', 'tenantforge_owner', '-d', 'postgres', '-tA', '-c', "SELECT count(*) FROM pg_database WHERE datname='tenantforge_browser'"], check=True, capture_output=True, text=True)
if probe.stdout.strip() == '0':
    subprocess.run(['docker', 'compose', 'exec', '-T', 'postgres', 'psql', '-U', 'tenantforge_owner', '-d', 'postgres', '-v', 'ON_ERROR_STOP=1', '-c', 'CREATE DATABASE tenantforge_browser OWNER tenantforge_owner'], check=True)
processes = []

def interrupted(*_):
    raise KeyboardInterrupt

signal.signal(signal.SIGTERM, interrupted)
signal.signal(signal.SIGINT, interrupted)
try:
    processes.append(subprocess.Popen([sys.executable, 'scripts/browser-server.py', '--fresh'], start_new_session=True))
    environment = {key: value for key, value in os.environ.items() if not key.startswith('TF_') or key.startswith('TF_E2E_')}
    environment['VITE_API_PROXY'] = 'http://127.0.0.1:8091'
    processes.append(subprocess.Popen(['npm', 'run', 'dev'], cwd=root / 'web', env=environment, start_new_session=True))
    deadline = time.monotonic() + 90
    for origin, path in [('http://127.0.0.1:8091', '/api/v1/setup/status'), ('http://localhost:5178', '/')]:
        while True:
            if any(process.poll() is not None for process in processes):
                raise RuntimeError('Browser verification service exited before readiness')
            try:
                with urllib.request.urlopen(origin + path, timeout=1) as response:
                    if response.status == 200:
                        break
            except OSError:
                pass
            if time.monotonic() >= deadline:
                raise RuntimeError('Browser verification service readiness timed out')
            time.sleep(0.2)
    environment['TF_E2E_URL'] = 'http://localhost:5178'
    environment['TF_E2E_ALLOW_SETUP'] = '1'
    test = subprocess.Popen(['npm', 'run', 'test:e2e'], cwd=root / 'web', env=environment, start_new_session=True)
    processes.append(test)
    code = test.wait(timeout=360)
except KeyboardInterrupt:
    code = 130
except (RuntimeError, subprocess.TimeoutExpired) as error:
    print(str(error), file=sys.stderr)
    code = 1
finally:
    for process in processes:
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGTERM)
    for process in processes:
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()
sys.exit(code)
