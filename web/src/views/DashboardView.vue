<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { api } from '../api/client'
import { state, currentTenant, t, allowed } from '../stores/app'
import { dateTime, formatValue } from '../api/forms'
import { useRequest } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
const { run, busy, error } = useRequest()
const dashboard = ref<{
  counts: Record<string, number>
  recent_activity: Record<string, unknown>[]
} | null>(null)
const modules = computed(() =>
  (state.me?.modules ?? []).filter(
    (module) =>
      allowed(module.key, 'read') &&
      !['audit', 'files', 'jobs', 'members', 'departments', 'roles', 'invitations'].includes(
        module.key,
      ),
  ),
)
async function load() {
  await run(async () => {
    dashboard.value = (await api<typeof dashboard.value>('/dashboard')).data
  })
}
onMounted(load)
function name(key: string) {
  const module = state.me?.modules.find((item) => item.key === key)
  return module ? (state.locale === 'zh' ? module.label : module.label_en) : key
}
</script>
<template>
  <PageHeader
    :title="
      t(
        `你好，${state.me?.user.display_name ?? ''}`,
        `Welcome, ${state.me?.user.display_name ?? ''}`,
      )
    "
    :description="
      t('掌握团队动态，从这里开始今天的工作。', 'A clear view of your workspace and today’s work.')
    "
    :eyebrow="currentTenant?.name"
  >
    <el-button :loading="busy" @click="load">{{ t('刷新', 'Refresh') }}</el-button>
  </PageHeader>
  <PageState :busy="busy" :error="error" @retry="load" />
  <template v-if="dashboard">
    <div class="stats-grid">
      <router-link
        v-for="(count, key) in dashboard.counts"
        :key="key"
        :to="modules.some((item) => item.key === key) ? `/resources/${key}` : '/'"
        class="stat-card"
      >
        <p>{{ name(String(key)) }}</p>
        <strong>{{ formatValue(count) }}</strong>
        <small>{{ t('当前可见记录', 'Records visible to you') }}</small>
      </router-link>
    </div>
    <div class="dashboard-grid">
      <section class="panel">
        <div class="panel-heading">
          <h2>{{ t('最近活动', 'Recent activity') }}</h2>
          <router-link v-if="allowed('audit', 'read')" to="/audit">
            {{ t('查看全部', 'View all') }} →
          </router-link>
        </div>
        <el-empty
          v-if="!dashboard.recent_activity?.length"
          :description="t('暂无可见活动', 'No visible activity')"
          :image-size="65"
        />
        <div
          v-for="entry in dashboard.recent_activity"
          :key="String(entry.id)"
          class="activity-entry"
        >
          <span class="activity-dot" />
          <div>
            <strong>{{ entry.action }} · {{ name(String(entry.resource ?? '')) }}</strong>
            <p>{{ entry.record_id }}</p>
          </div>
          <time>{{ dateTime(String(entry.created_at ?? '')) }}</time>
        </div>
      </section>
      <section class="panel">
        <h2>{{ t('快捷入口', 'Quick access') }}</h2>
        <div class="quick-links">
          <router-link v-for="module in modules" :key="module.key" :to="`/resources/${module.key}`">
            <span class="quick-icon" aria-hidden="true">{{ module.label_en.charAt(0) }}</span>
            <span>
              {{ state.locale === 'zh' ? module.label : module.label_en }}
              <small>{{ module.description }}</small>
            </span>
            <span aria-hidden="true">→</span>
          </router-link>
          <router-link to="/account">
            <span class="quick-icon" aria-hidden="true">◎</span>
            <span>
              {{ t('账号安全', 'Account security') }}
              <small>{{ t('密码、验证器与登录会话', 'Password, MFA and sessions') }}</small>
            </span>
            <span aria-hidden="true">→</span>
          </router-link>
        </div>
      </section>
    </div>
  </template>
</template>
