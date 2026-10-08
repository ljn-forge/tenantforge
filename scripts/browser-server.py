#!/usr/bin/env python3
"""Start the backend against the pre-created isolated browser verification DB."""
import argparse
import os
from pathlib import Path
import subprocess
import sys
from urllib.parse import parse_qsl, urlsplit, urlunsplit

root = Path(__file__).resolve().parent.parent
os.chdir(root)
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--fresh', action='store_true', help='Reset only the isolated browser fixtures after checking active connections.')
parser.add_argument('--assets', action='store_true', help='Use the public assets entrypoint in a separate extension browser database.')
arguments = parser.parse_args()
database = 'tenantforge_browser_assets' if arguments.assets else 'tenantforge_browser'
entrypoint = './examples/assets/cmd' if arguments.assets else './cmd/tenantforge'
for line in (root / '.env').read_text().splitlines():
    if line and not line.startswith('#'):
        key, value = line.split('=', 1)
        os.environ.setdefault(key, value)
for key in ['TF_DATABASE_URL', 'TF_MIGRATION_DATABASE_URL']:
    parts = urlsplit(os.environ[key])
    forbidden = {'host', 'hostaddr', 'port', 'dbname', 'database', 'service', 'servicefile', 'user', 'password'}
    if any(name.strip().lower() in forbidden for name, _ in parse_qsl(parts.query, keep_blank_values=True)):
        sys.exit('Isolated browser verification rejects database connection-target query overrides.')
    os.environ[key] = urlunsplit((parts.scheme, parts.netloc, '/' + database, parts.query, parts.fragment))
os.environ['TF_LISTEN'] = '127.0.0.1:8091'
os.environ['TF_PUBLIC_URL'] = 'http://localhost:5178'
os.environ['TF_DATA_DIR'] = str(root / ('.data/browser_assets' if arguments.assets else '.data/browser'))
subprocess.run(['go', 'run', entrypoint, 'migrate'], check=True)
if arguments.fresh:
    check = subprocess.run(['docker', 'compose', 'exec', '-T', 'postgres', 'psql', '-U', 'tenantforge_owner', '-d', 'postgres', '-tA', '-c', f"SELECT count(*) FROM pg_stat_activity WHERE datname='{database}' AND backend_type='client backend' AND pid<>pg_backend_pid()"], check=True, capture_output=True, text=True)
    if check.stdout.strip() != '0':
        sys.exit('Stop the existing isolated browser server before resetting its fixtures.')
    subprocess.run(['docker', 'compose', 'exec', '-T', 'postgres', 'psql', '-U', 'tenantforge_owner', '-d', database, '-v', 'ON_ERROR_STOP=1', '-c', 'TRUNCATE users,tenants,mail_outbox,login_attempts,oidc_flows CASCADE; UPDATE platform_state SET initialized=false'], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
for key in ['TF_DB_OWNER_PASSWORD', 'TF_DB_APP_PASSWORD', 'TF_MIGRATION_DATABASE_URL', 'TF_TEST_DATABASE_URL']:
    os.environ.pop(key, None)
os.execvp('go', ['go', 'run', entrypoint, 'serve'])
