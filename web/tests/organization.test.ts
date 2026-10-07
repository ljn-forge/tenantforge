import { expect, test } from 'vitest'
import { departmentForest } from '../src/api/organization'
test('department search retains ancestors and includes authorized nodes beyond a single list page', () => {
  const records = [
    { id: 'root', name: 'Operations', parent_id: null },
    { id: 'child', name: 'Support', parent_id: 'root' },
    { id: 'other', name: 'Finance', parent_id: null },
  ]
  const forest = departmentForest(records, 'support')
  expect(forest.map((item) => item.id)).toEqual(['root'])
  expect(forest[0]?.children.map((item) => item.id)).toEqual(['child'])
  expect(departmentForest(records).map((item) => item.id)).toEqual(['root', 'other'])
  expect(records[0]).not.toHaveProperty('children')
})
