import cytoscape, { type Core } from 'cytoscape'
import type {
    GraphRenderer,
    GraphEvent,
    GraphNode,
    GraphEdge,
} from './GraphRenderer'
import type { GraphLayout } from '@/types'

const SEVERITY_COLOR: Record<string, string> = {
    warning: '#f59e0b',
    error: '#ef4444',
    critical: '#ef4444',
    none: '#3fb950',
}

const LAYOUT_NAME: Record<GraphLayout, string> = {
    circle: 'circle',
    force: 'cose',
    table: 'grid', // 'table' is a UI-level mode that swaps the renderer entirely; if it's
    // ever requested at the renderer level (shouldn't happen normally),
    // fall back to a grid layout so we don't blow up.
}

export class CytoscapeGraphRenderer implements GraphRenderer {
    private cy: Core
    private listeners: Array<(e: GraphEvent) => void> = []

    constructor(container: HTMLElement) {
        this.cy = cytoscape({
            container,
            style: [
                {
                    selector: 'node',
                    style: {
                        'background-color': 'data(color)' as any,
                        label: 'data(label)' as any,
                        'font-size': 10,
                        color: '#c9d1d9',
                        'text-valign': 'center' as any,
                        'border-width': 2,
                        'border-color': '#0d1117',
                        width: 36,
                        height: 36,
                    },
                },
                {
                    selector: 'node[shield]',
                    style: {
                        'border-width': 4,
                        'border-color': '#58a6ff',
                    },
                },
                {
                    selector: 'node:selected',
                    style: {
                        'border-width': 4,
                        'border-color': '#58a6ff',
                    },
                },
                {
                    selector: 'edge',
                    style: {
                        width: 2,
                        'line-color': '#3fb950',
                        opacity: 0.6,
                    },
                },
            ],
            layout: { name: 'circle' },
        })

        this.cy.on('tap', 'node', (e) => {
            const id = e.target.id()
            const orig = e.originalEvent as MouseEvent | undefined
            const multi = orig?.metaKey || orig?.ctrlKey || false
            this.emit({ type: 'node-click', id, multi })
        })
        this.cy.on('mouseover', 'node', (e) =>
            this.emit({ type: 'node-hover', id: e.target.id() })
        )
        this.cy.on('tap', (e) => {
            if (e.target === this.cy) this.emit({ type: 'background-click' })
        })
    }

    setNodes(nodes: GraphNode[]) {
        this.cy.elements('node').remove()
        nodes.forEach((n) =>
            this.cy.add({
                group: 'nodes',
                data: {
                    id: n.id,
                    label: n.label ?? n.id,
                    color: SEVERITY_COLOR[n.severity],
                    shield: n.behindSentry,
                },
            })
        )
    }

    setEdges(edges: GraphEdge[]) {
        this.cy.elements('edge').remove()
        edges.forEach((e) =>
            this.cy.add({
                group: 'edges',
                data: {
                    id: `${e.source}-${e.target}`,
                    source: e.source,
                    target: e.target,
                },
            })
        )
    }

    setLayout(layout: GraphLayout) {
        const name = LAYOUT_NAME[layout]
        this.cy.layout({ name }).run()
    }

    setSelection(ids: string[]) {
        this.cy.nodes().unselect()
        ids.forEach((id) => this.cy.$(`#${id}`)?.select())
    }

    on(handler: (e: GraphEvent) => void) {
        this.listeners.push(handler)
    }

    destroy() {
        this.cy.destroy()
        this.listeners = []
    }

    private emit(e: GraphEvent) {
        for (const l of this.listeners) l(e)
    }
}
