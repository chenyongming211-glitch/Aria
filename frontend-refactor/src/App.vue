<!-- src/App.vue -->
<template>
  <div id="app">
    <el-config-provider :locale="locale" :size="componentSize">
      <router-view />
    </el-config-provider>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { ElConfigProvider } from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import en from 'element-plus/es/locale/lang/en'
import { useAppStore } from '@/stores'

const appStore = useAppStore()

const locale = computed(() => {
  const lang = appStore.lang || 'zh'
  return lang === 'zh' ? zhCn : en
})

const componentSize = computed(() => {
  return 'default' // You can make this configurable as well
})
</script>

<style>
#app {
  font-family: Avenir, Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  height: 100vh;
}
</style>