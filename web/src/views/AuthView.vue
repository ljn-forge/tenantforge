<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, APIError } from '../api/client'
import { state, t, loggedIn, setLocale, toggleTheme } from '../stores/app'
import { useRequest } from '../composables/request'
const route = useRoute()
const router = useRouter()
const { run, busy, error } = useRequest()
const reset = computed(() => ['/reset-password', '/reset'].includes(route.path))
const token = computed(() => (typeof route.query.token === 'string' ? route.query.token : ''))
const form = reactive({ identity: 'admin', email: '', password: '', totp: '', recovery_code: '' })
const mfa = ref(false)
const recovery = ref(false)
const sent = ref(false)
const resetConfirmed = ref(false)
const title = computed(() =>
  reset.value ? t('重置密码', 'Reset your password') : t('管理员登录', 'Administrator sign in'),
)
async function submit() {
  await run(async () => {
    if (reset.value) {
      const confirming = !!token.value
      await api(`/auth/reset/${confirming ? 'confirm' : 'request'}`, {
        method: 'POST',
        body: confirming
          ? { token: token.value, new_password: form.password }
          : { email: form.email },
      })
      sent.value = true
      resetConfirmed.value = confirming
      form.password = ''
      if (confirming) await router.replace('/reset-password')
      return
    }
    try {
      await api('/auth/login', {
        method: 'POST',
        body: {
          username: form.identity,
          password: form.password,
          ...(mfa.value
            ? recovery.value
              ? { recovery_code: form.recovery_code }
              : { totp: form.totp }
            : {}),
        },
      })
    } catch (failure) {
      if (failure instanceof APIError && failure.code === 'mfa_required') {
        mfa.value = true
        return
      }
      throw failure
    }
    form.password = ''
    form.totp = ''
    form.recovery_code = ''
    await loggedIn()
    const target =
      typeof route.query.redirect === 'string' &&
      route.query.redirect.startsWith('/') &&
      !route.query.redirect.startsWith('//')
        ? route.query.redirect
        : '/'
    await router.push(state.me?.user.must_change_password ? '/account?required=1' : target)
  })
}
</script>
<template>
  <div class="auth-page">
    <div class="auth-tools">
      <el-button text @click="setLocale">{{ state.locale === 'zh' ? 'EN' : '中文' }}</el-button>
      <el-button text @click="toggleTheme" :aria-label="t('切换主题', 'Toggle theme')">
        {{ state.dark ? '☀' : '☾' }}
      </el-button>
    </div>
    <section class="auth-story">
      <router-link class="brand" to="/">
        <span class="brand-mark">TF</span>
        <span>TenantForge</span>
      </router-link>
      <div>
        <p class="eyebrow">{{ t('为每一支团队建立秩序', 'Clarity for every team') }}</p>
        <h2>
          {{ t('专注业务，\n让协作井然有序。', 'Your work.\nYour people.\nOne clear workspace.') }}
        </h2>
        <p>
          {{
            t(
              '用户、角色、数据与字段权限，在一个管理后台统一维护。',
              'Manage users, roles, records and field access in one administration system.',
            )
          }}
        </p>
      </div>
      <small>
        {{ state.version }} · {{ t('用户 · 角色 · 权限', 'Users · Roles · Permissions') }}
      </small>
    </section>
    <main class="auth-panel">
      <div class="auth-card">
        <p class="eyebrow">TENANTFORGE</p>
        <h1>{{ title }}</h1>
        <p class="page-description">
          {{
            reset
              ? t('安全恢复你的账号访问。', 'Securely restore access to your account.')
              : t(
                  '使用管理员分配的账号登录。',
                  'Sign in with an account provided by your administrator.',
                )
          }}
        </p>
        <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
        <el-result
          v-if="sent"
          icon="success"
          :title="
            resetConfirmed ? t('密码已重置', 'Password reset') : t('请求已接收', 'Request received')
          "
          :sub-title="
            resetConfirmed
              ? t('请使用新密码登录。', 'Sign in using your new password.')
              : t(
                  '如果邮箱对应有效账号，你将收到重置链接。',
                  'If the email belongs to an account, a reset link will be delivered.',
                )
          "
        >
          <template #extra>
            <router-link to="/login">
              <el-button type="primary">{{ t('返回登录', 'Back to sign in') }}</el-button>
            </router-link>
          </template>
        </el-result>
        <el-form v-else @submit.prevent="submit" label-position="top">
          <el-form-item v-if="!reset" :label="t('用户名或邮箱', 'Username or email')">
            <el-input v-model="form.identity" autocomplete="username" required />
          </el-form-item>
          <el-form-item v-if="reset && !token" :label="t('邮箱', 'Email')">
            <el-input v-model="form.email" type="email" autocomplete="email" required />
          </el-form-item>
          <el-form-item
            v-if="!reset || token"
            :label="reset ? t('新密码', 'New password') : t('密码', 'Password')"
          >
            <el-input
              v-model="form.password"
              type="password"
              show-password
              :autocomplete="reset ? 'new-password' : 'current-password'"
              required
              :minlength="reset ? 12 : 1"
            />
            <small v-if="reset" class="hint">
              {{ t('至少 12 位字符', 'At least 12 characters') }}
            </small>
          </el-form-item>
          <template v-if="mfa">
            <el-form-item
              :label="
                recovery
                  ? t('一次性恢复码', 'One-time recovery code')
                  : t('验证器验证码', 'Authenticator code')
              "
            >
              <el-input v-if="recovery" v-model="form.recovery_code" required autocomplete="off" />
              <el-input
                v-else
                v-model="form.totp"
                required
                inputmode="numeric"
                pattern="[0-9]{6}"
                maxlength="6"
                autocomplete="one-time-code"
              />
            </el-form-item>
            <el-button text @click="recovery = !recovery">
              {{
                recovery
                  ? t('使用验证器', 'Use authenticator')
                  : t('使用恢复码', 'Use recovery code')
              }}
            </el-button>
          </template>
          <el-button type="primary" native-type="submit" class="full-width" :loading="busy">
            {{ reset ? t('提交', 'Submit') : t('登录', 'Sign in') }}
          </el-button>
        </el-form>
        <p v-if="!reset" class="hint default-account">
          {{
            t(
              '首次登录：账号 admin，初始密码 Admin@123456。登录后必须修改密码。',
              'First sign in: admin / Admin@123456. You must change this password immediately.',
            )
          }}
        </p>
        <div v-if="!reset" class="auth-links">
          <router-link to="/reset-password">{{ t('忘记密码？', 'Forgot password?') }}</router-link>
          <a v-if="state.oidc" href="/api/v1/auth/oidc/start">
            {{ t('企业身份登录', 'Sign in with your organization') }}
          </a>
        </div>
        <router-link v-if="reset && !sent" to="/login" class="back-link">
          {{ t('返回登录', 'Back to sign in') }}
        </router-link>
      </div>
    </main>
  </div>
</template>
