import { afterEach, expect, test, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { useCollection } from '../src/composables/collection'
afterEach(() => vi.unstubAllGlobals())
test('a filter submitted while a list is loading runs afterward instead of leaving discarded results', async () => {
  let resolveFirst!: (response: Response) => void
  const requests: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      requests.push(url)
      return requests.length === 1
        ? new Promise<Response>((resolve) => {
            resolveFirst = resolve
          })
        : Promise.resolve(
            new Response(
              JSON.stringify({
                data: [{ id: 'latest' }],
                meta: { total: 1, page: 1, page_size: 20 },
              }),
            ),
          )
    }),
  )
  let list!: ReturnType<typeof useCollection<{ id: string }>>
  const wrapper = mount(
    defineComponent({
      setup() {
        list = useCollection<{ id: string }>(() => '/resources/tickets')
        return {}
      },
      template: '<div />',
    }),
  )
  list.query.value = 'new filter'
  void list.search()
  resolveFirst(new Response(JSON.stringify({ data: [{ id: 'stale' }] })))
  await flushPromises()
  await flushPromises()
  expect(requests).toHaveLength(2)
  expect(requests[1]).toContain('q=new+filter')
  expect(list.items.value).toEqual([{ id: 'latest' }])
  wrapper.unmount()
})
