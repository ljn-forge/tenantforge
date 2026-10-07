"""Keep the published REST surface aligned with the registered server routes."""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parent.parent


class APIContractTests(unittest.TestCase):
    def test_every_registered_api_operation_is_documented_without_stale_routes(self):
        source = (ROOT / 'internal/platform/app.go').read_text()
        registered = set(re.findall(r'add\("(GET|POST|PATCH|DELETE|PUT) /api/v1([^" ]+)"', source))
        contract = json.loads((ROOT / 'docs/openapi.json').read_text())
        documented = {
            (method.upper(), path)
            for path, operations in contract['paths'].items()
            for method in operations
            if method in {'get', 'post', 'patch', 'delete', 'put'}
        }
        self.assertGreater(len(registered), 50, 'route extraction did not find the API')
        self.assertEqual(registered, documented)

    def test_row_authority_and_optional_workload_are_explicit_in_contract(self):
        contract = json.loads((ROOT / 'docs/openapi.json').read_text())
        schemas = contract['components']['schemas']
        self.assertIn('capabilities', schemas['Record']['required'])
        self.assertEqual(set(schemas['Capabilities']['required']), {'actions', 'write_fields', 'transitions'})
        self.assertFalse(schemas['Capabilities']['additionalProperties'])
        self.assertFalse(schemas['Workload'].get('required'), 'unauthorized KPI fields must be omittable')


if __name__ == '__main__':
    unittest.main()
