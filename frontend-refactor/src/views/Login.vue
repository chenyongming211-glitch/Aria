<!-- src/views/Login.vue - 深色主题登录页 -->
<template>
  <div class="login-container">
    <!-- 动态背景 -->
    <div class="background-layer">
      <div class="noise-overlay"></div>
      <div class="gradient-orb orb-1"></div>
      <div class="gradient-orb orb-2"></div>
      <div class="gradient-orb orb-3"></div>
      <div class="grid-pattern"></div>
    </div>

    <!-- 登录卡片 -->
    <div class="login-wrapper animate-slide-up">
      <div class="login-card">
        <!-- Logo 区域 -->
        <div class="logo-section">
          <div class="logo-container">
            <div class="logo-glow"></div>
            <img src="/aria-3d-cloud.png" alt="Aria Logo" class="logo-image" />
          </div>
          <h1 class="brand-title">
            Aria
            <span class="pro-badge">PRO</span>
          </h1>
          <p class="brand-subtitle">SD-WAN Controller Console</p>
        </div>

        <!-- 登录表单 -->
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
              placeholder="Username"
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
              placeholder="Password"
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
                Remember me
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
              <span v-if="!loading">Sign In</span>
              <span v-else>Signing in...</span>
            </el-button>
          </el-form-item>
        </el-form>

        <!-- 底部信息 -->
        <div class="footer-section">
          <div class="divider"></div>
          <p class="info-text">Aria Decentralized Networking System</p>
          <p class="version-text">v{{ appVersion }}</p>
        </div>
      </div>

      <!-- 装饰元素 -->
      <div class="decoration-line decoration-line-1"></div>
      <div class="decoration-line decoration-line-2"></div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed } from 'vue'
import { useRouter } from 'vue-router'
import { User, Lock } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useUserStore, useAppStore } from '@/stores'

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
        password: creds.password || '',
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

const loginRules = reactive({
  username: [
    { required: true, message: 'Please enter username', trigger: 'blur' },
    { min: 3, max: 50, message: 'Username must be 3-50 characters', trigger: 'blur' }
  ],
  password: [
    { required: true, message: 'Please enter password', trigger: 'blur' },
    { min: 6, message: 'Password must be at least 6 characters', trigger: 'blur' }
  ]
})

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
      if (loginForm.value.rememberMe) {
        localStorage.setItem('aria_remembered_login', JSON.stringify({
          username: loginForm.value.username,
          password: loginForm.value.password
        }))
      } else {
        localStorage.removeItem('aria_remembered_login')
      }

      const tenants = await userStore.loadTenants()
      if (tenants && tenants.length > 0) {
        localStorage.setItem('aria-current-tenant', JSON.stringify(tenants[0]))
      } else {
        localStorage.removeItem('aria-current-tenant')
      }

      router.push('/dashboard')
      ElMessage.success('Login successful!')
    } else {
      ElMessage.error('Login failed, please check your credentials')
    }
  } catch (error) {
    console.error('Login error:', error)
    if (error && typeof error === 'object' && !Array.isArray(error) && Object.keys(error).length > 0) {
      return
    }
    ElMessage.error('Login failed, please try again later')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
/* ============================================
   Login Page Styles (深色主题)
   ============================================ */
.login-container {
  min-height: 100vh;
  display: flex;
  justify-content: center;
  align-items: center;
  position: relative;
  background: var(--aria-sidebar-bg);
  overflow: hidden;
}

/* ============================================
   Background Layer
   ============================================ */
.background-layer {
  position: absolute;
  inset: 0;
  overflow: hidden;
  z-index: 0;
}

.noise-overlay {
  position: absolute;
  inset: 0;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noise'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noise)'/%3E%3C/svg%3E");
  opacity: 0.03;
  pointer-events: none;
}

.grid-pattern {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(var(--aria-dark-border-primary) 1px, transparent 1px),
    linear-gradient(90deg, var(--aria-dark-border-primary) 1px, transparent 1px);
  background-size: 60px 60px;
  opacity: 0.1;
  mask-image: radial-gradient(circle at center, black 0%, transparent 70%);
}

.gradient-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  opacity: 0.4;
  animation: float 20s ease-in-out infinite;
}

.orb-1 {
  width: 600px;
  height: 600px;
  top: -200px;
  left: -200px;
  background: radial-gradient(circle, rgba(59, 130, 246, 0.4) 0%, transparent 70%);
  animation-delay: 0s;
}

.orb-2 {
  width: 500px;
  height: 500px;
  bottom: -150px;
  right: -150px;
  background: radial-gradient(circle, rgba(34, 197, 94, 0.3) 0%, transparent 70%);
  animation-delay: -7s;
}

.orb-3 {
  width: 400px;
  height: 400px;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  background: radial-gradient(circle, rgba(6, 182, 212, 0.3) 0%, transparent 70%);
  animation-delay: -14s;
}

@keyframes float {
  0%, 100% { transform: translate(0, 0) scale(1); }
  25% { transform: translate(20px, -20px) scale(1.05); }
  50% { transform: translate(-10px, 10px) scale(0.95); }
  75% { transform: translate(-20px, -10px) scale(1.02); }
}

/* ============================================
   Login Wrapper
   ============================================ */
