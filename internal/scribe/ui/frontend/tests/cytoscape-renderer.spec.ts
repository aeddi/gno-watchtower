import { describe, it, expect, beforeEach, vi } from 'vitest'
import { CytoscapeGraphRenderer } from '@/renderers/graph/CytoscapeGraphRenderer'

const cyMock = {
    add: vi.fn(),
    remove: vi.fn(),
    $: vi.fn(() => ({ select: vi.fn() })),
    nodes: vi.fn(() => ({ unselect: vi.fn() })),
    layout: vi.fn(() => ({ run: vi.fn() })),
    destroy: vi.fn(),
    on: vi.fn(),
    elements: vi.fn(() => ({ remove: vi.fn() })),
}
vi.mock('cytoscape', () => ({
    default: vi.fn().mockImplementation(() => cyMock),
}))

describe('CytoscapeGraphRenderer', () => {
    let container: HTMLElement
    beforeEach(() => {
        container = document.createElement('div')
        Object.values(cyMock).forEach(
            (v) => typeof v === 'function' && (v as any).mockClear?.()
        )
    })

    it('initializes a Cytoscape instance', () => {
        const r = new CytoscapeGraphRenderer(container)
        expect(r).toBeDefined()
        r.destroy()
    })

    it('setLayout maps "force" → cose and "circle" → circle', () => {
        const r = new CytoscapeGraphRenderer(container)
        r.setLayout('force')
        expect(cyMock.layout).toHaveBeenCalledWith(
            expect.objectContaining({ name: 'cose' })
        )
        r.setLayout('circle')
        expect(cyMock.layout).toHaveBeenCalledWith(
            expect.objectContaining({ name: 'circle' })
        )
        r.destroy()
    })
})
