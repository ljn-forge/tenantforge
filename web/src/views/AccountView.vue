<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'
import { state, t, loadMe } from '../stores/app'
import { dateTime } from '../api/forms'
import { operationLabel } from '../api/presentation'
import { useCollection } from '../composables/collection'
import { useRequest, saved, confirmDelete } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
interface Session {
  id: string
  created_at: string
  last_seen_at: string
  expires_at: string
  current: boolean
}
interface SecurityEvent {
  id: string
  action: string
  request_id: string
  changes: Record<string, unknown>
  created_at: string
}
const requiredPassword = computed(() => !!state.me?.user.must_change_password)
const securityEvents = useCollection<SecurityEvent>(
  () => '/auth/security-events',
  () => !requiredPassword.value,
)
const router = useRouter()
const { run, busy, error } = useRequest()
const sessions = ref<Session[]>([])
const password = reactive({ current_password: '', new_password: '', totp: '' })
const security = reactive({ password: '', totp: '', recovery_code: '' })
const enrollment = ref<{ secret: string; otpauth_url: string } | null>(null)
const recoveryCodes = ref<string[]>([])
const mfaOpen = ref(false)
const disabling = ref(false)
const acknowledged = ref(false)
async function loadSessions() {
  sessions.value = (await api<Session[]>('/auth/sessions')).data ?? []
}
watch(
  requiredPassword,
  (required) => {
    if (!required) void run(loadSessions)
    else sessions.value = []
  },
  { immediate: true },
)
async function changePassword() {
  await run(async () => {
    const wasRequired = !!state.me?.user.must_change_password
    await api('/auth/password', {
      method: 'POST',
      body: {
        current_password: password.current_password,
        new_password: password.new_password,
        ...(password.totp ? { totp: password.totp } : {}),
      },
    })
    password.current_password = ''
    password.new_password = ''
    password.totp = ''
    saved()
    await loadMe()
    await loadSessions()
    if (wasRequired && !state.me?.user.must_change_password) await router.push('/')
  })
}
async function revoke(session: Session) {
  if (await confirmDelete(t('此登录会话', 'this sign-in session')))
    await run(async () => {
      await api(`/auth/sessions/${session.id}`, { method: 'DELETE' })
      if (session.current) {
        await loadMe()
        await router.push('/login')
      } else await loadSessions()
    })
}
function openMFA(disable = false) {
  disabling.value = disable
  security.password = ''
  security.totp = ''
  security.recovery_code = ''
  enrollment.value = null
  recoveryCodes.value = []
  acknowledged.value = false
  mfaOpen.value = true
}
function acknowledgeRecovery() {
  mfaOpen.value = false
  clearEnrollment()
}
function clearEnrollment() {
  enrollment.value = null
  security.password = ''
  security.totp = ''
  security.recovery_code = ''
  recoveryCodes.value = []
}
async function enroll() {
  await run(async () => {
    enrollment.value = (
      await api<{ secret: string; otpauth_url: string }>('/auth/mfa/enroll', {
        method: 'POST',
        body: { password: security.password },
      })
    ).data
    security.password = ''
  })
}
async function confirmMFA() {
  await run(async () => {
    const result = await api<{ recovery_codes: string[] }>('/auth/mfa/confirm', {
      method: 'POST',
      body: { totp: security.totp },
    })
    enrollment.value = null
    security.totp = ''
    recoveryCodes.value = result.data.recovery_codes
    await loadMe()
    saved()
  })
}
async function disableMFA() {
  await run(async () => {
    await api('/auth/mfa/disable', {
      method: 'POST',
      body: {
        password: security.password,
        ...(security.recovery_code
          ? { recovery_code: security.recovery_code }
          : { totp: security.totp }),
      },
    })
    clearEnrollment()
    mfaOpen.value = false
    await loadMe()
    saved()
  })
}
async function copySecret() {
  if (enrollment.value)
    await run(async () => {
      await navigator.clipboard.writeText(enrollment.value!.secret)
      saved()
    })
}
</script>
<template>
  <PageHeader
    :title="t('账号与安全', 'Account & security')"
    :description="
      t(
        '管理密码、双重验证与已登录设备。',
        'Manage your password, two-factor authentication and sign-in sessions.',
      )
    "
  />
  <el-alert
    v-if="requiredPassword"
    type="warning"
    :closable="false"
    show-icon
    :title="
      t(
        '首次登录请先修改初始密码，再进入管理系统。',
        'Change your initial password before entering the administration system.',
      )
    "
  />
  <PageState :error="error" @retry="() => run(loadSessions)" />
  <div class="account-grid">
    <section class="panel">
      <div class="panel-heading">
        <h2>{{ t('个人账号', 'Your account') }}</h2>
        <span class="avatar large">{{ state.me?.user.display_name.charAt(0) }}</span>
      </div>
      <el-descriptions :column="1">
        <el-descriptions-item :label="t('姓名', 'Name')">
          {{ state.me?.user.display_name }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('邮箱', 'Email')">
          {{ state.me?.user.email }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('注册时间', 'Created')">
          {{ dateTime(state.me?.user.created_at ?? '') }}
        </el-descriptions-item>
      </el-descriptions>
      <div class="security-status">
        <div>
          <h3>{{ t('双重验证', 'Two-factor authentication') }}</h3>
          <p>
            {{
              state.me?.user.mfa_enabled
                ? t('已启用验证器保护', 'Protected with an authenticator')
                : t('为账号增加一层验证保护', 'Add an extra layer of protection')
            }}
          </p>
        </div>
        <el-tag :type="state.me?.user.mfa_enabled ? 'success' : 'warning'">
          {{ state.me?.user.mfa_enabled ? t('已启用', 'Enabled') : t('未启用', 'Not enabled') }}
        </el-tag>
      </div>
      <el-button
        v-if="!state.me?.user.must_change_password"
        :type="state.me?.user.mfa_enabled ? 'default' : 'primary'"
        @click="openMFA(!!state.me?.user.mfa_enabled)"
      >
        {{
          state.me?.user.mfa_enabled
            ? t('关闭双重验证', 'Disable two-factor authentication')
            : t('设置双重验证', 'Set up two-factor authentication')
        }}
      </el-button>
    </section>
    <section class="panel">
      <h2>{{ t('修改密码', 'Change password') }}</h2>
      <el-form @submit.prevent="changePassword" label-position="top">
        <el-form-item :label="t('当前密码', 'Current password')">
          <el-input
            v-model="password.current_password"
            type="password"
            show-password
            autocomplete="current-password"
            required
          />
        </el-form-item>
        <el-form-item :label="t('新密码', 'New password')">
          <el-input
            v-model="password.new_password"
            type="password"
            show-password
            autocomplete="new-password"
            required
            minlength="12"
          />
          <small class="hint">
            {{
              t(
                '至少 12 位。修改后其他登录会话将失效。',
                'At least 12 characters. Other sessions will be revoked.',
              )
            }}
          </small>
        </el-form-item>
        <el-form-item
          v-if="state.me?.user.mfa_enabled"
          :label="t('验证器验证码', 'Authenticator code')"
        >
          <el-input
            v-model="password.totp"
            inputmode="numeric"
            pattern="[0-9]{6}"
            maxlength="6"
            required
            autocomplete="one-time-code"
          />
        </el-form-item>
        <el-button type="primary" native-type="submit" :loading="busy">
          {{ t('更新密码', 'Update password') }}
        </el-button>
      </el-form>
    </section>
  </div>
  <section v-if="!requiredPassword" class="panel">
    <div class="panel-heading">
      <h2>{{ t('登录会话', 'Sign-in sessions') }}</h2>
      <el-button :loading="busy" @click="() => run(loadSessions)">
        {{ t('刷新', 'Refresh') }}
      </el-button>
    </div>
    <el-table row-key="id" :data="sessions" :empty-text="t('暂无会话', 'No sessions')">
      <el-table-column :label="t('会话', 'Session')" min-width="220">
        <template #default="{ row }">
          {{ row.id.slice(0, 8) }}
          <el-tag v-if="row.current" class="inline-tag" type="success">
            {{ t('当前会话', 'Current session') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('最近访问', 'Last seen')" min-width="180">
        <template #default="{ row }">{{ dateTime(row.last_seen_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('到期时间', 'Expires')" min-width="180">
        <template #default="{ row }">{{ dateTime(row.expires_at) }}</template>
      </el-table-column>
      <el-table-column width="120">
        <template #default="{ row }">
          <el-button
            type="danger"
            link
            :disabled="busy"
            :data-session-id="row.id"
            @click="revoke(row)"
          >
            {{ t('撤销', 'Revoke') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </section>
  <section v-if="!requiredPassword" class="panel">
    <div class="panel-heading">
      <h2>{{ t('账号安全活动', 'Account security activity') }}</h2>
      <el-button :loading="securityEvents.busy.value" @click="securityEvents.load">
        {{ t('刷新活动', 'Refresh activity') }}
      </el-button>
    </div>
    <PageState
      :busy="securityEvents.busy.value"
      :error="securityEvents.error.value"
      @retry="securityEvents.load"
    />
    <el-table
      :data="securityEvents.items.value"
      :empty-text="t('暂无安全活动', 'No security activity')"
    >
      <el-table-column :label="t('活动', 'Activity')" min-width="240">
        <template #default="{ row }">{{ operationLabel(row.action) }}</template>
      </el-table-column>
      <el-table-column :label="t('时间', 'Time')" min-width="180">
        <template #default="{ row }">{{ dateTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column
        prop="request_id"
        :label="t('请求编号', 'Request ID')"
        min-width="220"
        show-overflow-tooltip
      />
    </el-table>
    <div class="pagination">
      <el-pagination
        v-model:current-page="securityEvents.page.value"
        :total="securityEvents.total.value"
        :page-size="securityEvents.pageSize.value"
        layout="prev,pager,next"
        @change="securityEvents.load"
      />
    </div>
  </section>
  <el-dialog
    v-model="mfaOpen"
    :title="
      disabling
        ? t('关闭双重验证', 'Disable two-factor authentication')
        : t('设置双重验证', 'Set up two-factor authentication')
    "
    width="560px"
    :close-on-click-modal="false"
    :close-on-press-escape="!recoveryCodes.length"
    :show-close="!recoveryCodes.length"
    @closed="clearEnrollment"
  >
    <el-alert v-if="error" type="error" :title="error" :closable="false" />
    <template v-if="recoveryCodes.length">
      <el-alert
        type="warning"
        :closable="false"
        :title="
          t(
            '恢复码仅显示一次。请保存到安全位置。',
            'Recovery codes are displayed only once. Store them somewhere safe.',
          )
        "
      />
      <div class="recovery-codes">
        <code v-for="code in recoveryCodes" :key="code">{{ code }}</code>
      </div>
      <el-checkbox v-model="acknowledged">
        {{ t('我已安全保存恢复码', 'I have securely saved my recovery codes') }}
      </el-checkbox>
      <div class="dialog-actions">
        <el-button type="primary" :disabled="!acknowledged" @click="acknowledgeRecovery">
          {{ t('完成', 'Done') }}
        </el-button>
      </div>
    </template>
    <template v-else-if="enrollment">
      <p>
        {{
          t(
            '在验证器应用中导入此密钥，或打开下方设置链接。',
            'Add this key to your authenticator app or open the setup link below.',
          )
        }}
      </p>
      <div class="enrollment-secret">
        <code>{{ enrollment.secret }}</code>
        <el-button text @click="copySecret">{{ t('复制', 'Copy') }}</el-button>
      </div>
      <a :href="enrollment.otpauth_url" class="back-link">
        {{ t('在验证器中打开', 'Open in authenticator') }}
      </a>
      <el-form @submit.prevent="confirmMFA" label-position="top">
        <el-form-item :label="t('验证器验证码', 'Authenticator code')">
          <el-input
            v-model="security.totp"
            inputmode="numeric"
            pattern="[0-9]{6}"
            maxlength="6"
            required
            autocomplete="one-time-code"
          />
        </el-form-item>
        <el-button native-type="submit" type="primary" :loading="busy">
          {{ t('验证并启用', 'Verify and enable') }}
        </el-button>
      </el-form>
    </template>
    <el-form v-else @submit.prevent="disabling ? disableMFA() : enroll()" label-position="top">
      <el-form-item :label="t('当前密码', 'Current password')">
        <el-input
          v-model="security.password"
          type="password"
          required
          autocomplete="current-password"
        />
      </el-form-item>
      <template v-if="disabling">
        <el-form-item :label="t('验证器验证码', 'Authenticator code')">
          <el-input
            v-model="security.totp"
            inputmode="numeric"
            maxlength="6"
            :required="!security.recovery_code"
            autocomplete="one-time-code"
          />
        </el-form-item>
        <el-form-item :label="t('或使用一次性恢复码', 'Or use a one-time recovery code')">
          <el-input v-model="security.recovery_code" autocomplete="off" />
        </el-form-item>
      </template>
      <el-button type="primary" native-type="submit" :loading="busy">
        {{ disabling ? t('确认关闭', 'Disable authentication') : t('继续设置', 'Continue setup') }}
      </el-button>
    </el-form>
  </el-dialog>
</template>
