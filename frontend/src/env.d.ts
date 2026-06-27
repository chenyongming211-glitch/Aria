/// <reference types="vite/client" />

import type { Permission, RoleName } from './types/permission'

declare module "*.vue" {
  import type { DefineComponent } from "vue";

  const component: DefineComponent<Record<string, unknown>, Record<string, unknown>, unknown>;
  export default component;
}

declare module 'vue-router' {
  interface RouteMeta {
    titleKey?: string
    requiresAuth?: boolean
    permission?: Permission | Permission[]
    role?: RoleName | string
    section?: string
    allowPasswordChange?: boolean
  }
}
