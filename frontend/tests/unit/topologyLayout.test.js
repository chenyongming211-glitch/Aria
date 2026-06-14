import { describe, it, expect } from 'vitest'
import { computeTopologyNodePositions } from '@/utils/topologyLayout'

describe('topology layout', () => {
  it('centers a single node in the visible chart area', () => {
    const [node] = computeTopologyNodePositions([{ id: 'node-1' }], {
      width: 1000,
      height: 600
    })

    expect(node).toMatchObject({
      id: 'node-1',
      x: 500,
      y: 300
    })
  })

  it('keeps two-node topologies away from clipped edges', () => {
    const nodes = computeTopologyNodePositions([
      { id: 'node-82-156-48-111' },
      { id: 'node-43-143-245-123' }
    ], {
      width: 1000,
      height: 600
    })

    expect(nodes).toHaveLength(2)
    expect(nodes[0].x).toBeGreaterThanOrEqual(140)
    expect(nodes[1].x).toBeLessThanOrEqual(860)
    expect(nodes[0].y).toBe(300)
    expect(nodes[1].y).toBe(300)
    expect(nodes[1].x - nodes[0].x).toBeGreaterThan(500)
  })

  it('normalizes hidden or zero-sized containers before computing positions', () => {
    const nodes = computeTopologyNodePositions([
      { id: 'node-1' },
      { id: 'node-2' }
    ], {
      width: 0,
      height: 0
    })

    expect(nodes[0].x).toBeGreaterThan(0)
    expect(nodes[0].y).toBeGreaterThan(0)
    expect(nodes[1].x).toBeGreaterThan(nodes[0].x)
  })
})
