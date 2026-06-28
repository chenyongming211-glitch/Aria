<template>
  <div class="change-password-container">
    <div class="background-layer">
      <div class="grid-pattern"></div>
    </div>

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

    <main class="change-password-main">
      <section class="password-panel">
        <div class="panel-header">
          <el-icon class="panel-icon"><Lock /></el-icon>
          <h2>{{ t('changePassword.title') }}</h2>
          <p>{{ t('changePassword.subtitle') }}</p>
        </div>

        <el-form
          ref="passwordFormRef"
          :model="passwordForm"
          :rules="passwordRules"
          label-position="top"
          class="password-form"
          @keyup.enter="handleChangePassword"
        >
          <el-form-item :label="t('changePassword.currentPassword')" prop="oldPassword">
            <el-input
              v-model="passwordForm.oldPassword"
              type="password"
              autocomplete="current-password"
              :placeholder="t('changePassword.currentPasswordPlaceholder')"
              show-password
              size="large"
            />
          </el-form-item>

          <el-form-item :label="t('changePassword.newPassword')" prop="newPassword">
            <el-input
              v-model="passwordForm.newPassword"
              type="password"
              autocomplete="new-password"
              :placeholder="t('changePassword.newPasswordPlaceholder')"
              show-password
              size="large"
            />
          </el-form-item>

          <el-form-item :label="t('changePassword.confirmPassword')" prop="confirmPassword">
            <el-input
              v-model="passwordForm.confirmPassword"
              type="password"
              autocomplete="new-password"
              :placeholder="t('changePassword.confirmPasswordPlaceholder')"
              show-password
              size="large"
            />
          </el-form-item>

          <el-button
            type="primary"
            size="large"
            class="submit-button"
            :loading="loading"
            @click="handleChangePassword"
          >
            {{ t('changePassword.submit') }}
          </el-button>
        </el-form>
      </section>
    </main>
  </div>
</template>

<script setup>
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Lock } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/stores'
import { t } from '@/i18n'

const router = useRouter()
const userStore = useUserStore()

const passwordFormRef = ref(null)
const loading = ref(false)

const passwordForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const validateConfirmPassword = (rule, value, callback) => {
  if (value !== passwordForm.newPassword) {
    callback(new Error(t('changePassword.mismatch')))
    return
  }
  callback()
}

const passwordRules = computed(() => ({
  oldPassword: [
    { required: true, message: t('changePassword.currentRequired'), trigger: 'blur' }
  ],
  newPassword: [
    { required: true, message: t('changePassword.newRequired'), trigger: 'blur' },
    { min: 6, message: t('changePassword.newLength'), trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: t('changePassword.confirmRequired'), trigger: 'blur' },
    { validator: validateConfirmPassword, trigger: 'blur' }
  ]
}))

const handleChangePassword = async () => {
  if (!passwordFormRef.value) return

  try {
    await passwordFormRef.value.validate()
  } catch {
    return
  }

  loading.value = true
  try {
    const result = await userStore.changePassword(passwordForm.oldPassword, passwordForm.newPassword)
    if (!result.success) {
      ElMessage.error(result.message || t('changePassword.failed'))
      return
    }

    ElMessage.success(t('changePassword.success'))
    userStore.logout()
    router.replace('/login')
  } catch (error) {
    ElMessage.error(`${t('changePassword.failed')}: ${error.message || t('changePassword.unknownError')}`)
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.change-password-container {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
  background: var(--aria-sidebar-bg);
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
  letter-spacing: 0;
}

.pro-badge {
  font-size: 10px;
  padding: 3px 8px;
  background: linear-gradient(135deg, rgba(59, 130, 246, 0.6) 0%, rgba(6, 182, 212, 0.6) 100%);
  border-radius: 4px;
  letter-spacing: 0;
  font-weight: 600;
}

.corner-subtitle {
  color: rgba(255, 255, 255, 0.5);
  font-size: 13px;
  margin: 4px 0 0 0;
  font-weight: 400;
}

.change-password-main {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 112px 24px 40px;
  position: relative;
  z-index: 10;
}

.password-panel {
  width: 100%;
  max-width: 430px;
  padding: 44px 38px;
  background: rgba(15, 23, 42, 0.78);
  backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  box-shadow: 0 25px 60px rgba(0, 0, 0, 0.5);
}

.panel-header {
  text-align: center;
  margin-bottom: 32px;
}

.panel-icon {
  width: 56px;
  height: 56px;
  padding: 14px;
  color: #fff;
  background: rgba(59, 130, 246, 0.18);
  border: 1px solid rgba(59, 130, 246, 0.32);
  border-radius: 8px;
  font-size: 28px;
  margin-bottom: 18px;
}

.panel-header h2 {
  color: #fff;
  font-size: 24px;
  font-weight: 600;
  margin: 0 0 8px;
}

.panel-header p {
  color: rgba(255, 255, 255, 0.55);
  font-size: 14px;
  margin: 0;
}

.password-form {
  margin: 0;
}

:deep(.el-form-item__label) {
  color: rgba(255, 255, 255, 0.85);
  font-size: 13px;
  font-weight: 500;
}

:deep(.el-input__wrapper) {
  background: rgba(0, 0, 0, 0.35);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  box-shadow: none;
}

:deep(.el-input__wrapper:hover) {
  border-color: rgba(59, 130, 246, 0.4);
}

:deep(.el-input__wrapper.is-focus) {
  border-color: var(--aria-primary);
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12);
}

:deep(.el-input__inner) {
  color: #fff;
}

:deep(.el-input__inner::placeholder) {
  color: rgba(255, 255, 255, 0.35);
}

.submit-button {
  width: 100%;
  height: 46px;
  margin-top: 6px;
  font-size: 15px;
  font-weight: 600;
  border-radius: 8px;
}

@media (max-width: 700px) {
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

  .password-panel {
    padding: 34px 24px;
  }
}
</style>
