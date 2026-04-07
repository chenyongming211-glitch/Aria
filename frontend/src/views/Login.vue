<!-- src/views/Login.vue - 深色主题登录页 -->
<template>
  <div class="login-container">
    <!-- 动态背景 -->
    <div class="background-layer">
      <div class="grid-pattern"></div>
      <div class="gradient-orb orb-1"></div>
      <div class="gradient-orb orb-2"></div>
    </div>

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
      <!-- 左侧大 Logo -->
      <div class="logo-side">
        <div class="logo-wrapper">
          <img src="/aria-3d-cloud.png" alt="Aria Logo" class="big-logo" />
        </div>
        <div class="features-tags">
          <span class="feature-tag">高性能</span>
          <span class="feature-tag">零信任</span>
          <span class="feature-tag">智能路由</span>
        </div>
      </div>

      <!-- 右侧登录表单 -->
      <div class="login-side">
        <div class="login-card">
          <div class="login-header">
            <h2>欢迎回来</h2>
            <p>请登录您的账号</p>
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
                placeholder="用户名"
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
                placeholder="密码"
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
                  记住我
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
                <span v-if="!loading">登录</span>
                <span v-else>登录中...</span>
              </el-button>
            </el-form-item>
          </el-form>

          <div class="footer-section">
            <p class="version-text">Aria Controller v{{ appVersion }}</p>
          </div>
        </div>
      </div>
    </div>

    <!-- 强制修改密码弹窗 -->
    <el-dialog
      v-model="showPasswordDialog"
      width="380px"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :show-close="false"
      :header="false"
      class="password-dialog"
    >
      <div class="password-dialog-content">
        <el-icon class="warning-icon"><WarningFilled /></el-icon>
        <h3 class="dialog-title">首次登录必须修改密码</h3>
        <p class="dialog-message">为了保障您的账号安全，请设置新的登录密码</p>
        
        <el-form
          ref="passwordFormRef"
          :model="passwordForm"
          :rules="passwordRules"
          label-position="top"
        >
          <el-form-item label="当前密码" prop="oldPassword">
            <el-input
              v-model="passwordForm.oldPassword"
              type="password"
              placeholder="请输入当前密码"
              show-password
              size="large"
            />
          </el-form-item>
          
          <el-form-item label="新密码" prop="newPassword">
            <el-input
              v-model="passwordForm.newPassword"
              type="password"
              placeholder="请输入新密码（至少6位）"
              show-password
              size="large"
            />
          </el-form-item>
          
          <el-form-item label="确认新密码" prop="confirmPassword">
            <el-input
              v-model="passwordForm.confirmPassword"
              type="password"
              placeholder="请再次输入新密码"
              show-password
              size="large"
            />
          </el-form-item>
        </el-form>
      </div>
      
      <template #footer>
        <el-button
          type="primary"
          size="large"
          class="change-password-btn"
          :loading="passwordLoading"
          @click="handleChangePassword"
        >
          确认修改
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed } from 'vue'
import { useRouter } from 'vue-router'
import { User, Lock, WarningFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useUserStore, useAppStore } from '@/stores'

const router = useRouter()
const userStore = useUserStore()
const appStore = useAppStore()

const appVersion = computed(() => appStore.version)

