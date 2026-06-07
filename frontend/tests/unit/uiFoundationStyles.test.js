import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const mainJs = readFileSync(resolve(__dirname, '../../src/main.js'), 'utf8')

describe('UI foundation styles', () => {
  it('keeps fixed table action columns on the themed table surface', () => {
    expect(mainJs).toContain('.el-table .el-table-fixed-column--right')
    expect(mainJs).toContain('.el-table .el-table-fixed-column--left')
    expect(mainJs).toContain('background: var(--aria-content-bg-secondary);')
  })

  it('gives table link action buttons a visible themed surface', () => {
    expect(mainJs).toContain('.el-table .el-button.is-link')
    expect(mainJs).toContain('background: var(--aria-content-bg-tertiary);')
    expect(mainJs).toContain('rgba(37, 99, 235, 0.08)')
    expect(mainJs).toContain('rgba(239, 68, 68, 0.08)')
  })
})
