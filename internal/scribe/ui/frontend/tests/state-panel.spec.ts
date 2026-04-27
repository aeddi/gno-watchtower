import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import StatePanel from '@/components/StatePanel.vue'
import { useTimelineStore } from '@/stores/timeline'

vi.mock('@/api/client', () => ({
    api: {
        getState: vi
            .fn()
            .mockImplementation((subject: string) =>
                Promise.resolve({
                    structured: {
                        block_height: subject === 'node-1' ? 100 : 99,
                        voting_power: 10,
                    },
                })
            ),
    },
}))

describe('StatePanel', () => {
    beforeEach(() => setActivePinia(createPinia()))

    it('renders chain state when no subjects selected', async () => {
        const w = mount(StatePanel)
        await flushPromises()
        expect(w.text().toLowerCase()).toContain('chain')
    })

    it('renders single-validator state when one subject selected', async () => {
        const store = useTimelineStore()
        store.setSubjects(['node-1'])
        const w = mount(StatePanel)
        await flushPromises()
        expect(w.text()).toContain('node-1')
        expect(w.text()).toContain('100')
    })

    it('renders comparison with collapsed identical rows', async () => {
        const store = useTimelineStore()
        store.setSubjects(['node-1', 'node-2'])
        const w = mount(StatePanel)
        await flushPromises()
        const html = w.html()
        // voting_power=10 for both → collapsed (single cell, displayed once)
        expect((html.match(/>10</g) ?? []).length).toBeLessThanOrEqual(1)
        // block_height differs → both shown
        expect(html).toContain('100')
        expect(html).toContain('99')
    })
})