// 强制修改密码弹窗
const showPasswordDialog = ref(false)
const passwordLoading = ref(false)
const passwordFormRef = ref(null)
const passwordForm = ref({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const validateConfirmPassword = (rule, value, callback) => {
  if (value !== passwordForm.value.newPassword) {
    callback(new Error('两次输入的密码不一致'))
  } else {
    callback()
  }
}

const passwordRules = reactive({
  oldPassword: [
    { required: true, message: '请输入当前密码', trigger: 'blur' }
  ],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, message: '密码至少6位', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    { validator: validateConfirmPassword, trigger: 'blur' }
  ]
})

const handleChangePassword = async () => {
  if (!passwordFormRef.value) {
    console.error('passwordFormRef is null')
    return
  }
  
  try {
    await passwordFormRef.value.validate()
  } catch (validationError) {
    console.error('Validation error:', validationError)
    return
  }
  
  passwordLoading.value = true
  
  try {
    const result = await userStore.changePassword(
      passwordForm.value.oldPassword,
      passwordForm.value.newPassword
    )
    
    console.log('Change password result:', result)
    
    if (result.success) {
      ElMessage.success('密码修改成功，请重新登录')
      showPasswordDialog.value = false
      
      userStore.logout()
      loginForm.value.password = ''
    } else {
      ElMessage.error(result.message || '密码修改失败')
    }
  } catch (error) {
    console.error('Change password error:', error)
    ElMessage.error('密码修改失败: ' + (error.message || '未知错误'))
  } finally {
    passwordLoading.value = false
  }
}

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

const loginRules = reactive({
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 50, message: '用户名长度需为 3-50 个字符', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, message: '密码至少 6 个字符', trigger: 'blur' }
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
      // 检查是否需要强制修改密码
      if (result.requirePasswordChange) {
        showPasswordDialog.value = true
        ElMessage.warning('首次登录必须修改密码')
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
      if (tenants && tenants.length > 0) {
        localStorage.setItem('aria-current-tenant', JSON.stringify(tenants[0]))
      } else {
        localStorage.removeItem('aria-current-tenant')
      }

      router.push('/dashboard')
      ElMessage.success('登录成功')
    } else {
      ElMessage.error('登录失败，请检查凭据')
    }
  } catch (error) {
    console.error('Login error:', error)
    if (error && typeof error === 'object' && !Array.isArray(error) && Object.keys(error).length > 0) {
      return
    }
    ElMessage.error('登录失败，请稍后重试')
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
  overflow: hidden;
}

.background-layer {
  position: absolute;
  inset: 0;
  z-index: 0;
}

.grid-pattern {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(var(--aria-dark-border-primary) 1px, transparent 1px),
    linear-gradient(90deg, var(--aria-dark-border-primary) 1px, transparent 1px);
  background-size: 60px 60px;
  opacity: 0.05;
}

.gradient-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(120px);
  opacity: 0.25;
}

.orb-1 {
  width: 600px;
  height: 600px;
  top: -200px;
  right: -100px;
  background: radial-gradient(circle, rgba(59, 130, 246, 0.4) 0%, transparent 70%);
}

.orb-2 {
  width: 500px;
  height: 500px;
  bottom: -150px;
  left: -100px;
  background: radial-gradient(circle, rgba(34, 197, 94, 0.3) 0%, transparent 70%);
}

.brand-corner {
  position: absolute;
  top: 40px;
  left: 60px;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 16px;
}

.corner-logo {
  width: 48px;
  height: 48px;
  object-fit: contain;
}

.corner-info {
  display: flex;
  flex-direction: column;
}

.corner-title {
  font-size: 26px;
  font-weight: 700;
  margin: 0;
  color: #fff;
  display: flex;
  align-items: center;
  gap: 10px;
  letter-spacing: -0.5px;
}

.pro-badge {
  font-size: 10px;
  padding: 3px 8px;
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.6) 0%, rgba(6, 182, 212, 0.6) 100%);
  border-radius: 4px;
  letter-spacing: 1px;
  font-weight: 600;
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
  gap: 100px;
  padding: 40px;
  position: relative;
  z-index: 10;
}

.logo-side {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 32px;
}

.logo-wrapper {
  position: relative;
  animation: float 6s ease-in-out infinite;
}

.logo-wrapper::before {
  content: '';
  position: absolute;
  inset: -40px;
  background: radial-gradient(circle, rgba(59, 130, 246, 0.25) 0%, transparent 70%);
  filter: blur(30px);
  animation: glow-pulse 3s ease-in-out infinite;
  z-index: -1;
}

@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-12px); }
}

@keyframes glow-pulse {
  0%, 100% { opacity: 0.6; transform: scale(1); }
  50% { opacity: 1; transform: scale(1.1); }
}

.big-logo {
  width: 480px;
  height: 480px;
  object-fit: contain;
  mask-image: radial-gradient(circle, white 40%, transparent 100%);
  -webkit-mask-image: radial-gradient(circle, white 40%, transparent 100%);
}

.features-tags {
  display: flex;
  gap: 16px;
}

