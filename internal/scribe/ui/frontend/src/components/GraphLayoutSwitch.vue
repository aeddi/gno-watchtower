<script setup lang="ts">
import { NSegmented } from 'naive-ui'
import { useTimelineStore } from '@/stores/timeline'
import type { GraphLayout } from '@/types'

const store = useTimelineStore()
const options: { label: string; value: GraphLayout }[] = [
    { label: 'Circle', value: 'circle' },
    { label: 'Force', value: 'force' },
    { label: 'Table', value: 'table' },
]
</script>

<template>
    <n-segmented
        :value="store.graphLayout"
        :options="options"
        size="small"
        @update:value="(v: GraphLayout) => store.setLayout(v)"
    />
</template>
