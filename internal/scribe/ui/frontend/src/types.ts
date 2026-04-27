export type Severity = 'warning' | 'error' | 'critical'
export type DiagnosticState = 'open' | 'recovered'
export type Step = '1s' | '5s' | '30s' | '5m' | '1h'
export type GraphLayout = 'circle' | 'force' | 'table'
export type ThemeMode = 'light' | 'dark' | 'system'

export interface Filters {
    severity: Severity[]
    state: DiagnosticState[]
    kinds: string[]
}

export interface ScribeEvent {
    event_id: string
    cluster_id: string
    time: string // RFC3339Nano
    ingest_time: string
    kind: string
    subject: string
    severity?: Severity
    state?: DiagnosticState
    recovers?: string
    payload: Record<string, unknown>
    provenance?: {
        type: string
        rule?: string
        doc_ref?: string
        source_event_ids?: string[]
        linked_signals?: Array<{
            type: 'loki' | 'vm'
            query: string
            url?: string
            from: string
            to: string
        }>
    }
}

export interface ChainState {
    block_height: number | null
    online_count: number | null
    valset_size: number | null
    online_voting_power: number | null
    total_voting_power: number | null
    open_diagnostics_count: number
}

export interface ValidatorState {
    validator: string
    block_height: number | null
    voting_power: number | null
    peer_count_in: number | null
    peer_count_out: number | null
    catching_up: boolean | null
    moniker: string | null
    behind_sentry: boolean | null
    open_diagnostics_count: number
}

export interface RuleMeta {
    code: string
    version: number
    kind: string
    severity: Severity
    kinds: string[]
    tick_period_seconds: number
    description: string
    doc_ref: string
    enabled: boolean
}
