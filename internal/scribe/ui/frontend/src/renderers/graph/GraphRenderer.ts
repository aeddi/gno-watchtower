import type { GraphLayout, Severity } from '@/types'

export interface GraphNode {
    id: string // validator id
    label?: string
    severity: Severity | 'none'
    behindSentry: boolean
}

export interface GraphEdge {
    source: string
    target: string
    weight?: number
}

export type GraphEvent =
    | { type: 'node-click'; id: string; multi: boolean }
    | { type: 'background-click' }
    | { type: 'node-hover'; id: string }

export interface GraphRenderer {
    setNodes(nodes: GraphNode[]): void
    setEdges(edges: GraphEdge[]): void
    setLayout(layout: GraphLayout): void
    setSelection(ids: string[]): void
    on(handler: (e: GraphEvent) => void): void
    destroy(): void
}
