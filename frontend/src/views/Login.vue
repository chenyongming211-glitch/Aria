<!-- src/views/Login.vue - 深色主题登录页 -->
<template>
  <div class="login-container">
    <!-- 左上角品牌 -->
    <div class="brand-corner">
      <img src="/aria-logo.png" alt="Aria Logo" class="corner-logo" />
      <div class="corner-info">
        <h1 class="corner-title">
          Aria
          <span class="pro-badge">PRO</span>
        </h1>
        <p class="corner-subtitle">Decentralized SD-WAN</p>
      </div>
    </div>

    <!-- 主要内容区域 -->
    <div class="login-wrapper">
      <div class="login-side">
        <div class="login-card">
          <div class="login-header">
            <h2>{{ t('login.welcome') }}</h2>
            <p>{{ t('login.subtitle') }}</p>
          </div>

          <el-form
            :model="loginForm"
            :rules="loginRules"
            ref="loginFormRef"
            class="login-form"
            @keyup.enter="handleLogin"
          >
            <el-form-item prop="username">
              <el-input
                v-model.trim="loginForm.username"
                name="username"
                autocomplete="username"
                :placeholder="t('login.usernamePlaceholder')"
                size="large"
                class="login-input"
              >
                <template #prefix>
                  <el-icon class="input-icon"><User /></el-icon>
                </template>
              </el-input>
            </el-form-item>

            <el-form-item prop="password">
              <el-input
                v-model.trim="loginForm.password"
                type="password"
                :placeholder="t('login.passwordPlaceholder')"
                size="large"
                class="login-input"
                show-password
              >
                <template #prefix>
                  <el-icon class="input-icon"><Lock /></el-icon>
                </template>
              </el-input>
            </el-form-item>

            <el-form-item>
              <div class="form-footer">
                <el-checkbox v-model="loginForm.rememberMe" class="login-checkbox">
                  {{ t('login.rememberMe') }}
                </el-checkbox>
              </div>
            </el-form-item>

            <el-form-item>
              <el-button
                type="primary"
                size="large"
                class="submit-button aria-btn-primary"
                :loading="loading"
                @click="handleLogin"
              >
                <span v-if="!loading">{{ t('login.submit') }}</span>
                <span v-else>{{ t('login.submitting') }}</span>
              </el-button>
            </el-form-item>
          </el-form>

          <div class="footer-section">
            <p class="version-text">Aria Controller v{{ appVersion }}</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { User, Lock } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useUserStore, useAppStore } from '@/stores'
import { t } from '@/i18n'

const router = useRouter()
const userStore = useUserStore()
const appStore = useAppStore()

const appVersion = computed(() => appStore.version)

const loadRememberedCredentials = () => {
  const remembered = localStorage.getItem('aria_remembered_login')
  if (remembered) {
    try {
      const creds = JSON.parse(remembered)
      return {
        username: creds.username || '',
        password: '',
        rememberMe: true
      }
    } catch {
      return {
        username: '',
        password: '',
        rememberMe: false
      }
    }
  }
  return {
    username: '',
    password: '',
    rememberMe: false
  }
}

const initialCredentials = loadRememberedCredentials()
const loginForm = ref({
  username: initialCredentials.username,
  password: initialCredentials.password,
  rememberMe: initialCredentials.rememberMe
})

const loading = ref(false)
const loginFormRef = ref(null)

const loginRules = computed(() => ({
  username: [
    { required: true, message: t('login.usernameRequired'), trigger: 'blur' },
    { min: 3, max: 50, message: t('login.usernameLength'), trigger: 'blur' }
  ],
  password: [
    { required: true, message: t('login.passwordRequired'), trigger: 'blur' },
    { min: 6, message: t('login.passwordLength'), trigger: 'blur' }
  ]
}))

