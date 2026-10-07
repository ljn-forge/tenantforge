#!/usr/bin/env python3
"""Restore browser acceptance fixtures and prove data, files and MFA recovery.

Run after browser-check.py. Only tenantforge_browser is modified. Snapshots and
restored databases are retained for inspection; no production database is reset.
"""
import http.cookiejar
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import time
from urllib.parse import urlunsplit
import urllib.error
import urllib.request

import backup

ROOT = backup.ROOT
TABLES = ['users', 'tenants', 'memberships', 'roles', 'member_roles',
          'departments', 'resources', 'files', 'jobs', 'audit', 'security_events']


class Client:
    def __init__(self, port):
        self.origin = 'http://127.0.0.1:' + str(port)
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), urllib.request.ProxyHandler({}))
        self.csrf = self.call('/auth/csrf')['csrf_token']
        self.tenant = ''

    def call(self, path, payload=None, raw=False):
        headers = {'X-CSRF-Token': getattr(self, 'csrf', '')}
        if getattr(self, 'tenant', ''):
            headers['X-Tenant-ID'] = self.tenant
        data = None
        if payload is not None:
            data = json.dumps(payload).encode()
            headers['Content-Type'] = 'application/json'
        request = urllib.request.Request(self.origin + '/api/v1' + path, data=data, headers=headers)
        try:
            with self.opener.open(request, timeout=30) as response:
                result = response.read()
        except urllib.error.HTTPError as error:
            raise RuntimeError('Recovery API check failed: ' + path + ' HTTP ' + str(error.code)) from None
        return result if raw else json.loads(result)['data']


def totp(secret):
    # Reuse the maintained OTPAuth dependency; keep the secret off argv/logs.
    script = "const {createRequire}=require('node:module');const OTP=createRequire(process.cwd()+'/web/package.json')('otpauth');let s='';process.stdin.on('data',v=>s+=v);process.stdin.on('end',()=>process.stdout.write(new OTP.TOTP({secret:s.trim()}).generate()));"
    return subprocess.run(['node', '-e', script], input=secret, capture_output=True, text=True, check=True, cwd=ROOT).stdout


def fingerprints(database):
    result = {}
    for table in TABLES:
        result[table] = backup.sql("SELECT count(*)::text || ':' || encode(sha256(convert_to(coalesce(string_agg(row_to_json(t)::text, ',' ORDER BY row_to_json(t)::text), ''),'UTF8')),'hex') FROM " + table + ' t', database)
    return result


def start(config, log):
    environment = {key: value for key, value in os.environ.items() if not key.startswith('TF_')}
    environment.update(config)
    for key in ['TF_DB_OWNER_PASSWORD', 'TF_DB_APP_PASSWORD', 'TF_MIGRATION_DATABASE_URL', 'TF_TEST_DATABASE_URL']:
        environment.pop(key, None)
    process = subprocess.Popen([str(ROOT / '.tools/bin/tenantforge-recovery'), 'serve'], cwd=ROOT, env=environment, stdout=log, stderr=log, start_new_session=True)
    try:
        deadline = time.monotonic() + 30
        while process.poll() is None and time.monotonic() < deadline:
            try:
                port = config['TF_LISTEN'].rsplit(':', 1)[1]
                with urllib.request.build_opener(urllib.request.ProxyHandler({})).open('http://127.0.0.1:' + port + '/readyz', timeout=1) as response:
                    if response.status == 200:
                        return process
            except OSError:
                time.sleep(0.2)
        raise RuntimeError('Recovery server did not become ready; inspect the private recovery log')
    except BaseException:
        stop(process)
        raise


def stop(process):
    if process and process.poll() is None:
        os.killpg(process.pid, signal.SIGTERM)
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()


