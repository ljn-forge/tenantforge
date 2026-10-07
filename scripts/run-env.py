#!/usr/bin/env python3
"""Load project-local private dotenv, exec a command without echoing values."""
import os
from pathlib import Path
import sys

root = Path(__file__).resolve().parent.parent
arguments = sys.argv[1:]
owner = False
config = root / '.env'
explicit_config = False
while arguments and arguments[0] in ['--owner', '--env-file']:
    if arguments[0] == '--owner':
        owner = True
        arguments = arguments[1:]
    else:
        if len(arguments) < 2:
            sys.exit('--env-file requires a private configuration path.')
        config = Path(arguments[1]).resolve()
        explicit_config = True
        arguments = arguments[2:]
if not config.exists():
    sys.exit('Run python3 scripts/init-env.py first.')
for line in config.read_text().splitlines():
    if line and not line.startswith('#'):
        key, value = line.split('=', 1)
        if explicit_config:
            os.environ[key] = value
        else:
            os.environ.setdefault(key, value)
os.chdir(root)
if not owner:
    for key in ['TF_DB_OWNER_PASSWORD', 'TF_DB_APP_PASSWORD', 'TF_MIGRATION_DATABASE_URL', 'TF_TEST_DATABASE_URL']:
        os.environ.pop(key, None)
if not arguments:
    sys.exit('Usage: scripts/run-env.py [--owner] [--env-file PATH] COMMAND [ARGS...]')
os.execvp(arguments[0], arguments)