.feature-tag {
  padding: 8px 20px;
  background: rgba(59, 130, 246, 0.15);
  border: 1px solid rgba(59, 130, 246, 0.3);
  border-radius: 20px;
  color: rgba(255, 255, 255, 0.8);
  font-size: 14px;
  font-weight: 500;
}

.login-side {
  display: flex;
  justify-content: center;
}

.login-card {
  width: 100%;
  max-width: 400px;
  padding: 48px 40px;
  background: rgba(15, 23, 42, 0.7);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 20px;
  box-shadow: 0 25px 60px rgba(0, 0, 0, 0.5);
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
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12);
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
  border-radius: 10px;
  background: linear-gradient(135deg, var(--aria-primary) 0%, #3b82f6 100%);
  border: none;
  transition: all 0.25s ease;
}

:deep(.submit-button:hover) {
  transform: translateY(-1px);
  box-shadow: 0 6px 20px rgba(59, 130, 246, 0.35);
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
  letter-spacing: 0.5px;
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
    flex-direction: column;
    gap: 40px;
    padding: 24px;
    padding-top: 100px;
  }

  .big-logo {
    width: 280px;
    height: 280px;
  }

  .features-tags {
    flex-wrap: wrap;
    justify-content: center;
    gap: 10px;
  }

  .feature-tag {
    padding: 6px 14px;
    font-size: 12px;
  }

  .login-card {
    padding: 36px 28px;
  }
}

.password-dialog-content {
  text-align: center;
  padding: 20px 0;
}

.warning-icon {
  font-size: 56px;
  color: #f59e0b;
  margin-bottom: 20px;
  display: block;
  animation: pulse 2s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.1); opacity: 0.8; }
}

.dialog-title {
  color: #fff;
  font-size: 22px;
  font-weight: 600;
  margin-bottom: 12px;
}

.dialog-message {
  color: rgba(255, 255, 255, 0.6);
  font-size: 14px;
  margin-bottom: 28px;
  line-height: 1.6;
}

:deep(.el-dialog.password-dialog) {
  background: linear-gradient(145deg, rgba(15, 23, 42, 0.98) 0%, rgba(30, 41, 59, 0.98) 100%) !important;
  border: 1px solid rgba(59, 130, 246, 0.3);
  border-radius: 20px;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.6), 0 0 40px rgba(59, 130, 246, 0.15) !important;
  max-width: 400px !important;
  width: 90% !important;
  overflow: hidden;
}

:deep(.el-dialog.password-dialog .el-dialog__header) {
  display: none;
}

:deep(.el-dialog.password-dialog .el-dialog__body) {
  background: transparent !important;
  color: #fff;
  padding: 16px 32px 32px;
}

:deep(.el-dialog.password-dialog .el-dialog__footer) {
  background: transparent !important;
  border: none;
  padding: 0 32px 28px;
}

:deep(.el-dialog.password-dialog .el-form-item__label) {
  color: rgba(255, 255, 255, 0.85);
  font-size: 13px;
  font-weight: 500;
}

:deep(.el-dialog.password-dialog .el-form-item) {
  margin-bottom: 20px;
}

:deep(.el-dialog.password-dialog .el-input__wrapper) {
  background: rgba(0, 0, 0, 0.5) !important;
  border: 1px solid rgba(59, 130, 246, 0.2) !important;
  border-radius: 10px;
  box-shadow: none !important;
}

:deep(.el-dialog.password-dialog .el-input__wrapper:hover) {
  border-color: rgba(59, 130, 246, 0.5) !important;
}

:deep(.el-dialog.password-dialog .el-input__wrapper.is-focus) {
  border-color: var(--aria-primary) !important;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.2) !important;
}

:deep(.el-dialog.password-dialog .el-input__inner) {
  color: #fff !important;
}

:deep(.el-dialog.password-dialog .el-input__inner::placeholder) {
  color: rgba(255, 255, 255, 0.4) !important;
}

:deep(.el-dialog.password-dialog .el-button--primary) {
  background: linear-gradient(135deg, var(--aria-primary) 0%, #2563eb 100%);
  border: none;
  border-radius: 10px;
  height: 44px;
  font-size: 15px;
  font-weight: 600;
}

:deep(.el-dialog.password-dialog .el-button--primary:hover) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(59, 130, 246, 0.4);
}
</style>
