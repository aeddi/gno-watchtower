<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { useTimelineStore } from '@/stores/timeline'
import { api } from '@/api/client'

const store = useTimelineStore()
const states = ref<Record<string, any>>({})
const loading = ref(false)

async function reload() {
    loading.value = true
    try {
        if (store.subjects.length === 0) {
            const s = await api.getState('_chain', store.at ?? undefined)
            states.value = { _chain: s.structured }
        } else {
            const out: Record<string, any> = {}
            await Promise.all(
                store.subjects.map(async (sub) => {
                    try {
                        const s = await api.getState(sub, store.at ?? undefined)
                        out[sub] = s.structured
                    } catch {
                        out[sub] = {}
                    }
                })
            )
            states.value = out
        }
    } finally {
        loading.value = false
    }
}

onMounted(reload)
watch([() => store.subjects, () => store.at], reload, { deep: true })

interface Row {
    key: string
    values: Record<string, any>
}

function rows(): Row[] {
    if (store.subjects.length === 0) {
        const s = states.value._chain ?? {}
        return Object.entries(s).map(([k, v]) => ({
            key: k,
            values: { _chain: v },
        }))
    }
    const keys = new Set<string>()
    for (const s of Object.values(states.value)) {
        for (const k of Object.keys(s ?? {})) keys.add(k)
    }
    return Array.from(keys).map((k) => ({
        key: k,
        values: Object.fromEntries(
            store.subjects.map((sub) => [sub, states.value[sub]?.[k]])
        ),
    }))
}

function isCollapsed(values: Record<string, any>): boolean {
    const vs = Object.values(values)
    return (
        vs.length > 1 &&
        vs.every((v) => JSON.stringify(v) === JSON.stringify(vs[0]))
    )
}

function activeColumns(): string[] {
    return store.subjects.length === 0 ? ['_chain'] : store.subjects
}
</script>

<template>
    <aside class="panel">
        <header class="title">
            <span v-if="store.subjects.length === 0">chain state</span>
            <span v-else-if="store.subjects.length === 1">{{
                store.subjects[0]
            }}</span>
            <span v-else>compare ({{ store.subjects.length }})</span>
            <span v-if="store.at" class="at"
                >@ {{ store.at.toLocaleString() }}</span
            >
            <span v-else class="at live">live</span>
        </header>

        <div v-if="loading" class="msg">loading…</div>
        <table v-else class="kv">
            <tbody>
                <tr v-for="r in rows()" :key="r.key">
                    <td class="key">{{ r.key }}</td>
                    <template v-if="isCollapsed(r.values)">
                        <td
                            class="val collapsed"
                            :colspan="activeColumns().length"
                        >
                            {{ Object.values(r.values)[0] ?? '—' }}
                        </td>
                    </template>
                    <template v-else>
                        <td
                            v-for="sub in activeColumns()"
                            :key="sub"
                            class="val"
                        >
                            {{ r.values[sub] ?? '—' }}
                        </td>
                    </template>
                </tr>
            </tbody>
        </table>
    </aside>
</template>

<style scoped>
.panel {
    padding: 0.6rem 0.8rem;
    background: #0d1117;
    height: 100%;
    overflow-y: auto;
    box-sizing: border-box;
}
.title {
    display: flex;
    gap: 0.6rem;
    align-items: baseline;
    padding-bottom: 0.4rem;
    border-bottom: 1px solid #30363d;
}
.at {
    color: #7d8590;
    font-size: 0.7rem;
    margin-left: auto;
}
.at.live {
    color: #3fb950;
}
.kv {
    width: 100%;
    border-collapse: collapse;
    font-family: ui-monospace, monospace;
    font-size: 0.8rem;
    margin-top: 0.4rem;
}
.kv td {
    padding: 0.2rem 0.4rem;
    border-bottom: 1px solid rgba(255, 255, 255, 0.04);
}
.key {
    color: #7d8590;
}
.val {
    text-align: right;
}
.val.collapsed {
    color: #c9d1d9;
    font-style: italic;
}
.msg {
    padding: 1rem;
    text-align: center;
    color: #7d8590;
}
</style>
