# Aria UI/UX Design System

## 概述

Aria 是一个企业级 SD-WAN 控制台，采用现代化深色主题设计。本文档定义了完整的设计系统，确保整个应用保持一致性和专业的外观。

## 设计原则

1. **企业级专业性** - 保持简洁、高效、可信赖的外观
2. **现代化深色主题** - 减少眼部疲劳，提升可读性
3. **一致的交互反馈** - 所有交互都有明确的视觉反馈
4. **无障碍访问** - 符合 WCAG AA 标准，支持键盘导航
5. **性能优先** - 使用高效的动画和过渡效果

---

## 色彩系统

### 主色调

| 用途 | 变量 | Hex 值 | RGB |
|------|--------|---------|-----|
| Primary | `--aria-primary` | #3B82F6 | rgb(59, 130, 246) |
| Primary Light | `--aria-primary-light` | #60A5FA | rgb(96, 165, 250) |
| Primary Dark | `--aria-primary-dark` | #2563EB | rgb(37, 99, 235) |

### 辅助色

| 用途 | 变量 | Hex 值 | RGB |
|------|--------|---------|-----|
| Success | `--aria-success` | #22C55E | rgb(34, 197, 94) |
| Warning | `--aria-warning` | #F59E0B | rgb(245, 158, 11) |
| Danger | `--aria-danger` | #EF4444 | rgb(239, 68, 68) |
| Info | `--aria-info` | #06B6D4 | rgb(6, 182, 212) |

### 背景色（深色模式）

| 层级 | 变量 | Hex 值 |
|------|--------|---------|
| Primary | `--aria-bg-primary` | #020617 |
| Secondary | `--aria-bg-secondary` | #0F172A |
| Tertiary | `--aria-bg-tertiary` | #1E293B |
| Elevated | `--aria-bg-elevated` | rgba(30, 41, 59, 0.8) |

### 背景色（浅色模式）

| 层级 | 变量 | Hex 值 |
|------|--------|---------|
| Primary | `--aria-bg-primary` | #F8FAFC |
| Secondary | `--aria-bg-secondary` | #FFFFFF |
| Tertiary | `--aria-bg-tertiary` | #F1F5F9 |

### 文字颜色（深色模式）

| 用途 | 变量 | Hex 值 |
|------|--------|---------|
| Primary | `--aria-text-primary` | #F8FAFC |
| Secondary | `--aria-text-secondary` | #CBD5E1 |
| Muted | `--aria-text-muted` | #64748B |
| Disabled | `--aria-text-disabled` | #475569 |

### 文字颜色（浅色模式）

| 用途 | 变量 | Hex 值 |
|------|--------|---------|
| Primary | `--aria-text-primary` | #0F172A |
| Secondary | `--aria-text-secondary` | #475569 |
| Muted | `--aria-text-muted` | #94A3B8 |
| Disabled | `--aria-text-disabled` | #CBD5E1 |

### 边框颜色

| 状态 | 变量 | 值 |
|------|--------|-----|
| Primary | `--aria-border-primary` | rgba(148, 163, 184, 0.2) |
| Hover | `--aria-border-hover` | rgba(59, 130, 246, 0.3) |
| Focus | `--aria-border-focus` | rgba(59, 130, 246, 0.5) |

---

## 字体系统

### 主字体

```css
font-family: 'Plus Jakarta Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
```

### 字体大小

| 用途 | 大小 | 字重 | 行高 |
|------|------|--------|------|
| Display / H1 | 28px | 700 | 1.3 |
| H2 | 24px | 600 | 1.3 |
| H3 | 20px | 600 | 1.3 |
| H4 | 18px | 600 | 1.3 |
| H5 | 16px | 600 | 1.3 |
| H6 | 14px | 600 | 1.3 |
| Body | 14px | 400 | 1.6 |
| Small | 12px | 400 | 1.5 |
| Tiny | 11px | 400 | 1.5 |

### 代码字体

```css
font-family: 'JetBrains Mono', 'Fira Code', 'Courier New', monospace;
```

---

## 间距系统

| 名称 | 变量 | 值 |
|------|--------|-----|
| Extra Small | `--aria-spacing-xs` | 4px |
| Small | `--aria-spacing-sm` | 8px |
| Medium | `--aria-spacing-md` | 16px |
| Large | `--aria-spacing-lg` | 24px |
| Extra Large | `--aria-spacing-xl` | 32px |
| 2X Large | `--aria-spacing-2xl` | 48px |

