import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DiagnosticItem from '@/components/DiagnosticItem.vue'
import type { ScribeEvent } from '@/types'

const sample: ScribeEvent = {
    event_id: '01J0',
    cluster_id: 'c1',
    time: '2026-04-27T12:34:56Z',
    ingest_time: '2026-04-27T12:34:56Z',
    kind: 'diagnostic.bft_at_risk_v1',
    subject: '_chain',
    severity: 'error',
    state: 'open',
    payload: { online_count: 2, valset_size: 4 },
    provenance: {
        type: 'rule',
        rule: 'diagnostic.bft_at_risk_v1',
        doc_ref: '/docs/rules/diagnostic.bft_at_risk_v1',
        source_event_ids: ['01JC0', '01JC1'],
        linked_signals: [
            {
                type: 'loki',
                query: '{level="error"}',
                from: '2026-04-27T12:30:00Z',
                to: '2026-04-27T12:40:00Z',
            },
        ],
    },
}

describe('DiagnosticItem', () => {
    it('renders collapsed by default with severity + kind + state + subject', () => {
        const w = mount(DiagnosticItem, { props: { event: sample } })
        expect(w.text()).toContain('bft_at_risk_v1')
        expect(w.text().toLowerCase()).toContain('open')
        expect(w.text()).toContain('_chain')
        expect(w.find('[data-testid="expanded"]').exists()).toBe(false)
    })

    it('expands on click and renders linked_signals + source_event_ids', async () => {
        const w = mount(DiagnosticItem, { props: { event: sample } })
        await w.find('[data-testid="header"]').trigger('click')
        expect(w.find('[data-testid="expanded"]').exists()).toBe(true)
        const expanded = w.find('[data-testid="expanded"]').text()
        expect(expanded).toContain('online_count')
        expect(expanded).toContain('loki')
        expect(expanded).toContain('01JC0')
    })

    it('emits select on header click for cursor sync', async () => {
        const w = mount(DiagnosticItem, { props: { event: sample } })
        await w.find('[data-testid="header"]').trigger('click')
        expect(w.emitted('select')).toBeTruthy()
        expect(w.emitted('select')![0][0]).toBe('2026-04-27T12:34:56Z')
    })
})
