#!/usr/bin/env python3
"""Private offline snapshots and non-destructive restore to a new database.

Uses PostgreSQL pg_dump/pg_restore and Python's safe tar extraction; does not
implement a database backup or cryptographic format.
"""
import argparse
import base64
from datetime import datetime, timezone
import hashlib
import fcntl
import json
import os
from pathlib import Path
from pathlib import PurePosixPath
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
from urllib.parse import parse_qsl, urlsplit, urlunsplit

ROOT = Path(__file__).resolve().parent.parent
COMPOSE = ['docker', 'compose']

def database_url(value):
    parts = urlsplit(value)
    forbidden = {'host', 'hostaddr', 'port', 'dbname', 'database', 'service', 'servicefile', 'user', 'password'}
    if any(key.strip().lower() in forbidden for key, _ in parse_qsl(parts.query, keep_blank_values=True)):
        raise ValueError('Database URL must not contain connection-target query overrides')
    return parts

def command(arguments, **kwargs):
    result = subprocess.run(arguments, cwd=ROOT, stderr=subprocess.PIPE, **kwargs)
    if result.returncode:
        raise RuntimeError('Backup/restore command failed; check the local service state and configuration.')
    return result

def sql(query, database='tenantforge'):
    return command(COMPOSE + ['exec', '-T', 'postgres', 'psql', '-v', 'ON_ERROR_STOP=1', '-U', 'tenantforge_owner', '-d', database, '-tA', '-c', query], stdout=subprocess.PIPE).stdout.decode().strip()

