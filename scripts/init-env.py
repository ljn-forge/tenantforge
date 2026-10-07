#!/usr/bin/env python3
"""Create private local configuration once; never print generated secrets."""
import base64
import os
from pathlib import Path
import secrets

ROOT = Path(__file__).resolve().parent.parent
target = ROOT / '.env'
owner = secrets.token_hex(24)
app = secrets.token_hex(24)
master = base64.b64encode(secrets.token_bytes(32)).decode()
content = f'''# Private local development configuration. Do not commit.
TF_DB_OWNER_PASSWORD={owner}
TF_DB_APP_PASSWORD={app}
TF_DATABASE_URL=postgresql://tenantforge_app:{app}@127.0.0.1:55439/tenantforge?sslmode=disable
TF_MIGRATION_DATABASE_URL=postgresql://tenantforge_owner:{owner}@127.0.0.1:55439/tenantforge?sslmode=disable
TF_TEST_DATABASE_URL=postgresql://tenantforge_owner:{owner}@127.0.0.1:55439/tenantforge_test?sslmode=disable
TF_MASTER_KEY={master}
TF_LISTEN=127.0.0.1:8090
TF_PUBLIC_URL=http://localhost:5178
TF_COOKIE_SECURE=false
TF_DATA_DIR=.data
TF_SMTP_HOST=127.0.0.1
TF_SMTP_PORT=1039
TF_SMTP_FROM=tenantforge@localhost
'''
try:
    descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
    print('Existing .env preserved.')
else:
    with os.fdopen(descriptor, 'w') as output:
        output.write(content)
    print('Private .env created. Secrets are not displayed.')
