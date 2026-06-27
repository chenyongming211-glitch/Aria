const MIN_WIDTH = 720
const MIN_HEIGHT = 520
const MIN_PADDING_X = 140
const MIN_PADDING_Y = 110

export interface TopologyChartSize {
  width: number
  height: number
}

export interface TopologyNode {
  id: string
  x?: number
  y?: number
  [key: string]: unknown
}

export interface TopologyLink {
  source: string
  target: string
  [key: string]: unknown
}

export type PositionedTopologyNode<T extends TopologyNode = TopologyNode> = T & {
  x: number
  y: number
}

const normalizeSize = (size: Partial<TopologyChartSize> = {}): TopologyChartSize => ({
  width: Math.max(Number(size.width) || 0, MIN_WIDTH),
  height: Math.max(Number(size.height) || 0, MIN_HEIGHT)
})

export const getTopologyChartSize = (
  element: { clientWidth?: number; clientHeight?: number } | null | undefined
): TopologyChartSize => normalizeSize({
  width: element?.clientWidth,
  height: element?.clientHeight
})

export const computeTopologyNodePositions = <T extends TopologyNode>(
  nodes: readonly T[] | null | undefined = [],
  size: Partial<TopologyChartSize> = {}
): Array<PositionedTopologyNode<T>> => {
  const normalizedNodes = Array.isArray(nodes) ? nodes : []
  if (normalizedNodes.length === 0) return []

  const { width, height } = normalizeSize(size)
  const centerX = width / 2
  const centerY = height / 2
  const paddingX = Math.max(MIN_PADDING_X, Math.min(width * 0.2, 220))
  const paddingY = Math.max(MIN_PADDING_Y, Math.min(height * 0.24, 170))
  const radiusX = Math.max(80, (width - paddingX * 2) / 2)
  const radiusY = Math.max(70, (height - paddingY * 2) / 2)

  if (normalizedNodes.length === 1) {
    return [{
      ...normalizedNodes[0],
      x: Math.round(centerX),
      y: Math.round(centerY)
    } as PositionedTopologyNode<T>]
  }

  if (normalizedNodes.length === 2) {
    return normalizedNodes.map((node, index) => ({
      ...node,
      x: Math.round(centerX + (index === 0 ? -radiusX : radiusX)),
      y: Math.round(centerY)
    }) as PositionedTopologyNode<T>)
  }

  return normalizedNodes.map((node, index) => {
    const angle = -Math.PI / 2 + (index * 2 * Math.PI / normalizedNodes.length)
    return {
      ...node,
      x: Math.round(centerX + Math.cos(angle) * radiusX),
      y: Math.round(centerY + Math.sin(angle) * radiusY)
    } as PositionedTopologyNode<T>
  })
}