def sha256(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()

def effective_configuration():
    config = {}
    for line in (ROOT / '.env').read_text().splitlines():
        if line and not line.startswith('#'):
            key, value = line.split('=', 1)
            config[key] = value
    config.update({key: value for key, value in os.environ.items() if key.startswith('TF_')})
    return config

def drill_configuration(config, database, destination):
    config = dict(config)
    for key in ['TF_DATABASE_URL', 'TF_MIGRATION_DATABASE_URL', 'TF_TEST_DATABASE_URL']:
        if key in config:
            parts = database_url(config[key])
            credentials = parts.netloc.rsplit('@', 1)[0] + '@' if '@' in parts.netloc else ''
            config[key] = urlunsplit((parts.scheme, credentials + '127.0.0.1:55439', '/' + database, parts.query, parts.fragment))
    config.update(TF_DATA_DIR=str(destination / 'data'), TF_LISTEN='127.0.0.1:8092',
                  TF_PUBLIC_URL='http://localhost:8092', TF_COOKIE_SECURE='false',
                  TF_STORAGE_BACKEND='local', TF_SMTP_HOST='', TF_CONTAINER_SMTP_HOST='',
                  TF_OIDC_ISSUER='', TF_OIDC_CLIENT_ID='', TF_OIDC_CLIENT_SECRET='')
    for key in list(config):
        if key.startswith('TF_S3_') or key in ['TF_SMTP_USER', 'TF_SMTP_PASSWORD']:
            config[key] = ''
    return config

def unpack_verified(archive, destination):
    destination = Path(destination)
    destination.mkdir(mode=0o700, parents=True, exist_ok=False)
    try:
        with tarfile.open(archive, 'r:gz') as source:
            for member in source.getmembers():
                resolved = (destination / member.name).resolve()
                if not resolved.is_relative_to(destination.resolve()) or member.issym() or member.islnk():
                    raise ValueError('Unsafe archive member')
                if not (member.isfile() or member.isdir()):
                    raise ValueError('Unsupported archive member')
            source.extractall(destination, filter='data')
        manifest = json.loads((destination / 'manifest.json').read_text())
        if manifest.get('format') != 1 or not isinstance(manifest.get('files'), dict):
            raise ValueError('Unsupported snapshot format')
        actual = {str(path.relative_to(destination)) for path in destination.rglob('*') if path.is_file() and path != destination / 'manifest.json'}
        if actual != set(manifest['files']):
            raise ValueError('Snapshot file inventory mismatch')
        for relative, expected in manifest['files'].items():
            path = (destination / relative).resolve()
            if not path.is_relative_to(destination.resolve()) or sha256(path) != expected:
                raise ValueError('Snapshot checksum mismatch')
        return manifest
    except (tarfile.TarError, OSError, KeyError, json.JSONDecodeError) as error:
        raise ValueError('Invalid snapshot archive') from error

def backup(output, native=False):
    lock_directory = ROOT / '.backups'
    lock_directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    with (lock_directory / '.snapshot.lock').open('a') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as error:
            raise ValueError('Another snapshot is in progress') from error
        return capture_snapshot(output, native)

def capture_snapshot(output, native=False):
    config = effective_configuration()
    container = '' if native else command(COMPOSE + ['ps', '-a', '-q', 'app'], stdout=subprocess.PIPE).stdout.decode().strip()
    if not native:
        if not container:
            raise ValueError('No application container found; use --native for native deployment')
        environment = json.loads(command(['docker', 'inspect', '--format', '{{json .Config.Env}}', container], stdout=subprocess.PIPE).stdout)
        actual = dict(entry.split('=', 1) for entry in environment if entry.startswith('TF_'))
        config.update(actual)
    source = database_url(config.get('TF_DATABASE_URL', ''))
    allowed_hosts = {'127.0.0.1', 'localhost'} if native else {'postgres'}
    if source.hostname not in allowed_hosts or source.port != (55439 if native else 5432):
        raise ValueError('Snapshot only supports this Compose PostgreSQL service; external databases require their own backup tools')
    source_database = source.path.removeprefix('/')
    if not re.fullmatch(r'tenantforge(?:_[a-z0-9_]{1,50})?', source_database):
        raise ValueError('Compose snapshot requires a tenantforge database name')
    output = Path(output).resolve()
    if output.exists():
        raise ValueError('Output already exists; choose a new snapshot filename')
    output.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    running = command(COMPOSE + ['ps', '--status', 'running', '--services'], stdout=subprocess.PIPE).stdout.decode().splitlines()
    restart = not native and 'app' in running
    if restart:
        command(COMPOSE + ['stop', 'app'], stdout=subprocess.PIPE)
    temporary_archive = None
    try:
        if sql("SELECT count(*) FROM pg_stat_activity WHERE datname='" + source_database + "' AND backend_type='client backend' AND pid<>pg_backend_pid()") != '0':
            raise ValueError('Stop the native TenantForge server before taking an offline snapshot')
        with tempfile.TemporaryDirectory(prefix='tenantforge-snapshot-', dir=output.parent) as temporary:
            stage = Path(temporary)
            with (stage / 'database.dump').open('wb') as stream:
                command(COMPOSE + ['exec', '-T', 'postgres', 'pg_dump', '-U', 'tenantforge_owner', '--format=custom', source_database], stdout=stream)
            (stage / 'environment.private').write_text('\n'.join(key + '=' + value for key, value in config.items()) + '\n')
            os.chmod(stage / 'environment.private', 0o600)
            data = stage / 'data'
            data.mkdir(mode=0o700)
            if native:
                native_data = Path(config.get('TF_DATA_DIR') or './data')
                if not native_data.is_absolute():
                    native_data = ROOT / native_data
                if not native_data.is_dir():
                    raise ValueError('Configured native data directory does not exist')
                if output.parent.is_relative_to(native_data.resolve()):
                    raise ValueError('Snapshot output cannot be inside the captured data directory')
                shutil.copytree(native_data, data, dirs_exist_ok=True, symlinks=False)
            elif container:
                configured = PurePosixPath(config.get('TF_DATA_DIR') or './data')
                if not configured.is_absolute():
                    configured = PurePosixPath('/app') / configured
                command(['docker', 'cp', container + ':' + str(configured) + '/.', str(data)], stdout=subprocess.PIPE)
            else:
                raise ValueError('No application container found; use --native for native deployment')
            manifest = {
                'format': 1,
                'created_at': datetime.now(timezone.utc).isoformat(),
                'database': source_database,
                'files': {str(path.relative_to(stage)): sha256(path) for path in stage.rglob('*') if path.is_file()},
            }
            (stage / 'manifest.json').write_text(json.dumps(manifest, indent=2))
            descriptor, temporary_archive = tempfile.mkstemp(prefix='.snapshot-', suffix='.tar.gz', dir=output.parent)
            os.close(descriptor)
            with tarfile.open(temporary_archive, 'w:gz') as archive:
                archive.add(stage, arcname='.')
            os.link(temporary_archive, output)
            Path(temporary_archive).unlink()
            temporary_archive = None
        print('Private snapshot created:', output)
    finally:
        if temporary_archive:
            Path(temporary_archive).unlink(missing_ok=True)
        if restart:
            command(COMPOSE + ['start', 'app'], stdout=subprocess.PIPE)

def restore(archive, database, destination):
    if not re.fullmatch(r'tenantforge_restore_[a-z0-9_]{1,40}', database):
        raise ValueError('Restore database must be a new tenantforge_restore_* name')
    if sql("SELECT count(*) FROM pg_database WHERE datname='" + database + "'", 'postgres') != '0':
        raise ValueError('Restore never overwrites an existing database')
    destination = Path(destination).resolve()
    unpack_verified(archive, destination)
    config = {}
    if not (destination / 'database.dump').is_file() or not (destination / 'environment.private').is_file():
        raise ValueError('Snapshot is missing database or private configuration')
    for line in (destination / 'environment.private').read_text().splitlines():
        if line and not line.startswith('#'):
            key, value = line.split('=', 1)
            config[key] = value
    try:
        key = base64.b64decode(config.get('TF_MASTER_KEY', ''), validate=True)
    except ValueError as error:
        raise ValueError('Snapshot master key invalid') from error
    if len(key) != 32 or not config.get('TF_DATABASE_URL'):
        raise ValueError('Snapshot configuration incomplete')
    config = drill_configuration(config, database, destination)
    sql('CREATE DATABASE "' + database + '" OWNER tenantforge_owner', 'postgres')
    with (destination / 'database.dump').open('rb') as stream:
        command(COMPOSE + ['exec', '-T', 'postgres', 'pg_restore', '-U', 'tenantforge_owner', '--exit-on-error', '--dbname', database], stdin=stream, stdout=subprocess.PIPE)
    target = destination / '.env'
    descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, 'w') as stream:
        stream.write('\n'.join(key + '=' + value for key, value in config.items()) + '\n')
    print('Restored into a new isolated database:', database)
    print('Private restored configuration:', target)

def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', default='.backups/tenantforge-' + datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '.tar.gz')
    parser.add_argument('--native', action='store_true', help='Snapshot stopped native server .data instead of container storage')
    parser.add_argument('--restore', type=Path)
    parser.add_argument('--database')
    parser.add_argument('--destination', type=Path)
    args = parser.parse_args()
    try:
        if args.restore:
            if not args.database or not args.destination:
                parser.error('--restore requires --database and --destination')
            restore(args.restore, args.database, args.destination)
        else:
            backup(args.output, args.native)
    except (ValueError, RuntimeError, OSError) as error:
        sys.exit(str(error))

if __name__ == '__main__':
    main()
