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

    def test_job_catalog_and_safe_display_metadata_have_closed_schemas(self):
        contract = json.loads((ROOT / 'docs/openapi.json').read_text())
        self.assertTrue('/job-types' in contract['paths'], 'extension job catalog is missing')
        schemas = contract['components']['schemas']
        self.assertEqual(schemas['Job']['properties']['definition']['$ref'], '#/components/schemas/JobPresentation')
        self.assertFalse(schemas['JobPresentation']['additionalProperties'])
        self.assertNotIn('payload', schemas['JobPresentation']['properties'])
        self.assertNotIn('input_fields', schemas['JobPresentation']['properties'])
        self.assertEqual(set(schemas['JobType']['required']), {
            'type', 'label', 'label_en', 'resource', 'action', 'input_fields', 'result_fields',
        })
        self.assertFalse(schemas['JobType']['additionalProperties'])


    def test_support_activity_and_independent_assignment_are_explicit(self):
        contract = json.loads((ROOT / 'docs/openapi.json').read_text())
        schemas = contract['components']['schemas']
        self.assertEqual(schemas['Capabilities']['properties']['reply_kinds']['items']['enum'], ['reply', 'note'])
        self.assertNotIn('reply_kinds', schemas['Capabilities']['required'])
        for name in ['TicketEvent', 'TicketReplyInput', 'TicketReplyResult', 'TicketAssignmentInput']:
            self.assertFalse(schemas[name]['additionalProperties'])
        self.assertEqual(schemas['TicketReplyInput']['properties']['body']['maxLength'], 4000)
        self.assertEqual(set(schemas['TicketReplyInput']['required']), {'version', 'kind', 'body'})
        assignment = contract['paths']['/resources/{module}/{id}/assignment']['post']
        self.assertIn('tickets:update is not required', assignment['description'])
        activity = contract['paths']['/resources/{module}/{id}/activity']['get']
        self.assertEqual(activity['responses']['200']['content']['application/json']['schema']['properties']['meta']['$ref'], '#/components/schemas/Meta')
        transition = contract['paths']['/resources/{module}/{id}/transition']['post']
        self.assertIn('resolution', transition['requestBody']['content']['application/json']['schema']['properties'])


if __name__ == '__main__':
    unittest.main()
