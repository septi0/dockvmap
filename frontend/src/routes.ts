import { get } from 'svelte/store'
import { replace } from 'svelte-spa-router'
import { wrap } from 'svelte-spa-router/wrap'
import type { RouteDetail } from 'svelte-spa-router'
import { auth } from './lib/services/auth'
import Login from './routes/Login.svelte'
import Setup from './routes/Setup.svelte'
import Dashboard from './routes/Dashboard.svelte'
import Registries from './routes/Registries.svelte'
import Images from './routes/Images.svelte'
import CreateImage from './routes/CreateImage.svelte'
import ImageDetails from './routes/ImageDetails.svelte'
import AuditLog from './routes/AuditLog.svelte'
import ProxyTokens from './routes/ProxyTokens.svelte'
import Profile from './routes/Profile.svelte'
import NotFound from './routes/NotFound.svelte'

function requireAuth() {
  return get(auth).status === 'authenticated'
}

let intendedLocation = '/'

export function takeIntendedLocation(): string {
  const location = intendedLocation
  intendedLocation = '/'
  return location
}

export function handleConditionsFailed(detail: RouteDetail) {
  const status = get(auth).status

  if (status === 'setup-required') {
    replace('/setup')
  } else if (status === 'authenticated') {
    replace('/')
  } else {
    intendedLocation = detail.location
    replace('/login')
  }
}

export default {
  '/login': wrap({
    component: Login,
    conditions: [() => get(auth).status === 'unauthenticated'],
  }),

  '/setup': wrap({
    component: Setup,
    conditions: [() => get(auth).status === 'setup-required'],
  }),

  '/': wrap({ component: Dashboard, conditions: [requireAuth] }),
  '/registries': wrap({ component: Registries, conditions: [requireAuth] }),
  '/images': wrap({ component: Images, conditions: [requireAuth] }),
  '/images/new': wrap({ component: CreateImage, conditions: [requireAuth] }),
  '/images/:id': wrap({ component: ImageDetails, conditions: [requireAuth] }),
  '/audit-log': wrap({ component: AuditLog, conditions: [requireAuth] }),
  '/proxy-tokens': wrap({ component: ProxyTokens, conditions: [requireAuth] }),
  '/account/profile': wrap({ component: Profile, conditions: [requireAuth] }),

  '*': NotFound,
}
