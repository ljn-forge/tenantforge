import { createRouter, createWebHistory } from 'vue-router'
import { state } from './stores/app'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('./views/AuthView.vue') },
    { path: '/setup', redirect: '/login' },
    { path: '/reset-password', alias: '/reset', component: () => import('./views/AuthView.vue') },
    {
      path: '/accept-invitation',
      alias: '/invite',
      component: () => import('./views/InvitationAccept.vue'),
    },
    { path: '/', component: () => import('./views/DashboardView.vue') },
    { path: '/resources/:module', component: () => import('./views/ResourcesView.vue') },
    { path: '/roles', component: () => import('./views/RolesView.vue') },
    { path: '/departments', component: () => import('./views/DepartmentsView.vue') },
    { path: '/members', component: () => import('./views/UsersView.vue') },
    { path: '/invitations', component: () => import('./views/InvitationsView.vue') },
    { path: '/files', component: () => import('./views/FilesView.vue') },
    { path: '/jobs', component: () => import('./views/ActivityView.vue') },
    { path: '/audit', component: () => import('./views/ActivityView.vue') },
    { path: '/account', component: () => import('./views/AccountView.vue') },
    { path: '/platform/tenants', component: () => import('./views/TenantsView.vue') },
    { path: '/:pathMatch(.*)*', component: () => import('./views/NotFoundView.vue') },
  ],
})
router.beforeEach((to) => {
  if (state.me?.user.must_change_password && to.path !== '/account')
    return { path: '/account', query: { required: '1' } }
  if (
    !state.me &&
    !['/login', '/reset-password', '/reset'].includes(to.path) &&
    to.path !== '/setup'
  )
    return { path: '/login', query: { redirect: to.fullPath } }
  if (state.me && to.path === '/login') return '/'
})
