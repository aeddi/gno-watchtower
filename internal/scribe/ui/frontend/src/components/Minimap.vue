<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useTimelineStore } from '@/stores/timeline'
import { api } from '@/api/client'
import type { ScribeEvent } from '@/types'

interface Bin {
    from: Date
    to: Date
    count: number
    topSeverity: 'warning' | 'error' | 'critical' | 'none'
}

const store = useTimelineStore()
const events = ref<ScribeEvent[]>([])

const range = computed(() => {
    const to = store.at ?? new Date()
    const from = new Date(to.getTime() - 7 * 86400_000)
    return { from, to }
})

const bins = computed<Bin[]>(() => {
    const { from, to } = range.value
    const total = to.getTime() - from.getTime()
    const N = 80
    const binMs = total / N
    const out: Bin[] = []
    for (let i = 0; i < N; i++) {
        const f = new Date(from.getTime() + i * binMs)
        const t = new Date(from.getTime() + (i + 1) * binMs)
        const inBin = events.value.filter((e) => {
            const et = new Date(e.time).getTime()
            return et >= f.getTime() && et < t.getTime()
        })
        let top: Bin['topSeverity'] = 'none'
        for (const e of inBin) {
            if (e.severity === 'critical') {
                top = 'critical'
                break
            }
            if (e.severity === 'error' && top !== 'critical') top = 'error'
            if (e.severity === 'warning' && top === 'none') top = 'warning'
        }
        out.push({ from: f, to: t, count: inBin.length, topSeverity: top })
    }
    return out
})

const SEV_COLOR: Record<string, string> = {
    warning: '#f59e0b',
    error: '#ef4444',
    critical: '#ef4444',
    none: '#1f2937',
}

async function reload() {
    const r = await api.listEvents({
        kind: 'diagnostic.*',
        from: range.value.from,
        to: range.value.to,
        limit: 1000,
    })
    events.value = r.events
}

onMounted(reload)
watch([() => store.at, () => store.subjects], reload, { deep: true })

function maxCount(): number {
    return Math.max(1, ...bins.value.map((b) => b.count))
}

function jumpTo(b: Bin) {
    const mid = new Date((b.from.getTime() + b.to.getTime()) / 2)
    store.setAt(mid)
}
</script>

<template>
    <div class="minimap">
        <span class="label">last 7 days</span>
        <div class="strip">
            <div
                v-for="(b, i) in bins"
                :key="i"
                data-testid="minimap-bar"
                class="bar"
                :style="{
                    height: `${(b.count / maxCount()) * 100}%`,
                    background: SEV_COLOR[b.topSeverity],
                }"
                :title="`${b.count} events @ ${b.from.toLocaleTimeString()}`"
                @click="jumpTo(b)"
            />
        </div>
    </div>
</template>

<style scoped>
.minimap {
    padding: 0.4rem 0.8rem;
    border-bottom: 1px solid #30363d;
    background: #0d1117;
}
.label {
    color: #7d8590;
    font-size: 0.65rem;
}
.strip {
    display: flex;
    align-items: flex-end;
    height: 32px;
    gap: 1px;
    margin-top: 0.2rem;
}
.bar {
    flex: 1;
    cursor: pointer;
    min-height: 1px;
    transition: opacity 0.1s;
}
.bar:hover {
    opacity: 0.7;
}
</style>
