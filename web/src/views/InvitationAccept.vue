<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api/client'
import { t, loggedIn } from '../stores/app'
import { useRequest } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
const route = useRoute()
const router = useRouter()
const token = ref(typeof route.query.token === 'string' ? route.query.token : '')
const { run, busy, error } = useRequest()
const valid = computed(() => !!token.value.trim())
async function accept() {
  await run(async () => {
    await api('/invitations/accept', { method: 'POST', body: { token: token.value } })
    token.value = ''
    await loggedIn()
    await router.push('/')
  })
}
</script>
<template>
  <PageHeader
    :title="t('加入组织', 'Join a workspace')"
    :description="
      t(
        '请确认你登录的邮箱与邀请接收邮箱一致。',
        'Sign in with the email address that received the invitation.',
      )
    "
  />
  <div class="panel narrow-panel">
    <el-alert v-if="error" type="error" :title="error" :closable="false" />
    <el-form @submit.prevent="accept" label-position="top">
      <el-form-item :label="t('邀请凭据', 'Invitation token')">
        <el-input v-model="token" type="password" required autocomplete="off" />
      </el-form-item>
      <el-button type="primary" native-type="submit" :disabled="!valid" :loading="busy">
        {{ t('接受邀请', 'Accept invitation') }}
      </el-button>
    </el-form>
  </div>
</template>
