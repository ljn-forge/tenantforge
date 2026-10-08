"""Exercise compressed loading budgets against Vite's real manifest structure."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
SPEC = importlib.util.spec_from_file_location('web_budget', ROOT / 'scripts/check-web-budget.py')


class WebBudgetTests(unittest.TestCase):
    def setUp(self):
        self.module = importlib.util.module_from_spec(SPEC)
        SPEC.loader.exec_module(self.module)

    def test_static_closure_deduplicates_shared_files_and_ignores_unvisited_routes(self):
        manifest = {
            'index.html': {'file': 'index.js', 'imports': ['shared'], 'dynamicImports': ['jobs']},
            'login': {'file': 'login.js', 'imports': ['shared'], 'css': ['login.css']},
            'shared': {'file': 'shared.js', 'css': ['shared.css']},
            'jobs': {'file': 'jobs.js', 'css': ['jobs.css']},
        }
        self.assertEqual(self.module.initial_files(manifest, ['index.html', 'login']), {
            'index.js', 'login.js', 'shared.js', 'shared.css', 'login.css',
        })

    def test_missing_chunk_is_an_error_instead_of_an_underestimated_budget(self):
        with self.assertRaises(ValueError):
            self.module.initial_files({'index.html': {'file': 'index.js', 'imports': ['missing']}}, ['index.html'])

    def test_budget_uses_sum_of_individually_compressed_resources(self):
        with tempfile.TemporaryDirectory() as directory:
            dist = Path(directory)
            (dist / 'index.js').write_bytes(b'a' * 5000)
            (dist / 'index.css').write_bytes(b'b' * 3000)
            result = self.module.measure(dist, {'index.js', 'index.css'})
            self.assertLess(result['js'], 100)
            self.assertLess(result['css'], 100)
            self.assertTrue(self.module.within_budget(result, 100, 100))
            self.assertFalse(self.module.within_budget(result, 10, 100))
            self.assertFalse(self.module.within_budget(result, 100, 10))


if __name__ == '__main__':
    unittest.main()
