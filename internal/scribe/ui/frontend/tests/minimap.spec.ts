import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import Minimap from '@/components/Minimap.vue'
import { useTimelineStore } from '@/stores/timeline'

vi.mock('@/api/client', () => ({
    api: {
        listEvents: vi.fn().mockResolvedValue({
            events: Array.from({ length: 50 }, (_, i) => ({
                event_id: `01J${i}`,
                cluster_id: 'c1',
                kind: 'diagnostic.bft_at_risk_v1',
                subject: '_chain',
                time: new Date(Date.now() - i * 60_000).toISOString(),
                ingest_time: new Date(Date.now() - i * 60_000).toISOString(),
                severity: 'error',
                state: 'open',
                payload: {},
            })),
            next_cursor: '',
        }),
    },
}))

describe('Minimap', () => {
    beforeEach(() => setActivePinia(createPinia()))

    it('renders aggregated bins from events', async () => {
        const w = mount(Minimap)
        await flushPromises()
        const bars = w.findAll('[data-testid="minimap-bar"]')
        expect(bars.length).toBeGreaterThan(0)
    })

    it('clicking a bin sets the cursor', async () => {
        const store = useTimelineStore()
        const setAt = vi.spyOn(store, 'setAt')
        const w = mount(Minimap)
        await flushPromises()
        const bars = w.findAll('[data-testid="minimap-bar"]')
        if (bars.length > 0) {
            await bars[0].trigger('click')
            expect(setAt).toHaveBeenCalled()
        }
    })
})
