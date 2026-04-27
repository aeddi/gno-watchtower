import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import Timeline from '@/components/Timeline.vue'
import { useTimelineStore } from '@/stores/timeline'

const setLanes = vi.fn()
const setItems = vi.fn()
const setStep = vi.fn()
const setCursor = vi.fn()
const destroy = vi.fn()
let onHandler: any = () => {}

vi.mock('@/renderers/timeline/VisTimelineRenderer', () => ({
    VisTimelineRenderer: vi.fn().mockImplementation(() => ({
        setLanes,
        setItems,
        setStep,
        setCursor,
        destroy,
        on: (h: any) => {
            onHandler = h
        },
    })),
}))

vi.mock('@/api/client', () => ({
    api: {
        listEvents: vi.fn().mockResolvedValue({ events: [], next_cursor: '' }),
        listSubjects: vi.fn().mockResolvedValue(['_chain', 'node-1']),
    },
}))

describe('Timeline component', () => {
    beforeEach(() => {
        setActivePinia(createPinia())
        setLanes.mockClear()
        setItems.mockClear()
        setStep.mockClear()
        setCursor.mockClear()
        destroy.mockClear()
    })

    it('mounts and configures the renderer with subjects as lanes', async () => {
        mount(Timeline)
        await flushPromises()
        expect(setLanes).toHaveBeenCalled()
        expect(setItems).toHaveBeenCalled()
        expect(setStep).toHaveBeenCalled()
    })

    it('item-click event sets store.at', async () => {
        const store = useTimelineStore()
        mount(Timeline)
        await flushPromises()
        onHandler({
            type: 'item-click',
            itemId: 'x',
            time: new Date('2026-04-27T12:00:00Z'),
        })
        expect(store.at?.toISOString()).toBe('2026-04-27T12:00:00.000Z')
    })

    it('updates cursor when store.at changes', async () => {
        const store = useTimelineStore()
        mount(Timeline)
        await flushPromises()
        setCursor.mockClear()
        store.setAt(new Date('2026-04-27T13:00:00Z'))
        await flushPromises()
        expect(setCursor).toHaveBeenCalled()
    })
})
