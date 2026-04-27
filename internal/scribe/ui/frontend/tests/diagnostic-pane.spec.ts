import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount, flushPromises } from '@vue/test-utils'
import DiagnosticPane from '@/components/DiagnosticPane.vue'
import { useTimelineStore } from '@/stores/timeline'

// Stub the SSE wrapper so the component's live-mode EventStream doesn't
// touch jsdom's missing EventSource global.
vi.mock('@/api/sse', () => ({
    EventStream: vi.fn().mockImplementation(() => ({ close: vi.fn() })),
}))

vi.mock('@/api/client', () => ({
    api: {
        listEvents: vi.fn().mockResolvedValue({
            events: [
                {
                    event_id: '01J0',
                    kind: 'diagnostic.bft_at_risk_v1',
                    subject: '_chain',
                    time: '2026-04-27T12:34:56Z',
                    ingest_time: '2026-04-27T12:34:56Z',
                    cluster_id: 'c1',
                    severity: 'error',
                    state: 'open',
                    payload: { online_count: 2, valset_size: 4 },
                },
                {
                    event_id: '01J1',
                    kind: 'diagnostic.consensus_slow_v1',
                    subject: '_chain',
                    time: '2026-04-27T12:30:00Z',
                    ingest_time: '2026-04-27T12:30:00Z',
                    cluster_id: 'c1',
                    severity: 'warning',
                    state: 'open',
                    payload: { p95_seconds: 8.4 },
                },
            ],
            next_cursor: '',
        }),
    },
}))

describe('DiagnosticPane', () => {
    beforeEach(() => setActivePinia(createPinia()))

    it('lists events from the API on mount', async () => {
        const w = mount(DiagnosticPane)
        await flushPromises()
        expect(w.text()).toContain('bft_at_risk_v1')
        expect(w.text()).toContain('consensus_slow_v1')
    })

    it('emits store.setAt when an item is selected', async () => {
        const store = useTimelineStore()
        const setAt = vi.spyOn(store, 'setAt')
        const w = mount(DiagnosticPane)
        await flushPromises()
        await w.find('[data-testid="header"]').trigger('click')
        expect(setAt).toHaveBeenCalled()
    })
})
