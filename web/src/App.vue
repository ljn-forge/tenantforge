<script setup lang="ts">
import { computed, ref, nextTick, onMounted, onUnmounted } from 'vue'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import en from 'element-plus/es/locale/lang/en'
import ModuleIcon from './components/ModuleIcon.vue'
import { Sunny, Moon, ArrowDown } from '@element-plus/icons-vue'
import { useRoute, useRouter } from 'vue-router'
import {
  state,
  currentTenant,
  t,
  allowed,
  switchTenant,
  logout,
  setLocale,
  toggleTheme,
  bootstrap,
} from './stores/app'
import { useRequest } from './composables/request'
const route = useRoute()
const router = useRouter()
const { run, busy } = useRequest()
const menu = ref(false)
const menuButton = ref<HTMLButtonElement | null>(null)
const sidebar = ref<HTMLElement | null>(null)
async function openMenu() {
  menu.value = true
  await nextTick()
  sidebar.value?.querySelector<HTMLElement>('a')?.focus()
}
function closeMenu() {
  menu.value = false
  menuButton.value?.focus()
}
function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && menu.value) {
    event.preventDefault()
    closeMenu()
  }
}
onMounted(() => document.addEventListener('keydown', handleKeydown))
onUnmounted(() => document.removeEventListener('keydown', handleKeydown))
const publicPage = computed(() =>
  ['/login', '/setup', '/reset-password', '/reset'].includes(route.path),
)
const links = computed(() =>
  state.me?.user.must_change_password
    ? []
    : [
        {
          path: '/',
          label: t('工作台', 'Overview'),
          show: true,
          section: t('工作空间', 'Workspace'),
        },
        ...(state.me?.modules ?? [])
          .filter(
            (module) =>
              ![
                'audit',
                'files',
                'jobs',
                'members',
                'departments',
                'roles',
                'invitations',
              ].includes(module.key),
          )
          .map((module) => ({
            path: `/resources/${module.key}`,
            label: state.locale === 'zh' ? module.label : module.label_en,
            show: allowed(module.key, 'read'),
            section: t('工作空间', 'Workspace'),
          })),
        ...[
          ['members', '用户管理', 'Users'],
          ['departments', '部门与岗位', 'Organization'],
          ['roles', '角色与授权', 'Roles & access'],
          ['invitations', '邀请', 'Invitations'],
        ].map(([key, zh, en]) => ({
          path: `/${key}`,
          label: t(zh!, en!),
          show: allowed(key!, 'read'),
          section: t('组织与权限', 'Organization & access'),
        })),
        ...[
          ['files', '文件', 'Files'],
          ['jobs', '后台任务', 'Jobs'],
          ['audit', '审计记录', 'Audit trail'],
        ].map(([key, zh, en]) => ({
          path: `/${key}`,
          label: t(zh!, en!),
          show: allowed(key!, 'read'),
          section: t('管理与运维', 'Administration'),
        })),
        {
          path: '/platform/tenants',
          label: t('平台租户', 'Platform tenants'),
          show: state.me?.platform_admin,
          section: t('平台', 'Platform'),
        },
      ].filter((item) => item.show),
)
const sections = computed(() => [...new Set(links.value.map((item) => item.section))])
const title = computed(
  () =>
    links.value.find((item) => item.path === route.path)?.label ??
    t('账号设置', 'Account settings'),
)
async function retryConnection() {
  await bootstrap()
  if (!state.startupError)
    await router.replace(
      state.me ? (state.me.user.must_change_password ? '/account?required=1' : '/') : '/login',
    )
}
async function chooseTenant(value: string) {
  await run(async () => {
    await switchTenant(value)
    await router.push('/')
    menu.value = false
  })
}
function profileCommand(value: string) {
  return value === 'logout' ? exit() : router.push('/account')
}
async function exit() {
  await run(async () => {
    await logout()
    await router.push('/login')
  })
}
</script>

