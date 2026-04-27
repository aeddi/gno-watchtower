// Tiny duration parser for URL params like "12h", "30m", "5s".
// Returns milliseconds; throws on invalid input.
export function parseDuration(s: string): number {
    const m = /^(\d+(?:\.\d+)?)(ms|s|m|h|d)$/.exec(s)
    if (!m) throw new Error(`invalid duration: ${s}`)
    const n = parseFloat(m[1])
    switch (m[2]) {
        case 'ms':
            return n
        case 's':
            return n * 1_000
        case 'm':
            return n * 60_000
        case 'h':
            return n * 3_600_000
        case 'd':
            return n * 86_400_000
    }
    throw new Error(`unreachable: ${s}`)
}

export function formatDuration(ms: number): string {
    if (ms < 1000) return `${ms}ms`
    const s = ms / 1000
    if (s < 60) return `${s}s`
    const m = s / 60
    if (m < 60) return `${m}m`
    const h = m / 60
    if (h < 24) return `${h}h`
    return `${h / 24}d`
}
