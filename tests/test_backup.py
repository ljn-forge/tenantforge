import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import os
from types import SimpleNamespace

spec = importlib.util.spec_from_file_location('backup', Path(__file__).resolve().parents[1] / 'scripts/backup.py')
backup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backup)

class BackupTests(unittest.TestCase):
    def test_connection_target_overrides_cannot_escape_restore_isolation(self):
        for query in ['dbname=tenantforge', 'database=tenantforge', 'host=remote', 'hostaddr=10.0.0.1', 'port=5433', 'service=live', '%64bname=tenantforge', 'sslmode=disable&dbname=tenantforge', ' dbname =tenantforge', '%20host%20=remote']:
            with self.subTest(query=query):
                config = {'TF_DATABASE_URL': 'postgresql://127.0.0.1:55439/tenantforge?' + query}
                with self.assertRaisesRegex(ValueError, 'connection-target query overrides'):
                    backup.drill_configuration(config, 'tenantforge_restore_safe', Path('/restored'))
        restored = backup.drill_configuration({'TF_DATABASE_URL': 'postgresql://runtime@postgres:5432/tenantforge?sslmode=disable&pool_max_conns=2'}, 'tenantforge_restore_safe', Path('/restored'))
        parsed = backup.database_url(restored['TF_DATABASE_URL'])
        self.assertEqual(parsed.path, '/tenantforge_restore_safe')
        self.assertEqual(parsed.hostname, '127.0.0.1')
    def test_restore_drill_cannot_contact_live_integrations(self):
        config = backup.drill_configuration({'TF_COOKIE_SECURE': 'true', 'TF_SMTP_HOST': 'live-mail', 'TF_S3_BUCKET': 'live-bucket', 'TF_S3_SECRET_KEY': 'live-key', 'TF_STORAGE_BACKEND': 's3', 'TF_OIDC_ISSUER': 'live-idp'}, 'tenantforge_restore_drill', Path('/drill'))
        self.assertEqual(config['TF_COOKIE_SECURE'], 'false')
        self.assertEqual(config['TF_STORAGE_BACKEND'], 'local')
        self.assertEqual(config['TF_SMTP_HOST'], '')
        self.assertEqual(config['TF_S3_BUCKET'], '')
        self.assertEqual(config['TF_OIDC_ISSUER'], '')

    def test_native_snapshot_uses_effective_data_and_master_key(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            custom = root / 'custom-objects'
            custom.mkdir()
            (custom / 'object').write_bytes(b'actual object')
            (root / '.env').write_text('TF_DATA_DIR=missing\nTF_MASTER_KEY=file-key\nTF_DATABASE_URL=postgresql://127.0.0.1:55439/tenantforge\n')
            def fake_command(arguments, **kwargs):
                if 'pg_dump' in arguments:
                    kwargs['stdout'].write(b'database dump')
                return SimpleNamespace(stdout=b'postgres\n')
            with patch.object(backup, 'ROOT', root), patch.object(backup, 'sql', return_value='0'), patch.object(backup, 'command', side_effect=fake_command), patch.dict(os.environ, {'TF_DATA_DIR': str(custom), 'TF_MASTER_KEY': 'effective-key'}, clear=True):
                backup.backup(root / 'snapshot.tar.gz', native=True)
            backup.unpack_verified(root / 'snapshot.tar.gz', root / 'restored')
            self.assertEqual((root / 'restored/data/object').read_bytes(), b'actual object')
            self.assertIn('TF_MASTER_KEY=effective-key', (root / 'restored/environment.private').read_text())

    def test_remote_database_snapshot_fails_closed(self):
        with patch.object(backup, 'effective_configuration', return_value={'TF_DATABASE_URL': 'postgresql://remote-host:5432/tenantforge'}):
            with self.assertRaisesRegex(ValueError, 'external databases'):
                backup.backup('/unused/snapshot.tar.gz', native=True)

    def test_snapshot_publication_does_not_replace_concurrent_file(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / 'objects').mkdir()
            target = root / 'snapshot.tar.gz'
            def fake_command(arguments, **kwargs):
                if 'pg_dump' in arguments:
                    kwargs['stdout'].write(b'dump')
                    target.write_bytes(b'concurrent snapshot')
                return SimpleNamespace(stdout=b'postgres\n')
            config = {'TF_DATABASE_URL': 'postgresql://127.0.0.1:55439/tenantforge', 'TF_DATA_DIR': str(root / 'objects')}
            with patch.object(backup, 'ROOT', root), patch.object(backup, 'effective_configuration', return_value=config), patch.object(backup, 'command', side_effect=fake_command), patch.object(backup, 'sql', return_value='0'):
                with self.assertRaises(FileExistsError):
                    backup.backup(target, native=True)
            self.assertEqual(target.read_bytes(), b'concurrent snapshot')

    def test_incomplete_restore_never_creates_database(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / 'source'
            source.mkdir()
            (source / 'database.dump').write_bytes(b'dump')
            (source / 'manifest.json').write_text(json.dumps({'format': 1, 'files': {'database.dump': hashlib.sha256(b'dump').hexdigest()}}))
            archive = root / 'snapshot.tar.gz'
            with tarfile.open(archive, 'w:gz') as output:
                output.add(source, arcname='.')
            with patch.object(backup, 'sql', return_value='0') as sql:
                with self.assertRaisesRegex(ValueError, 'missing database or private configuration'):
                    backup.restore(archive, 'tenantforge_restore_incomplete', root / 'restore')
                self.assertFalse(any(call.args[0].startswith('CREATE') for call in sql.call_args_list))

    def test_container_snapshot_reads_actual_container_configuration(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            def fake_command(arguments, **kwargs):
                if 'inspect' in arguments:
                    return SimpleNamespace(stdout=json.dumps(['TF_DATABASE_URL=postgresql://runtime@postgres:5432/tenantforge', 'TF_MASTER_KEY=container-key', 'TF_DATA_DIR=/custom/data']).encode())
                if 'pg_dump' in arguments:
                    kwargs['stdout'].write(b'database dump')
                if 'cp' in arguments:
                    self.assertEqual(arguments[-2], 'container-id:/custom/data/.')
                    (Path(arguments[-1]) / 'object').write_bytes(b'container object')
                return SimpleNamespace(stdout=b'container-id\n' if '-q' in arguments else b'postgres\n')
            with patch.object(backup, 'effective_configuration', return_value={'TF_MASTER_KEY': 'caller-key'}), patch.object(backup, 'command', side_effect=fake_command), patch.object(backup, 'sql', return_value='0'):
                backup.backup(root / 'snapshot.tar.gz')
            backup.unpack_verified(root / 'snapshot.tar.gz', root / 'restored')
            self.assertIn('TF_MASTER_KEY=container-key', (root / 'restored/environment.private').read_text())

    def test_archive_rejects_escape(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            archive = root / 'malicious.tar.gz'
            with tarfile.open(archive, 'w:gz') as output:
                entry = tarfile.TarInfo('../escaped')
                entry.size = 4
                output.addfile(entry, io.BytesIO(b'evil'))
            with self.assertRaises(ValueError):
                backup.unpack_verified(archive, root / 'restore')
            self.assertFalse((root / 'escaped').exists())

    def test_integrity_checks_archive_before_database_restore(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / 'source'
            source.mkdir()
            (source / 'database.dump').write_bytes(b'changed content')
            (source / 'manifest.json').write_text(json.dumps({'format': 1, 'files': {'database.dump': hashlib.sha256(b'original').hexdigest()}}))
            archive = root / 'corrupt.tar.gz'
            with tarfile.open(archive, 'w:gz') as output:
                output.add(source, arcname='.')
            with self.assertRaises(ValueError):
                backup.unpack_verified(archive, root / 'restore')

    def test_valid_archive_checks_every_file(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / 'source'
            source.mkdir()
            (source / 'database.dump').write_bytes(b'dump')
            (source / 'manifest.json').write_text(json.dumps({'format': 1, 'files': {'database.dump': hashlib.sha256(b'dump').hexdigest()}}))
            archive = root / 'valid.tar.gz'
            with tarfile.open(archive, 'w:gz') as output:
                output.add(source, arcname='.')
            backup.unpack_verified(archive, root / 'restore')
            self.assertEqual((root / 'restore/database.dump').read_bytes(), b'dump')
