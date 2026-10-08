import { expect, test } from 'vitest'
import { activitySentence, operationLabel } from '../src/api/presentation'
import { state } from '../src/stores/app'
import type { Module } from '../src/api/types'
test('support reply operation has a human label in both locales', () => {
  state.locale = 'zh'
  expect(operationLabel('reply')).toBe('记录沟通与备注')
  state.locale = 'en'
  expect(operationLabel('reply')).toBe('Reply')
})

test('activity explains the recorded operation using module labels without inventing an actor name', () => {
  const modules = [{ key: 'assets', label: '资产', label_en: 'Assets' }] as Module[]
  state.locale = 'en'
  const event = { resource: 'assets', action: 'create', actor_id: 'raw-actor-id' }
  expect(activitySentence(event, modules)).toBe('Created a record in Assets')
  expect(activitySentence(event, modules)).not.toContain('raw-actor-id')
  state.locale = 'zh'
  expect(activitySentence(event, modules)).toBe('新建了资产记录')
})

test('control resources have human labels even when registry labels are canonical keys', async () => {
  const { resourceLabel } = await import('../src/api/presentation')
  const modules = [{ key: 'members', label: 'members', label_en: 'members' }] as Module[]
  state.locale = 'zh'
  expect(resourceLabel('members', modules)).toBe('用户')
  state.locale = 'en'
  expect(resourceLabel('members', modules)).toBe('Users')
})