<template>
  <el-config-provider :locale="state.locale === 'zh' ? zhCn : en">
    <div v-if="state.startupError" class="connection-error">
      <span class="brand-mark">TF</span>
      <h1>{{ t('连接暂不可用', 'Connection unavailable') }}</h1>
      <p role="alert">{{ state.startupError }}</p>
      <el-button type="primary" :loading="state.startupBusy" @click="retryConnection">
        {{ t('重新连接', 'Retry connection') }}
      </el-button>
    </div>
    <router-view v-else-if="publicPage" />
    <div v-else class="shell">
      <button
        v-if="menu"
        class="sidebar-backdrop"
        aria-label="Close navigation"
        @click="closeMenu"
      />
      <aside id="workspace-navigation" ref="sidebar" class="sidebar" :class="{ opened: menu }">
        <router-link to="/" class="brand">
          <span class="brand-mark">TF</span>
          <span>
            TenantForge
            <small>{{ t('团队管理，清晰有序', 'Your workspace, well governed') }}</small>
          </span>
        </router-link>
        <div class="tenant-picker">
          <label for="tenant-select">{{ t('当前组织', 'Current workspace') }}</label>
          <el-select
            id="tenant-select"
            :model-value="state.tenant"
            :disabled="busy || state.loading || state.me?.user.must_change_password"
            :placeholder="t('选择组织', 'Select workspace')"
            @change="chooseTenant"
          >
            <el-option
              v-for="tenant in state.me?.tenants.filter((item) => item.status === 'active')"
              :key="tenant.id"
              :label="tenant.name"
              :value="tenant.id"
            />
          </el-select>
        </div>
        <nav aria-label="Main navigation">
          <div v-for="section in sections" :key="section" class="nav-section">
            <p>{{ section }}</p>
            <router-link
              v-for="link in links.filter((item) => item.section === section)"
              :key="link.path"
              :to="link.path"
              :class="{ active: route.path === link.path }"
              @click="menu = false"
            >
              <ModuleIcon
                :name="link.path === '/' ? 'overview' : (link.path.split('/').pop() ?? 'overview')"
              />
              {{ link.label }}
            </router-link>
          </div>
        </nav>
        <div class="sidebar-footer">
          <span class="status-dot" />
          {{ t('独立工作空间', 'Isolated workspace') }}
          <small>{{ state.version }}</small>
        </div>
      </aside>
      <main class="main">
        <header class="topbar">
          <div class="topbar-title">
            <button
              ref="menuButton"
              class="mobile-menu"
              :aria-label="t('打开导航', 'Open navigation')"
              :aria-expanded="menu"
              aria-controls="workspace-navigation"
              @click="menu ? closeMenu() : openMenu()"
            >
              <ModuleIcon name="menu" />
            </button>
            <span>{{ currentTenant?.name ?? 'TenantForge' }}</span>
            <span class="breadcrumb-divider">/</span>
            <strong>{{ title }}</strong>
          </div>
          <div class="topbar-actions">
            <el-button text @click="setLocale">
              {{ state.locale === 'zh' ? 'EN' : '中文' }}
            </el-button>
            <el-button text :aria-label="t('切换主题', 'Toggle theme')" @click="toggleTheme">
              <el-icon>
                <Sunny v-if="state.dark" />
                <Moon v-else />
              </el-icon>
            </el-button>
            <el-dropdown @command="profileCommand">
              <button class="profile-button">
                <span class="avatar">
                  {{ state.me?.user.display_name.slice(0, 1).toUpperCase() }}
                </span>
                <span>{{ state.me?.user.display_name }}</span>
                <el-icon><ArrowDown /></el-icon>
              </button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="account">
                    {{ t('账号与安全', 'Account & security') }}
                  </el-dropdown-item>
                  <el-dropdown-item command="logout" :disabled="busy">
                    {{ t('退出登录', 'Sign out') }}
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </header>
        <section class="content" :aria-busy="state.loading">
          <el-skeleton v-if="state.loading" :rows="8" animated />
          <router-view v-else :key="`${state.tenant}:${route.path}`" />
        </section>
      </main>
    </div>
  </el-config-provider>
</template>