.login-wrapper {
  position: relative;
  z-index: 10;
  width: 100%;
  max-width: 480px;
  padding: 20px;
}

/* ============================================
   Login Card (深色玻璃态)
   ============================================ */
.login-card {
  padding: 48px;
  position: relative;
  overflow: hidden;
  background: rgba(15, 23, 42, 0.75);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: var(--aria-radius-xl);
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.3);
}

/* ============================================
   Logo Section
   ============================================ */
.logo-section {
  text-align: center;
  margin-bottom: 40px;
}

.logo-container {
  display: inline-flex;
  position: relative;
  margin-bottom: 20px;
}

.logo-glow {
  position: absolute;
  inset: -20px;
  background: radial-gradient(circle, rgba(59, 130, 246, 0.3) 0%, transparent 70%);
  filter: blur(20px);
  animation: glow 3s ease-in-out infinite;
}

@keyframes glow {
  0%, 100% { opacity: 0.5; }
  50% { opacity: 1; }
}

.logo-image {
  width: 72px;
  height: 72px;
  position: relative;
  z-index: 1;
}

.brand-title {
  font-size: 36px;
  font-weight: 700;
  margin: 0;
  letter-spacing: -0.5px;
  background: linear-gradient(135deg, var(--aria-primary) 0%, var(--aria-primary-light) 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  color: var(--aria-primary);
}

.pro-badge {
  font-size: 12px;
  padding: 4px 10px;
  background: rgba(59, 130, 246, 0.2);
  border: 1px solid rgba(59, 130, 246, 0.3);
  border-radius: var(--radius-full);
  margin-left: 8px;
  letter-spacing: 1px;
  font-weight: 600;
  -webkit-text-fill-color: var(--aria-primary);
  box-shadow: 0 2px 8px rgba(59, 130, 246, 0.2);
}

.brand-subtitle {
  color: var(--aria-dark-text-muted);
  font-size: 14px;
  margin: 12px 0 0 0;
  font-weight: 400;
  letter-spacing: 0.3px;
}

/* ============================================
   Login Form
   ============================================ */
.login-form {
  margin-bottom: 24px;
}

/* Input Styles (深色) */
:deep(.login-input .el-input__wrapper) {
  background: rgba(0, 0, 0, 0.3);
  border: 1px solid var(--aria-dark-border-primary);
  box-shadow: none;
  transition: all var(--aria-transition-base);
  padding: 4px 16px;
  border-radius: var(--aria-radius-md);
}

:deep(.login-input .el-input__wrapper:hover) {
  border-color: var(--aria-dark-border-hover);
  background: rgba(0, 0, 0, 0.4);
}

:deep(.login-input .el-input__wrapper.is-focus) {
  border-color: var(--aria-primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
}

:deep(.login-input .el-input__inner) {
  background: transparent;
  color: var(--aria-dark-text-primary);
  font-size: 15px;
  padding: 8px 0;
}

:deep(.login-input .el-input__inner::placeholder) {
  color: var(--aria-dark-text-muted);
}

.input-icon {
  color: var(--aria-dark-text-muted);
  font-size: 18px;
  transition: color var(--aria-transition-fast);
}

:deep(.login-input .el-input__wrapper:hover) .input-icon,
:deep(.login-input .el-input__wrapper.is-focus) .input-icon {
  color: var(--aria-primary);
}

/* Form Footer */
.form-footer {
  display: flex;
  justify-content: flex-start;
}

:deep(.login-checkbox .el-checkbox__label) {
  color: var(--aria-dark-text-secondary);
  font-size: 14px;
}

:deep(.login-checkbox .el-checkbox__input.is-checked .el-checkbox__inner) {
  background-color: var(--aria-primary);
  border-color: var(--aria-primary);
}

/* ============================================
   Footer Section
   ============================================ */
.footer-section {
  text-align: center;
  padding-top: 24px;
}

.divider {
  height: 1px;
  background: var(--aria-dark-border-primary);
  margin-bottom: 20px;
}

.info-text {
  color: var(--aria-dark-text-muted);
  font-size: 13px;
  margin: 0 0 8px 0;
}

.version-text {
  color: var(--aria-dark-text-disabled);
  font-size: 12px;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  margin: 0;
  letter-spacing: 0.5px;
}

/* ============================================
   Decoration Lines
   ============================================ */
.decoration-line {
  position: absolute;
  width: 1px;
  height: 100px;
  background: linear-gradient(180deg, transparent 0%, var(--aria-dark-border-primary) 50%, transparent 100%);
  opacity: 0.5;
}

.decoration-line-1 {
  left: 0;
  top: 50%;
  transform: translateY(-50%);
}

.decoration-line-2 {
  right: 0;
  top: 50%;
  transform: translateY(-50%);
}

/* ============================================
   Responsive
   ============================================ */
@media (max-width: 640px) {
  .login-wrapper {
    padding: 16px;
  }

  .login-card {
    padding: 32px 24px;
  }

  .logo-image {
    width: 60px;
    height: 60px;
  }

  .brand-title {
    font-size: 28px;
  }

  .form-footer {
    flex-direction: column;
    gap: 12px;
    align-items: flex-start;
  }
}
</style>
