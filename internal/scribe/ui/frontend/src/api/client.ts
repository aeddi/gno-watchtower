import type { ScribeEvent, RuleMeta, Severity, DiagnosticState } from '@/types'

export interface ListEventsParams {
    kind?: string
    subject?: string
    severity?: Severity[]
    state?: DiagnosticState
    from?: Date
    to?: Date
    limit?: number
    cursor?: string
}

export interface ListEventsResult {
    events: ScribeEvent[]
    next_cursor: string
}

export class ApiClient {
    constructor(private baseUrl: string = '') {}

    private url(
        path: string,
        params?: Record<string, string | undefined>
    ): string {
        const base =
            this.baseUrl ||
            (typeof window !== 'undefined'
                ? window.location.origin
                : 'http://localhost')
        const u = new URL(path, base)
        if (params) {
            for (const [k, v] of Object.entries(params)) {
                if (v !== undefined && v !== '') u.searchParams.set(k, v)
            }
        }
        // When baseUrl is empty (default for SPA), return relative path + query.
        return this.baseUrl ? u.toString() : `${u.pathname}${u.search}`
    }

    async listEvents(p: ListEventsParams): Promise<ListEventsResult> {
        const url = this.url('/api/events', {
            kind: p.kind,
            subject: p.subject,
            severity: p.severity?.join(','),
            state: p.state,
            from: p.from?.toISOString(),
            to: p.to?.toISOString(),
            limit: p.limit?.toString(),
            cursor: p.cursor,
        })
        const r = await fetch(url)
        if (!r.ok) throw new Error(`listEvents: ${r.status} ${r.statusText}`)
        return r.json()
    }

    async getState(
        subject: string,
        at?: Date
    ): Promise<{ structured: any; events_replayed?: number }> {
        const url = this.url('/api/state', { subject, at: at?.toISOString() })
        const r = await fetch(url)
        if (!r.ok) throw new Error(`getState: ${r.status} ${r.statusText}`)
        return r.json()
    }

    async listRules(): Promise<RuleMeta[]> {
        const r = await fetch(this.url('/api/rules'))
        if (!r.ok) throw new Error(`listRules: ${r.status} ${r.statusText}`)
        return r.json()
    }

    async listSubjects(): Promise<string[]> {
        const r = await fetch(this.url('/api/subjects'))
        if (!r.ok) throw new Error(`listSubjects: ${r.status} ${r.statusText}`)
        const data = await r.json()
        return data.subjects ?? []
    }

    async listSamples(p: {
        subject: string
        from: Date
        to: Date
        step?: string
    }): Promise<{ buckets: any[] }> {
        const url = this.url('/api/samples', {
            subject: p.subject,
            from: p.from.toISOString(),
            to: p.to.toISOString(),
            step: p.step,
        })
        const r = await fetch(url)
        if (!r.ok) throw new Error(`listSamples: ${r.status} ${r.statusText}`)
        return r.json()
    }

    async getRuleDoc(kind: string): Promise<string> {
        const r = await fetch(this.url(`/docs/rules/${kind}`))
        if (!r.ok) throw new Error(`getRuleDoc(${kind}): ${r.status}`)
        return r.text()
    }
}

// Default singleton for app-wide use.
export const api = new ApiClient()
