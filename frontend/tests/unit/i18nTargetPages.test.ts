import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import { locales } from '@/i18n'

type LocaleTree = Record<string, unknown>

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const srcRoot = resolve(__dirname, '../../src')

const targetSources = [
  'router/index.ts',
  'components/layout/Layout.vue',
  'views/Dashboard.vue',
  'views/Nodes.vue',
  'views/Monitoring.vue',
  'views/Policies.vue',
  'views/Settings.vue'
]

const literalKeyPatterns = [
  /\bt\(\s*['"`]([^'"`]+)['"`]\s*\)/g,
  /\btitleKey:\s*['"`]([^'"`]+)['"`]/g,
  /\blabelKey:\s*['"`]([^'"`]+)['"`]/g
]

const allowedEnglishCjkKeys = new Set([
  'common.chinese'
])

const cjkPattern = /[\u3400-\u9fff]/

function readSource(path: string): string {
  return readFileSync(resolve(srcRoot, path), 'utf8')
}

function collectLiteralKeys(): string[] {
  const keys = new Set<string>()

  for (const path of targetSources) {
    const source = readSource(path)
    for (const pattern of literalKeyPatterns) {
      for (const match of source.matchAll(pattern)) {
        keys.add(match[1])
      }
    }
  }

  return [...keys].sort()
}

function readLocaleValue(locale: LocaleTree, key: string): unknown {
  return key.split('.').reduce<unknown>((current, segment) => {
    if (!current || typeof current !== 'object') return undefined
    return (current as LocaleTree)[segment]
  }, locale)
}

describe('target page i18n coverage', () => {
  it('defines all literal translation keys used by Dashboard, Nodes, Monitoring, Policy Center, Settings, and Layout', () => {
    const keys = collectLiteralKeys()
    const missing = keys.flatMap((key) => (
      ['en', 'zh'].flatMap((locale) => (
        typeof readLocaleValue(locales[locale] as LocaleTree, key) === 'string'
          ? []
          : [`${locale}:${key}`]
      ))
    ))

    expect(missing).toEqual([])
  })

  it('keeps English target page labels free of Chinese text except intentional language-switch labels', () => {
    const leakedChinese = collectLiteralKeys().flatMap((key) => {
      const value = readLocaleValue(locales.en as LocaleTree, key)
      if (typeof value !== 'string') return []
      if (allowedEnglishCjkKeys.has(key)) return []
      return cjkPattern.test(value) ? [`${key}: ${value}`] : []
    })

    expect(leakedChinese).toEqual([])
  })
})