---

## 圆角系统

| 名称 | 变量 | 值 |
|------|--------|-----|
| Small | `--aria-radius-sm` | 6px |
| Medium | `--aria-radius` | 10px |
| Default | `--aria-radius-md` | 12px |
| Large | `--aria-radius-lg` | 16px |
| Extra Large | `--aria-radius-xl` | 20px |
| 2X Large | `--aria-radius-2xl` | 24px |
| Full | `--aria-radius-full` | 9999px |

---

## 阴影系统

| 名称 | 变量 | 值 |
|------|--------|-----|
| Small | `--aria-shadow-sm` | 0 1px 2px rgba(0, 0, 0, 0.1) |
| Medium | `--aria-shadow` | 0 4px 6px rgba(0, 0, 0, 0.1) |
| Large | `--aria-shadow-md` | 0 10px 15px rgba(0, 0, 0, 0.1) |
| Extra Large | `--aria-shadow-lg` | 0 20px 25px rgba(0, 0, 0, 0.1) |
| XL | `--aria-shadow-xl` | 0 25px 50px rgba(0, 0, 0, 0.25) |
| Glow | `--aria-shadow-glow` | 0 0 20px rgba(59, 130, 246, 0.3) |
| Glow Success | `--aria-shadow-glow-success` | 0 0 20px rgba(34, 197, 94, 0.3) |

---

## 过渡效果

| 名称 | 变量 | 值 |
|------|--------|-----|
| Fast | `--aria-transition-fast` | 150ms cubic-bezier(0.4, 0, 0.2, 1) |
| Base | `--aria-transition-base` | 200ms cubic-bezier(0.4, 0, 0.2, 1) |
| Slow | `--aria-transition-slow` | 300ms cubic-bezier(0.4, 0, 0.2, 1) |

---

## 组件样式

### 玻璃态卡片 (Glass Card)

```css
.glass-card {
  background: var(--aria-glass-bg);
  backdrop-filter: blur(var(--aria-glass-blur));
  border: 1px solid var(--aria-glass-border);
  border-radius: var(--aria-radius-lg);
  box-shadow: var(--aria-shadow-lg);
  transition: all var(--aria-transition-base);
}

.glass-card:hover {
  border-color: var(--aria-border-hover);
  box-shadow: var(--aria-shadow-xl);
  transform: translateY(-2px);
}
```

### 渐变按钮 (Gradient Button)

```css
.aria-btn-primary {
  background: linear-gradient(135deg, var(--aria-primary) 0%, var(--aria-primary-dark) 100%);
  border: none;
  color: white;
  border-radius: var(--aria-radius);
  padding: 12px 24px;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--aria-transition-base);
  box-shadow: 0 4px 14px rgba(59, 130, 246, 0.3);
}

.aria-btn-primary:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(59, 130, 246, 0.4);
}
```

### 状态徽章 (Status Badge)

```css
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: var(--radius-full);
  font-size: 12px;
  font-weight: 500;
  transition: all var(--aria-transition-fast);
}

.status-badge.online {
  background: rgba(34, 197, 94, 0.15);
  color: var(--aria-success);
  border: 1px solid rgba(34, 197, 94, 0.2);
}

.status-badge.offline {
  background: rgba(239, 68, 68, 0.15);
  color: var(--aria-danger);
  border: 1px solid rgba(239, 68, 68, 0.2);
}

.status-badge.maintenance {
  background: rgba(245, 158, 11, 0.15);
  color: var(--aria-warning);
  border: 1px solid rgba(245, 158, 11, 0.2);
}
```

### 统计卡片 (Stat Card)

```css
.stat-card {
  position: relative;
  overflow: hidden;
  padding: var(--aria-spacing-lg);
  border-radius: var(--aria-radius-lg);
  background: var(--aria-bg-secondary);
  border: 1px solid var(--aria-border-primary);
  transition: all var(--aria-transition-base);
}

.stat-card:hover {
  transform: translateY(-4px);
  box-shadow: var(--aria-shadow-lg);
  border-color: var(--aria-border-hover);
}

.stat-card .stat-value {
  font-size: 32px;
  font-weight: 700;
  color: var(--aria-text-primary);
  margin-bottom: 4px;
  letter-spacing: -0.5px;
}

.stat-card .stat-label {
  font-size: 14px;
  font-weight: 500;
  color: var(--aria-text-secondary);
  margin-bottom: 8px;
}

.stat-card .stat-change {
  font-size: 12px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.stat-card .stat-change.positive {
  color: var(--aria-success);
}

.stat-card .stat-change.negative {
  color: var(--aria-danger);
}
```

