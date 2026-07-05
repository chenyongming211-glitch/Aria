import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import router from '@/router'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const srcRoot = resolve(__dirname, '../../src')

function readSource(path: string): string {
  return readFileSync(resolve(srcRoot, path), 'utf8')
}

describe('UI polish guardrails', () => {
  it('keeps the old AI assistant out of the v0.1.0 navigation surface', () => {
    expect(router.getRoutes().some((route) => route.name === 'AiAssistant')).toBe(false)

    const layoutSource = readSource('components/layout/Layout.vue')
    expect(layoutSource).not.toContain("nav.aiAssistant")
    expect(layoutSource).not.toContain("/ai-assistant")
  })

  it('does not expose Ask AI actions before Hermes replaces the legacy AI flow', () => {
    const monitoringSource = readSource('views/Monitoring.vue')
    const nodeDetailSource = readSource('views/NodeMonitorDetail.vue')

    expect(monitoringSource).not.toContain('askAIForAlert')
    expect(nodeDetailSource).not.toContain('askAIForContext')
    expect(monitoringSource).not.toContain("monitoringPage.askAI")
    expect(nodeDetailSource).not.toContain("monitoringPage.askAI")
  })

  it('keeps auth pages free of decorative glow and gradient-orb treatments', () => {
    for (const path of ['views/Login.vue', 'views/ChangePassword.vue']) {
      const source = readSource(path)
      expect(source).not.toMatch(/gradient-orb|orb-\d|glow-pulse|avatar-glow/)
      expect(source).not.toMatch(/radial-gradient|linear-gradient|backdrop-filter|filter:\s*blur/)
      expect(source).not.toMatch(/0 25px 60px|0 6px 20px|box-shadow:\s*0 0/)
    }
  })

  it('uses dense console framing instead of decorative hero or logo glow chrome', () => {
    const dashboardSource = readSource('views/Dashboard.vue')
    const layoutSource = readSource('components/layout/Layout.vue')

    expect(dashboardSource).not.toContain('page-hero')
    expect(dashboardSource).not.toMatch(/stat-card-\w+ \.progress-fill \{ background: linear-gradient/)
    expect(layoutSource).not.toContain('logo-glow')
    expect(layoutSource).not.toMatch(/linear-gradient|radial-gradient/)
  })
})
