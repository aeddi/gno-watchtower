import { watch } from 'vue'
import type {
    GraphLayout,
    Step,
    Severity,
    DiagnosticState,
    ThemeMode,
} from '@/types'
import type { useTimelineStore } from '@/stores/timeline'
import { parseDuration, formatDuration } from '@/utils/duration'

const DEFAULT_WINDOW_MS = 12 * 3600 * 1000

type Store = ReturnType<typeof useTimelineStore>

const STEPS: Step[] = ['1s', '5s', '30s', '5m', '1h']
const LAYOUTS: GraphLayout[] = ['circle', 'force', 'table']
const SEVERITIES: Severity[] = ['warning', 'error', 'critical']
const STATES: DiagnosticState[] = ['open', 'recovered']
const THEMES: ThemeMode[] = ['light', 'dark', 'system']

function isOneOf<T extends string>(
    v: string | null | undefined,
    opts: readonly T[]
): T | undefined {
    return v && (opts as readonly string[]).includes(v) ? (v as T) : undefined
}

export function storeToParams(s: Store): URLSearchParams {
    const p = new URLSearchParams()
    if (s.at && !isNaN(s.at.getTime())) p.set('at', s.at.toISOString())
    if (s.subjects.length) p.set('subject', s.subjects.join(','))
    if (s.step !== '30s') p.set('step', s.step)
    if (s.graphLayout !== 'circle') p.set('layout', s.graphLayout)
    if (s.window !== DEFAULT_WINDOW_MS)
        p.set('window', formatDuration(s.window))
    if (s.filters.severity.length)
        p.set('severity', s.filters.severity.join(','))
    if (s.filters.state.length) p.set('state', s.filters.state.join(','))
    if (s.filters.kinds.length) p.set('kind', s.filters.kinds.join(','))
    if (s.theme !== 'system') p.set('theme', s.theme)
    return p
}

export function paramsToStore(p: URLSearchParams, s: Store) {
    const at = p.get('at')
    if (at) {
        const d = new Date(at)
        s.setAt(isNaN(d.getTime()) ? null : d)
    } else {
        s.setAt(null)
    }

    const subj = p.get('subject')
    s.setSubjects(subj ? subj.split(',').filter(Boolean) : [])

    const step = isOneOf(p.get('step'), STEPS)
    if (step) s.setStep(step)

    const layout = isOneOf(p.get('layout'), LAYOUTS)
    if (layout) s.setLayout(layout)

    const sev = p.get('severity')
    s.filters.severity = sev
        ? sev
              .split(',')
              .filter((v): v is Severity => SEVERITIES.includes(v as Severity))
        : []
    const st = p.get('state')
    s.filters.state = st
        ? st
              .split(',')
              .filter((v): v is DiagnosticState =>
                  STATES.includes(v as DiagnosticState)
              )
        : []
    const kinds = p.get('kind')
    s.filters.kinds = kinds ? kinds.split(',').filter(Boolean) : []

    const win = p.get('window')
    if (win) {
        try {
            s.setWindow(parseDuration(win))
        } catch {
            // ignore bad input
        }
    }

    const theme = isOneOf(p.get('theme'), THEMES)
    if (theme) s.setTheme(theme)
}

// Initialize the bidirectional binding. Call once at app startup.
export function initUrlSync(s: Store) {
    // hydrate from current URL
    paramsToStore(new URLSearchParams(window.location.search), s)

    // store → URL
    watch(
        () =>
            [
                s.at,
                s.subjects,
                s.step,
                s.graphLayout,
                s.window,
                s.filters,
                s.theme,
            ] as const,
        () => {
            const p = storeToParams(s)
            const qs = p.toString()
            const url = qs
                ? `${window.location.pathname}?${qs}`
                : window.location.pathname
            window.history.replaceState({}, '', url)
        },
        { deep: true }
    )

    // URL → store (popstate, e.g. user clicks back)
    window.addEventListener('popstate', () => {
        paramsToStore(new URLSearchParams(window.location.search), s)
    })
}
