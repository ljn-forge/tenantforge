import type { Department } from './types'
export interface TreeDepartment extends Department {
  children: TreeDepartment[]
}
export function departmentForest(records: Department[], query = ''): TreeDepartment[] {
  const map = new Map<string, TreeDepartment>(
    records.map((item) => [item.id, { ...item, children: [] }]),
  )
  const included = new Set<string>()
  const normalized = query.trim().toLocaleLowerCase()
  for (const record of records) {
    if (normalized && !record.name.toLocaleLowerCase().includes(normalized)) continue
    let node: TreeDepartment | undefined = map.get(record.id)
    while (node && !included.has(node.id)) {
      included.add(node.id)
      node = node.parent_id ? map.get(node.parent_id) : undefined
    }
  }
  const roots: TreeDepartment[] = []
  map.forEach((item) => {
    if (!included.has(item.id)) return
    if (item.parent_id && included.has(item.parent_id)) map.get(item.parent_id)!.children.push(item)
    else roots.push(item)
  })
  return roots
}
