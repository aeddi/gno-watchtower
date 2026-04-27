import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { Filters, GraphLayout, Step, ThemeMode } from '@/types'
import { parseDuration } from '@/utils/duration'

export const useTimelineStore = defineStore('timeline', () => {
    const at = ref<Date | null>(null)
    const subjects = ref<string[]>([])
    const window = ref<number>(parseDuration('12h'))
    const step = ref<Step>('30s')
    const graphLayout = ref<GraphLayout>('circle')
    const filters = ref<Filters>({ severity: [], state: [], kinds: [] })
    const theme = ref<ThemeMode>('system')

    const liveMode = computed(() => at.value === null)

    function setAt(date: Date | null) {
        at.value = date
    }
    function setSubjects(ids: string[]) {
        subjects.value = [...ids]
    }
    function toggleSubject(id: string) {
        const i = subjects.value.indexOf(id)
        if (i === -1) subjects.value = [...subjects.value, id]
        else subjects.value = subjects.value.filter((s) => s !== id)
    }
    function setStep(s: Step) {
        step.value = s
    }
    function setLayout(l: GraphLayout) {
        graphLayout.value = l
    }
    function setWindow(ms: number) {
        window.value = ms
    }
    function setTheme(t: ThemeMode) {
        theme.value = t
    }
    function goLive() {
        at.value = null
    }

    return {
        at,
        subjects,
        window,
        step,
        graphLayout,
        filters,
        theme,
        liveMode,
        setAt,
        setSubjects,
        toggleSubject,
        setStep,
        setLayout,
        setWindow,
        setTheme,
        goLive,
    }
})
