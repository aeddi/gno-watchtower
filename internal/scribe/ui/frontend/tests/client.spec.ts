import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ApiClient } from '@/api/client'

describe('ApiClient', () => {
    let originalFetch: typeof fetch

    beforeEach(() => {
        originalFetch = global.fetch
    })
    afterEach(() => {
        global.fetch = originalFetch
    })

    it('listEvents builds query string from params', async () => {
        const fetchMock = vi.fn().mockResolvedValue({
            ok: true,
            json: async () => ({ events: [], next_cursor: '' }),
        })
        global.fetch = fetchMock as any

        const c = new ApiClient('')
        await c.listEvents({
            kind: 'diagnostic.bft_at_risk_v1',
            severity: ['error', 'critical'],
            state: 'open',
            from: new Date('2026-04-27T10:00:00Z'),
            limit: 50,
        })
        const url = fetchMock.mock.calls[0][0] as string
        expect(url).toContain('/api/events')
        expect(url).toContain('kind=diagnostic.bft_at_risk_v1')
        expect(url).toContain('severity=error%2Ccritical')
        expect(url).toContain('state=open')
        expect(url).toContain('from=2026-04-27T10%3A00%3A00.000Z')
        expect(url).toContain('limit=50')
    })

    it('listRules returns parsed array', async () => {
        const fetchMock = vi.fn().mockResolvedValue({
            ok: true,
            json: async () => [
                {
                    code: 'bft_at_risk',
                    version: 1,
                    kind: 'diagnostic.bft_at_risk_v1',
                    severity: 'error',
                    kinds: [],
                    tick_period_seconds: 0,
                    description: '',
                    doc_ref: '/docs/rules/diagnostic.bft_at_risk_v1',
                    enabled: true,
                },
            ],
        })
        global.fetch = fetchMock as any

        const c = new ApiClient('')
        const rules = await c.listRules()
        expect(rules).toHaveLength(1)
        expect(rules[0].kind).toBe('diagnostic.bft_at_risk_v1')
    })

    it('throws on non-2xx', async () => {
        global.fetch = vi
            .fn()
            .mockResolvedValue({
                ok: false,
                status: 500,
                statusText: 'oops',
            }) as any
        const c = new ApiClient('')
        await expect(c.listEvents({})).rejects.toThrow(/500/)
    })
})
