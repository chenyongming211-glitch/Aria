// src/main.js
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'

// 引入全局样式
import './styles/global.css'
import './styles/element-variables.scss'

// 引入字体
import '@fontsource-variable/plus-jakarta-sans'

import App from './App.vue'
import router from './router'
import useAppStore from './stores/app'

const app = createApp(App)
const pinia = createPinia()

// Register all icons
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(pinia)
app.use(router)
app.use(ElementPlus)

app.mount('#app')

// Fetch version from API
const appStore = useAppStore()
appStore.fetchVersion()

// Add global styles for Element Plus components override (浅色主题)
const style = document.createElement('style')
style.textContent = `
  /* === Element Plus 组件覆盖 - 浅色主题 === */

  /* 卡片样式 */
  .el-card {
    background: var(--aria-content-bg-secondary);
    border: 1px solid var(--aria-border-primary);
    border-radius: var(--aria-radius-lg);
    box-shadow: var(--aria-shadow-sm);
    transition: all var(--aria-transition-base);
  }

  .el-card__header {
    padding: 20px 24px;
    border-bottom: 1px solid var(--aria-border-primary);
    background: transparent;
  }

  .el-card__body {
    padding: 24px;
  }

  /* 表单样式 */
  .el-form-item__label {
    font-weight: 500;
    color: var(--aria-text-secondary);
    font-size: 14px;
  }

  /* 表格样式 */
  .el-table {
    --el-table-bg-color: transparent;
    --el-table-tr-bg-color: transparent;
    --el-table-row-hover-bg-color: var(--aria-content-bg-tertiary);
    --el-table-border-color: var(--aria-border-primary);
    border-radius: var(--aria-radius-md);
    overflow: hidden;
  }

  .el-table__header-wrapper {
    border-radius: var(--aria-radius-md) var(--aria-radius-md) 0 0;
  }

  .el-table__body-wrapper {
    border-radius: 0 0 var(--aria-radius-md) var(--aria-radius-md);
  }

  .el-table th.el-table__cell {
    background: var(--aria-content-bg-tertiary);
    color: var(--aria-text-secondary);
    font-weight: 600;
    font-size: 13px;
    border-bottom: 1px solid var(--aria-border-primary);
  }

  .el-table td.el-table__cell {
    color: var(--aria-text-primary);
    border-bottom: 1px solid var(--aria-border-primary);
  }

  .el-table--striped .el-table__body tr.el-table__row--striped td.el-table__cell {
    background: var(--aria-content-bg-secondary);
  }

  .el-table--enable-row-hover .el-table__body tr:hover > td.el-table__cell {
    background: var(--aria-content-bg-tertiary);
  }

  /* 按钮样式 */
  .el-button {
    border-radius: var(--aria-radius);
    font-weight: 500;
    transition: all var(--aria-transition-base);
  }

  .el-button--primary {
    background: linear-gradient(135deg, var(--aria-primary) 0%, var(--aria-primary-dark) 100%);
    border: none;
    box-shadow: 0 2px 8px rgba(59, 130, 246, 0.25);
  }

  .el-button--primary:hover,
  .el-button--primary:focus {
    background: linear-gradient(135deg, var(--aria-primary-light) 0%, var(--aria-primary) 100%);
    box-shadow: 0 4px 12px rgba(59, 130, 246, 0.35);
    transform: translateY(-1px);
  }

  .el-button--primary:active {
    transform: translateY(0);
  }

  .el-button.is-link {
    color: var(--aria-text-secondary);
    background: transparent;
    border: none;
    padding: 8px 12px;
  }

  .el-button.is-link:hover {
    color: var(--aria-primary);
  }

  /* 输入框样式 */
  .el-input__wrapper {
    background: var(--aria-content-bg-secondary);
    border: 1px solid var(--aria-border-primary);
    border-radius: var(--aria-radius-md);
    box-shadow: none;
    transition: all var(--aria-transition-base);
  }

  .el-input__wrapper:hover {
    border-color: var(--aria-border-hover);
  }

  .el-input__wrapper.is-focus {
    border-color: var(--aria-primary);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .el-input__inner {
    background: transparent;
    color: var(--aria-text-primary);
    font-size: 14px;
  }

  .el-input__inner::placeholder {
    color: var(--aria-text-muted);
  }

  .el-input__prefix-inner,
  .el-input__suffix-inner {
    color: var(--aria-text-muted);
  }

  /* Textarea */
  .el-textarea__inner {
    background: var(--aria-content-bg-secondary);
    border: 1px solid var(--aria-border-primary);
    border-radius: var(--aria-radius-md);
    color: var(--aria-text-primary);
    font-size: 14px;
  }

  .el-textarea__inner::placeholder {
    color: var(--aria-text-muted);
  }

  .el-textarea__inner:focus {
    border-color: var(--aria-primary);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  /* 下拉菜单 */
  .el-select-dropdown {
    background: var(--aria-content-bg-secondary);
    border: 1px solid var(--aria-border-primary);
    box-shadow: var(--aria-shadow-lg);
    border-radius: var(--aria-radius-md);
  }

  .el-select-dropdown__item {
    color: var(--aria-text-secondary);
  }

  .el-select-dropdown__item:hover,
  .el-select-dropdown__item.is-selected {
    background: var(--aria-content-bg-tertiary);
    color: var(--aria-primary);
  }

  /* 对话框 */
  .el-dialog {
    background: var(--aria-content-bg-secondary);
    border: 1px solid var(--aria-border-primary);
    border-radius: var(--aria-radius-lg);
    box-shadow: var(--aria-shadow-xl);
  }

  .el-dialog__header {
    border-bottom: 1px solid var(--aria-border-primary);
    padding: 20px 24px;
  }

  .el-dialog__title {
    color: var(--aria-text-primary);
    font-weight: 600;
    font-size: 18px;
  }

  .el-dialog__body {
    color: var(--aria-text-secondary);
    padding: 24px;
  }

  .el-dialog__footer {
    border-top: 1px solid var(--aria-border-primary);
    padding: 16px 24px;
  }

  /* 分页 */
  .el-pagination {
    --el-pagination-bg-color: transparent;
    --el-pagination-text-color: var(--aria-text-secondary);
    --el-pagination-border-color: var(--aria-border-primary);
  }

  .el-pagination button,
  .el-pager li {
    background: var(--aria-content-bg-secondary);
    border: 1px solid var(--aria-border-primary);
    color: var(--aria-text-secondary);
    border-radius: var(--aria-radius-sm);
    transition: all var(--aria-transition-fast);
  }

  .el-pagination button:hover,
  .el-pager li:hover {
    border-color: var(--aria-border-hover);
    color: var(--aria-text-primary);
  }

  .el-pager li.is-active {
    background: var(--aria-primary);
    border-color: var(--aria-primary);
    color: white;
  }

  /* 标签 */
  .el-tag {
    border-radius: var(--radius-full);
    font-weight: 500;
    border: none;
  }

  .el-tag--success {
    background: rgba(34, 197, 94, 0.1);
    color: #22C55E;
  }

  .el-tag--warning {
    background: rgba(245, 158, 11, 0.1);
    color: #F59E0B;
  }

  .el-tag--danger {
    background: rgba(239, 68, 68, 0.1);
    color: #EF4444;
  }

  .el-tag--info {
    background: rgba(59, 130, 246, 0.1);
    color: #3B82F6;
  }

  .el-tag--primary {
    background: rgba(59, 130, 246, 0.1);
    color: #3B82F6;
  }

  /* 复选框 */
  .el-checkbox__label {
    color: var(--aria-text-secondary);
  }

  .el-checkbox__input.is-checked .el-checkbox__inner {
    background-color: var(--aria-primary);
    border-color: var(--aria-primary);
  }

  .el-checkbox__input.is-checked + .el-checkbox__label {
    color: var(--aria-text-primary);
  }

  /* 进度条 */
  .el-progress {
    --el-progress-bg-color: var(--aria-content-bg-tertiary);
  }

  /* Popover */
  .el-popover {
    background: var(--aria-content-bg-secondary);
    border: 1px solid var(--aria-border-primary);
    box-shadow: var(--aria-shadow-lg);
    border-radius: var(--aria-radius-md);
  }

  .el-popover__title {
    color: var(--aria-text-primary);
    font-weight: 600;
  }

  /* Skeleton */
  .el-skeleton {
    --el-skeleton-circle-radius: 50%;
  }

  /* Menu for dropdown */
  .el-menu--horizontal {
    border-bottom: 1px solid var(--aria-border-primary);
  }

  .el-menu--horizontal .el-menu-item {
    color: var(--aria-text-secondary);
  }

  .el-menu--horizontal .el-menu-item:hover {
    background: var(--aria-content-bg-tertiary);
    color: var(--aria-text-primary);
  }

  .el-menu--horizontal .el-menu-item.is-active {
    background: var(--aria-content-bg-tertiary);
    color: var(--aria-primary);
  }
`
document.head.appendChild(style)
