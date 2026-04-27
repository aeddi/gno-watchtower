<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useTimelineStore } from '@/stores/timeline'
import { api } from '@/api/client'
import type { ScribeEvent } from '@/types'
import DiagnosticItem from './DiagnosticItem.vue'

const store = useTimelineStore()
const events = ref<ScribeEvent[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

async function reload() {
    loading.value = true
    error.value = null
    try {
        const r = await api.listEvents({
            kind: 'diagnostic.*',
            subject:
                store.subjects.length === 1 ? store.subjects[0] : undefined,
            severity: store.filters.severity.length
                ? store.filters.severity
                : undefined,
            state:
                store.filters.state.length === 1
                    ? store.filters.state[0]
                    : undefined,
            to: store.at ?? undefined,
            limit: 200,
        })
        // Server returns ASC; we want newest-at-top per ops-log convention.
        events.value = r.events.slice().reverse()
    } catch (e: any) {
        error.value = e?.message ?? String(e)
    } finally {
        loading.value = false
    }
}

onMounted(reload)
watch([() => store.subjects, () => store.filters, () => store.at], reload, {
    deep: true,
})

function onSelect(timeIso: string) {
    store.setAt(new Date(timeIso))
}
</script>

<template>
    <section class="pane">
        <header class="filters">
            <span class="label">filter</span>
            <span class="chip">severity: any</span>
            <span class="chip">state: any</span>
            <span v-for="s in store.subjects" :key="s" class="subject-chip">
                {{ s }}
                <button class="x" @click="store.toggleSubject(s)">✕</button>
            </span>
            <span v-if="store.liveMode" class="live">⬤ live</span>
            <button v-else class="go-live" @click="store.goLive()">
                go live
            </button>
        </header>

        <div v-if="loading" class="state-msg">loading…</div>
        <div v-else-if="error" class="state-msg error">error: {{ error }}</div>
        <div v-else-if="!events.length" class="state-msg">
            no diagnostics in current window — try widening the time range.
        </div>

        <div v-else class="list">
            <DiagnosticItem
                v-for="ev in events"
                :key="ev.event_id"
                :event="ev"
                @select="onSelect"
            />
        </div>
    </section>
</template>

<style scoped>
.pane {
    display: flex;
    flex-direction: column;
    height: 100%;
}
.filters {
    display: flex;
    gap: 0.4rem;
    align-items: center;
    flex-wrap: wrap;
    padding: 0.6rem 0.8rem;
    border-bottom: 1px solid #30363d;
}
.label {
    color: #7d8590;
    font-size: 0.7rem;
}
.chip,
.subject-chip {
    background: #1f3a5f;
    color: #58a6ff;
    padding: 2px 8px;
    border-radius: 10px;
    font-size: 0.75rem;
}
.subject-chip {
    background: #1f2937;
    color: #c9d1d9;
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
}
.x {
    background: transparent;
    border: none;
    color: #7d8590;
    cursor: pointer;
    padding: 0;
    font-size: 0.8rem;
}
.live {
    margin-left: auto;
    color: #3fb950;
}
.go-live {
    margin-left: auto;
    background: #1f3a5f;
    color: #58a6ff;
    border: none;
    padding: 0.2rem 0.6rem;
    border-radius: 3px;
    cursor: pointer;
}
.list {
    padding: 0.6rem;
    overflow-y: auto;
    flex: 1;
}
.state-msg {
    padding: 1rem;
    color: #7d8590;
    text-align: center;
}
.state-msg.error {
    color: #ef4444;
}
</style>
