import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import NetworkGraph from '@/components/NetworkGraph.vue'
import { useTimelineStore } from '@/stores/timeline'

const setNodes = vi.fn()
const setEdges = vi.fn()
const setLayout = vi.fn()
const setSelection = vi.fn()
const destroy = vi.fn()
let onHandler: any = () => {}

vi.mock('@/renderers/graph/CytoscapeGraphRenderer', () => ({
    CytoscapeGraphRenderer: vi.fn().mockImplementation(() => ({
        setNodes,
        setEdges,
        setLayout,
        setSelection,
        destroy,
        on: (h: any) => {
            onHandler = h
        },
    })),
}))

vi.mock('@/api/client', () => ({
    api: {
        listSubjects: vi
            .fn()
            .mockResolvedValue(['_chain', 'node-1', 'node-2', 'node-3']),
        listEvents: vi.fn().mockResolvedValue({ events: [], next_cursor: '' }),
        getState: vi.fn().mockResolvedValue({ structured: {} }),
    },
}))

describe('NetworkGraph', () => {
    beforeEach(() => {
        setActivePinia(createPinia())
        setNodes.mockClear()
        setEdges.mockClear()
        setLayout.mockClear()
    })

    it('mounts and provisions nodes (excluding _chain)', async () => {
        mount(NetworkGraph)
        await flushPromises()
        expect(setNodes).toHaveBeenCalled()
        const nodes = setNodes.mock.calls[0][0]
        expect(nodes.map((n: any) => n.id)).not.toContain('_chain')
    })

    it('node-click event toggles subject in store', async () => {
        const store = useTimelineStore()
        mount(NetworkGraph)
        await flushPromises()
        onHandler({ type: 'node-click', id: 'node-1', multi: false })
        expect(store.subjects).toEqual(['node-1'])
    })

    it('Cmd-click adds to multi-selection', async () => {
        const store = useTimelineStore()
        mount(NetworkGraph)
        await flushPromises()
        onHandler({ type: 'node-click', id: 'node-1', multi: false })
        onHandler({ type: 'node-click', id: 'node-3', multi: true })
        expect(store.subjects).toEqual(['node-1', 'node-3'])
    })

    it('background-click clears selection', async () => {
        const store = useTimelineStore()
        store.setSubjects(['node-1'])
        mount(NetworkGraph)
        await flushPromises()
        onHandler({ type: 'background-click' })
        expect(store.subjects).toEqual([])
    })
})
