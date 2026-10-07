<script setup lang="ts">
import { ref, watch } from 'vue'
import type { Member } from '../../api/types'
import { allPages, api } from '../../api/client'
import { t } from '../../stores/app'
import { useRequest, saved } from '../../composables/request'
const props = defineProps<{ member: Member | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { run, busy, error } = useRequest(),
  members = ref<Member[]>([]),
  owner = ref('')
watch(
  () => props.member,
  (member) => {
    owner.value = ''
    if (member)
      void run(async () => {
        members.value = await allPages<Member>('/members')
      })
  },
)
async function transfer() {
  if (!props.member) return
  await run(async () => {
    await api(`/members/${props.member!.id}/transfer`, {
      method: 'POST',
      body: { owner_id: owner.value },
    })
    saved()
    emit('saved')
    emit('close')
  })
}
</script>
<template>
  <el-dialog
    :model-value="!!member"
    :title="t('转移负责的记录', 'Transfer owned records')"
    width="520px"
    @close="emit('close')"
  >
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <p>
      {{
        t(
          `将${member?.display_name ?? ''}负责的记录转移给：`,
          `Move records owned by ${member?.display_name ?? ''} to:`,
        )
      }}
    </p>
    <el-form @submit.prevent="transfer" label-position="top">
      <el-form-item :label="t('新的负责人', 'New owner')">
        <el-select v-model="owner" filterable required>
          <el-option
            v-for="item in members.filter(
              (item) => item.status === 'active' && item.id !== member?.id,
            )"
            :key="item.id"
            :value="item.user_id"
            :label="item.display_name + ' · ' + item.email"
          />
        </el-select>
      </el-form-item>
      <p class="hint">
        {{
          t(
            '此操作会批量变更记录归属，请确认接收用户。',
            'This changes ownership across records. Verify the recipient.',
          )
        }}
      </p>
      <div class="dialog-actions">
        <el-button @click="emit('close')">{{ t('取消', 'Cancel') }}</el-button>
        <el-button type="primary" native-type="submit" :disabled="!owner" :loading="busy">
          {{ t('确认转移', 'Transfer records') }}
        </el-button>
      </div>
    </el-form>
  </el-dialog>
</template>
