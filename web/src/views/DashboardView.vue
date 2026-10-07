<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { api } from '../api/client'
import { state, currentTenant, t, allowed } from '../stores/app'
import { dateTime } from '../api/forms'
import { activitySentence, moduleLabel, moduleDescription } from '../api/presentation'
import { useRequest } from '../composables/request'
import PageHeader from '../components/PageHeader.vue'
import PageState from '../components/PageState.vue'
import ModuleIcon from '../components/ModuleIcon.vue'
interface Dashboard {
  counts: Record<string, number>
  recent_activity: Record<string, unknown>[]
  workload?: Record<string, number>
}
const { run, busy, error } = useRequest(),
  dashboard = ref<Dashboard | null>(null)
const modules = computed(() =>
  (state.me?.modules ?? []).filter(
    (module) =>
      allowed(module.key, 'read') &&
      !['audit', 'files', 'jobs', 'members', 'departments', 'roles', 'invitations'].includes(
        module.key,
      ),
  ),
)
const workloadDefinitions = [
  {
    key: 'open_tickets',
    zh: '待处理工单',
    en: 'Open tickets',
    resource: 'tickets',
    path: '/resources/tickets?status=open',
    hintZh: '推进处理中工作',
    hintEn: 'Keep work moving',
    tone: 'teal',
  },
  {
    key: 'urgent_tickets',
    zh: '紧急工单',
    en: 'Urgent tickets',
    resource: 'tickets',
    path: '/resources/tickets?priority=urgent',
    hintZh: '优先关注',
    hintEn: 'Needs attention first',
    tone: 'danger',
  },
  {
    key: 'running_jobs',
    zh: '执行中的任务',
    en: 'Running jobs',
    resource: 'jobs',
    path: '/jobs?status=running',
    hintZh: '查看后台进度',
    hintEn: 'Track background progress',
    tone: 'neutral',
  },
  {
    key: 'failed_jobs',
    zh: '失败任务',
    en: 'Failed jobs',
    resource: 'jobs',
    path: '/jobs?status=failed',
    hintZh: '检查原因并重试',
    hintEn: 'Review and retry',
    tone: 'warning',
  },
  {
    key: 'members',
    zh: '团队用户',
    en: 'Team users',
    resource: 'members',
    path: '/members',
    hintZh: '管理团队访问',
    hintEn: 'Manage team access',
    tone: 'neutral',
  },
]
const workload = computed(() =>
  workloadDefinitions.filter((item) => Object.hasOwn(dashboard.value?.workload ?? {}, item.key)),
)
async function load() {
  await run(async () => {
    dashboard.value = (await api<Dashboard>('/dashboard')).data
  })
}
onMounted(load)
function name(key: string) {
  const module = state.me?.modules.find((item) => item.key === key)
  return module ? moduleLabel(module) : key
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
      t(
        '从待办和需要关注的工作开始。每个数字都来自当前可见的数据。',
        'Start with pending work and what needs attention. Every count reflects your current access.',
      )
    "
    :eyebrow="currentTenant?.name"
  >
    <el-button :loading="busy" @click="load">{{ t('刷新', 'Refresh') }}</el-button>
  </PageHeader>
  <PageState :busy="busy" :error="error" @retry="load" />
  <template v-if="dashboard">
    <div v-if="workload.length" class="workload-grid">
      <router-link
        v-for="item in workload"
        :key="item.key"
        :to="item.path"
        class="workload-card"
        :class="item.tone"
      >
        <div class="workload-label">
          <span>{{ t(item.zh, item.en) }}</span>
          <ModuleIcon :name="item.resource" />
        </div>
        <strong>{{ dashboard.workload?.[item.key] }}</strong>
        <small>
          {{ t(item.hintZh, item.hintEn) }}
          <span aria-hidden="true">↗</span>
        </small>
      </router-link>
    </div>
    <div class="dashboard-grid">
      <section class="panel activity-panel">
        <div class="panel-heading">
          <div>
            <span class="eyebrow">{{ t('团队动态', 'TEAM ACTIVITY') }}</span>
            <h2>{{ t('最近发生的事', 'Recent activity') }}</h2>
          </div>
          <router-link v-if="allowed('audit', 'read')" to="/audit">
            {{ t('查看审计', 'View audit') }}
            <span aria-hidden="true">→</span>
          </router-link>
        </div>
        <el-empty
          v-if="!dashboard.recent_activity?.length"
          :description="t('暂无可见活动', 'No visible activity')"
          :image-size="56"
        />
        <div
          v-for="entry in dashboard.recent_activity.slice(0, 7)"
          :key="String(entry.id)"
          class="activity-entry"
        >
          <span class="activity-icon"><ModuleIcon :name="String(entry.resource)" /></span>
          <div>
            <strong>{{ activitySentence(entry, state.me?.modules ?? []) }}</strong>
            <small>{{ t('已记录授权操作', 'Authorized operation recorded') }}</small>
          </div>
          <time>{{ dateTime(String(entry.created_at ?? '')) }}</time>
        </div>
      </section>
      <section class="panel">
        <div class="panel-heading">
          <div>
            <span class="eyebrow">{{ t('开始工作', 'YOUR WORKSPACE') }}</span>
            <h2>{{ t('快捷入口', 'Quick access') }}</h2>
          </div>
        </div>
        <div class="quick-links">
          <router-link v-for="module in modules" :key="module.key" :to="`/resources/${module.key}`">
            <span class="quick-icon"><ModuleIcon :name="module.key" /></span>
            <span>
              {{ moduleLabel(module) }}
              <small>{{ moduleDescription(module) }}</small>
            </span>
            <span aria-hidden="true">→</span>
          </router-link>
          <router-link to="/account">
            <span class="quick-icon"><ModuleIcon name="account" /></span>
            <span>
              {{ t('账号安全', 'Account security') }}
              <small>{{ t('密码、验证器与登录会话', 'Password, MFA and sessions') }}</small>
            </span>
            <span aria-hidden="true">→</span>
          </router-link>
        </div>
      </section>
    </div>
    <section v-if="Object.keys(dashboard.counts).length" class="panel resource-summary">
      <div class="panel-heading">
        <div>
          <span class="eyebrow">{{ t('当前可见', 'VISIBLE TO YOU') }}</span>
          <h2>{{ t('业务资源总览', 'Resource overview') }}</h2>
        </div>
      </div>
      <div class="resource-counts">
        <router-link
          v-for="(count, key) in dashboard.counts"
          :key="key"
          :to="`/resources/${key}`"
          class="stat-card"
        >
          <ModuleIcon :name="String(key)" />
          <p>{{ name(String(key)) }}</p>
          <strong>{{ count }}</strong>
        </router-link>
      </div>
    </section>
  </template>
</template>