---

## 动画效果

### 淡入动画

```css
@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

.animate-fade-in {
  animation: fadeIn 0.3s ease-out;
}
```

### 向上滑动动画

```css
@keyframes slideUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.animate-slide-up {
  animation: slideUp 0.4s ease-out;
}
```

### 缩放动画

```css
@keyframes scaleIn {
  from {
    opacity: 0;
    transform: scale(0.9);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}

.animate-scale-in {
  animation: scaleIn 0.3s ease-out;
}
```

### 浮动动画

```css
@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-10px); }
}

.animate-float {
  animation: float 4s ease-in-out infinite;
}
```

### 脉冲动画（状态指示器）

```css
@keyframes pulse-dot {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.6; }
}

.status-dot {
  animation: pulse-dot 2s ease-in-out infinite;
}
```

---

## 页面过渡

```css
.page-fade-enter-active,
.page-fade-leave-active {
  transition: all 0.2s ease;
}

.page-fade-enter-from {
  opacity: 0;
  transform: translateY(10px);
}

.page-fade-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}
```

---

## 响应式断点

| 断点 | 设备 | 最小宽度 |
|--------|------|----------|
| xs | 小屏手机 | 0px |
| sm | 大屏手机 | 640px |
| md | 平板 | 768px |
| lg | 小屏桌面 | 1024px |
| xl | 大屏桌面 | 1280px |
| 2xl | 超大屏 | 1536px |

---

## 无障碍访问

### 焦点状态

```css
:focus-visible {
  outline: 2px solid var(--aria-primary);
  outline-offset: 2px;
}
```

### 减少动画

```css
@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}
```

### 触摸目标大小

最小触摸目标尺寸为 44x44px，确保移动设备上的可用性。

---

## 滚动条样式

```css
::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

::-webkit-scrollbar-track {
  background: var(--aria-bg-secondary);
  border-radius: var(--aria-radius);
}

::-webkit-scrollbar-thumb {
  background: var(--aria-border-hover);
  border-radius: var(--aria-radius);
  transition: background var(--aria-transition-fast);
}

::-webkit-scrollbar-thumb:hover {
  background: var(--aria-primary);
}
```

---

## 使用指南

### 1. 引入样式

```javascript
// main.js
import './styles/global.css'
```

### 2. 使用 CSS 变量

```css
.my-component {
  background: var(--aria-bg-secondary);
  color: var(--aria-text-primary);
  border-radius: var(--aria-radius-lg);
  padding: var(--aria-spacing-lg);
  box-shadow: var(--aria-shadow-md);
  transition: all var(--aria-transition-base);
}
```

### 3. 使用组件类

```vue
<template>
  <div class="glass-card">
    <!-- 内容 -->
  </div>
</template>

<style scoped>
.glass-card {
  /* 继承全局玻璃态样式 */
}
</style>
```

---

## 图标系统

使用 Element Plus 图标库，保持一致的图标风格。

```vue
<template>
  <el-icon><Monitor /></el-icon>
  <el-icon><CircleCheck /></el-icon>
  <el-icon><TrendCharts /></el-icon>
</template>
```

---

## 字体安装

```bash
npm install @fontsource-variable/plus-jakarta-sans
```

```javascript
// main.js
import '@fontsource-variable/plus-jakarta-sans'
```

---

## 注意事项

1. **始终使用 CSS 变量** - 确保主题切换的一致性
2. **遵循间距系统** - 使用预定义的间距变量
3. **保持过渡一致性** - 使用预定义的过渡变量
4. **无障碍优先** - 确保所有交互元素都有焦点状态
5. **性能考虑** - 避免过度使用复杂动画
6. **移动优先** - 确保响应式设计从移动端开始

---

## 更新日志

- **2024-02-23** - 初始设计系统发布
  - 完整的色彩系统
  - 字体和排版规范
  - 间距和圆角系统
  - 组件样式库
  - 动画和过渡效果
  - 无障碍访问指南
