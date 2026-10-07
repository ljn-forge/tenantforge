import { expect, test } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import RoleEditor from '../src/components/RoleEditor.vue'
import { state } from '../src/stores/app'
import type { Grant, Module } from '../src/api/types'
const descriptor: Module = {
  key: 'tickets',
  label: '工单',
  label_en: 'Tickets',
  description: '',
  actions: ['read', 'create', 'update'],
  fields: [
    { key: 'title', label: '标题', label_en: 'Title', type: 'string' },
    { key: 'secret', label: '秘密', label_en: 'Secret', type: 'string', sensitive: true },
  ],
}
test('matrix cell keeps multiple scope grants independent and preset preview never changes existing grants before confirmation', async () => {
  state.locale = 'en'
  const grants: Grant[] = [
    { resource: 'tickets', action: 'read', scope: 'department', read_fields: ['title'] },
    {
      resource: 'tickets',
      action: 'read',
      scope: 'self',
      read_fields: ['secret'],
      unmask_fields: ['secret'],
    },
  ]
  const original = JSON.stringify(grants)
  const wrapper = mount(RoleEditor, {
    props: { modelValue: grants, modules: [descriptor], departments: [] },
    global: { plugins: [ElementPlus] },
  })
  await flushPromises()
  const cell = wrapper.find('button[aria-label="Tickets · Read"]')
  expect(cell.exists()).toBe(true)
  await cell.trigger('click')
  await flushPromises()
  expect(wrapper.findAll('[data-grant]')).toHaveLength(2)
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Own-work preset')!
    .trigger('click')
  await flushPromises()
  expect(wrapper.text()).toContain('Review added access')
  expect(JSON.stringify(grants)).toBe(original)
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Apply preset')!
    .trigger('click')
  await flushPromises()
  const result = wrapper.emitted('update:modelValue')!.at(-1)![0] as Grant[]
  expect(result.slice(0, 2)).toEqual(grants)
  expect(result.slice(2).map((grant) => [grant.action, grant.scope])).toEqual([
    ['create', 'self'],
    ['update', 'self'],
  ])
  expect(result.slice(2).every((grant) => !grant.unmask_fields?.length)).toBe(true)
  wrapper.unmount()
})

test('preset preview shows preserved broader authority and the proposed additions before changing anything', async () => {
  state.locale = 'en'
  const grants: Grant[] = [
    { resource: 'tickets', action: 'update', scope: 'tenant', write_fields: ['title'] },
  ]
  const wrapper = mount(RoleEditor, {
    props: { modelValue: grants, modules: [descriptor], departments: [] },
    global: { plugins: [ElementPlus] },
  })
  await flushPromises()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Own-work preset')!
    .trigger('click')
  await flushPromises()
  expect(wrapper.find('.preset-preview').text()).toContain('Entire workspace')
  expect(wrapper.find('.preset-preview').text()).toContain('Preserved')
  expect(wrapper.find('.preset-preview').text()).toContain('Added')
  expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  wrapper.unmount()
})
