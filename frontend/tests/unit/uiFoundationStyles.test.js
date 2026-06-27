import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const mainSource = readFileSync(resolve(__dirname, '../../src/main.ts'), 'utf8')

describe('UI foundation styles', () => {
  it('keeps fixed table action columns on the themed table surface', () => {
    expect(mainSource).toContain('--el-table-header-bg-color: var(--aria-content-bg-tertiary);')
    expect(mainSource).toContain('--el-table-tr-bg-color: var(--aria-content-bg-secondary);')
    expect(mainSource).toContain('.el-table .el-table__header-wrapper tr th.el-table-fixed-column--right')
    expect(mainSource).toContain('.el-table .el-table__header-wrapper tr th.el-table-fixed-column--left')
    expect(mainSource).toContain('.el-table .el-table__body-wrapper tr td.el-table-fixed-column--right')
    expect(mainSource).toContain('.el-table .el-table__body-wrapper tr td.el-table-fixed-column--left')
    expect(mainSource).toContain('background: var(--el-table-header-bg-color);')
    expect(mainSource).toContain('background: var(--el-table-tr-bg-color);')
  })

  it('gives table link action buttons a visible themed surface', () => {
    expect(mainSource).toContain('.el-table .el-button.is-link')
    expect(mainSource).toContain('background: var(--aria-content-bg-tertiary);')
    expect(mainSource).toContain('rgba(37, 99, 235, 0.08)')
    expect(mainSource).toContain('rgba(239, 68, 68, 0.08)')
  })
})