const handleLogin = async () => {
  if (!loginFormRef.value) return

  try {
    await new Promise((resolve, reject) => {
      loginFormRef.value.validate((valid, fields) => {
        if (valid) {
          resolve()
        } else {
          reject(fields)
        }
      })
    })

    loading.value = true

    const credentials = {
      username: loginForm.value.username,
      password: loginForm.value.password
    }

    const result = await userStore.login(credentials)

    if (result.success) {
      // 检查是否需要强制修改密码
      if (result.requirePasswordChange) {
        router.push('/change-password')
        return
      }

      if (loginForm.value.rememberMe) {
        localStorage.setItem('aria_remembered_login', JSON.stringify({
          username: loginForm.value.username
        }))
      } else {
        localStorage.removeItem('aria_remembered_login')
      }

      const tenants = await userStore.loadTenants()
      const switchableTenants = (tenants || []).filter((tenant) => {
        const status = String(tenant?.status || 'active').toLowerCase()
        return status === 'active'
      })
      if (switchableTenants.length > 0) {
        let loginTenantId = ''
        try {
          const currentUser = JSON.parse(localStorage.getItem('aria_user') || '{}')
          loginTenantId = currentUser?.tenant_id || ''
        } catch (error) {
          console.warn('Failed to parse current user from localStorage:', error)
        }

        const selectedTenant = loginTenantId
          ? switchableTenants.find((tenant) => tenant.id === loginTenantId)
          : null

        localStorage.setItem('aria-current-tenant', JSON.stringify(selectedTenant || switchableTenants[0]))
      } else {
        localStorage.removeItem('aria-current-tenant')
      }

      router.push('/dashboard')
      ElMessage.success(t('login.success'))
    } else {
      ElMessage.error(t('login.invalidCredentials'))
    }
  } catch (error) {
    console.error('Login error:', error)
    if (error && typeof error === 'object' && !Array.isArray(error) && Object.keys(error).length > 0) {
      return
    }
    ElMessage.error(t('login.retryLater'))
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  position: relative;
  background: var(--aria-sidebar-bg);
  overflow: auto;
}

.brand-corner {
  position: absolute;
  top: 28px;
  left: 32px;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 12px;
}

.corner-logo {
  width: 40px;
  height: 40px;
  object-fit: contain;
}

.corner-info {
  display: flex;
  flex-direction: column;
}

.corner-title {
  font-size: 22px;
  font-weight: 700;
  margin: 0;
  color: #fff;
  display: flex;
  align-items: center;
  gap: 8px;
  letter-spacing: 0;
}

.pro-badge {
  font-size: 10px;
  padding: 2px 6px;
  background: rgba(59, 130, 246, 0.14);
  border: 1px solid rgba(96, 165, 250, 0.34);
  border-radius: 4px;
  letter-spacing: 0;
  font-weight: 600;
  color: #bfdbfe;
}

.corner-subtitle {
  color: rgba(255, 255, 255, 0.5);
  font-size: 13px;
  margin: 4px 0 0 0;
  font-weight: 400;
}

.login-wrapper {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 112px 24px 40px;
  position: relative;
  z-index: 10;
}

.login-side {
  display: flex;
  justify-content: center;
  width: min(100%, 420px);
}

.login-card {
  width: 100%;
  max-width: 400px;
  padding: 40px 36px;
  background: #0f172a;
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 8px;
  box-shadow: var(--aria-shadow-md);
}

.login-header {
  text-align: center;
  margin-bottom: 36px;
}

.login-header h2 {
  font-size: 26px;
  font-weight: 600;
  color: #fff;
  margin: 0 0 8px 0;
}

.login-header p {
  color: rgba(255, 255, 255, 0.45);
  font-size: 14px;
  margin: 0;
}

.login-form {
  margin-bottom: 24px;
}

:deep(.login-input .el-input__wrapper) {
  background: rgba(0, 0, 0, 0.35);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: none;
  transition: all var(--aria-transition-base);
  padding: 4px 16px;
  border-radius: 10px;
}

:deep(.login-input .el-input__wrapper:hover) {
  border-color: rgba(59, 130, 246, 0.4);
}

:deep(.login-input .el-input__wrapper.is-focus) {
  border-color: var(--aria-primary);
  outline: 2px solid rgba(59, 130, 246, 0.24);
  outline-offset: 1px;
}

:deep(.login-input .el-input__inner) {
  background: transparent;
  color: #fff;
  font-size: 15px;
}

:deep(.login-input .el-input__inner::placeholder) {
  color: rgba(255, 255, 255, 0.35);
}

.input-icon {
  color: rgba(255, 255, 255, 0.35);
  font-size: 18px;
}

:deep(.login-input .el-input__wrapper:hover) .input-icon,
:deep(.login-input .el-input__wrapper.is-focus) .input-icon {
  color: var(--aria-primary);
}

.form-footer {
  display: flex;
  justify-content: flex-start;
}

:deep(.login-checkbox .el-checkbox__label) {
  color: rgba(255, 255, 255, 0.5);
  font-size: 14px;
}

:deep(.login-checkbox .el-checkbox__input.is-checked .el-checkbox__inner) {
  background-color: var(--aria-primary);
  border-color: var(--aria-primary);
}

:deep(.submit-button) {
  width: 100%;
  height: 46px;
  font-size: 15px;
  font-weight: 600;
  border-radius: 8px;
  background: var(--aria-primary);
  border: none;
  transition: all 0.25s ease;
}

:deep(.submit-button:hover) {
  background: var(--aria-primary-dark);
}

.footer-section {
  text-align: center;
  padding-top: 20px;
}

.version-text {
  color: rgba(255, 255, 255, 0.25);
  font-size: 11px;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  margin: 0;
  letter-spacing: 0;
}

@media (max-width: 900px) {
  .brand-corner {
    top: 24px;
    left: 24px;
  }

  .corner-logo {
    width: 40px;
    height: 40px;
  }

  .corner-title {
    font-size: 20px;
  }

  .corner-subtitle {
    font-size: 11px;
  }

  .login-wrapper {
    padding: 24px;
    padding-top: 100px;
  }

  .login-card {
    padding: 36px 28px;
  }
}
</style>