def main():
    os.umask(0o077)
    def interrupted(*_):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    for port in [8091, 8092]:
        try:
            with socket.create_connection(('127.0.0.1', port), timeout=0.2):
                raise RuntimeError('Stop existing verification servers before recovery checks')
        except OSError:
            pass
    config = backup.effective_configuration()
    parts = backup.database_url(config['TF_DATABASE_URL'])
    if parts.hostname not in ['localhost', '127.0.0.1'] or parts.port != 55439:
        raise RuntimeError('Recovery checks require the local Compose PostgreSQL service')
    config.update(TF_DATABASE_URL=urlunsplit((parts.scheme, parts.netloc, '/tenantforge_browser', parts.query, '')),
                  TF_LISTEN='127.0.0.1:8091', TF_PUBLIC_URL='http://localhost:5178',
                  TF_COOKIE_SECURE='false', TF_DATA_DIR=str(ROOT / '.data/browser'),
                  TF_STORAGE_BACKEND='local', TF_SMTP_HOST='', TF_OIDC_ISSUER='')
    for key in list(config):
        if key.startswith('TF_S3_'):
            config[key] = ''
    if backup.sql('SELECT count(*) FROM users', 'tenantforge_browser') == '0':
        raise RuntimeError('Run browser-check.py first to create acceptance fixtures')
    (ROOT / '.tools/bin').mkdir(parents=True, exist_ok=True)
    subprocess.run(['go', 'build', '-o', '.tools/bin/tenantforge-recovery', './cmd/tenantforge'], cwd=ROOT, check=True)
    stamp = time.strftime('%Y%m%d_%H%M%S', time.gmtime())
    database = 'tenantforge_restore_' + stamp
    destination = ROOT / '.backups' / ('restored-' + stamp)
    archive = ROOT / '.backups' / ('browser-' + stamp + '.tar.gz')
    archive.parent.mkdir(mode=0o700, exist_ok=True)
    source = restored = None
    enrolled = False
    codes = []
    password = os.environ.get('TF_E2E_PASSWORD', 'BrowserAcceptance-Only-2026!')
    username = os.environ.get('TF_E2E_USERNAME', 'admin')
    with (archive.parent / ('recovery-' + stamp + '.log')).open('w') as log:
        try:
            source = start(config, log)
            client = Client(8091)
            user = client.call('/auth/login', {'username': username, 'password': password})
            if user['mfa_enabled'] or user['must_change_password']:
                raise RuntimeError('Browser fixture administrator must have completed password change and disabled MFA')
            client.csrf = client.call('/auth/csrf')['csrf_token']
            secret = client.call('/auth/mfa/enroll', {'password': password})['secret']
            enrolled = True
            codes = client.call('/auth/mfa/confirm', {'totp': totp(secret)})['recovery_codes']
            client.tenant = client.call('/auth/me')['tenants'][0]['id']
            file_rows = client.call('/files')
            if not file_rows:
                raise RuntimeError('Browser fixtures must contain an uploaded file')
            file_id = file_rows[0]['id']
            content = client.call('/files/' + file_id + '/download', raw=True)
            stop(source)
            source = None
            before = fingerprints('tenantforge_browser')
            os.environ.update(config)
            backup.backup(archive, native=True)
            backup.restore(archive, database, destination)
            if fingerprints(database) != before:
                raise RuntimeError('Restored database fingerprints do not match source')
            original_files = {str(path.relative_to(ROOT / '.data/browser')): backup.sha256(path) for path in (ROOT / '.data/browser').rglob('*') if path.is_file()}
            recovered_files = {str(path.relative_to(destination / 'data')): backup.sha256(path) for path in (destination / 'data').rglob('*') if path.is_file()}
            if original_files != recovered_files:
                raise RuntimeError('Restored object checksums do not match source')
            restored = start(backup.drill_configuration(config, database, destination), log)
            recovered = Client(8092)
            recovered.call('/auth/login', {'username': username, 'password': password, 'totp': totp(secret)})
            recovered.csrf = recovered.call('/auth/csrf')['csrf_token']
            recovered.tenant = client.tenant
            if not recovered.call('/auth/me')['permissions'] or recovered.call('/files/' + file_id + '/download', raw=True) != content:
                raise RuntimeError('Restored permission or authenticated file check failed')
            recovered.call('/auth/logout', {})
            print('Recovery PASS: 11 table fingerprints, object checksums, encrypted MFA login, permissions and file download')
        finally:
            stop(restored)
            try:
                if enrolled:
                    if source is None:
                        source = start(config, log)
                    # A cancelled confirm response may already have committed.
                    if client.call('/auth/me')['user']['mfa_enabled']:
                        factor = {'recovery_code': codes[0]} if codes else {'totp': totp(secret)}
                        client.call('/auth/mfa/disable', {'password': password, **factor})
                    client.call('/auth/logout', {})
            finally:
                stop(source)


if __name__ == '__main__':
    main()
