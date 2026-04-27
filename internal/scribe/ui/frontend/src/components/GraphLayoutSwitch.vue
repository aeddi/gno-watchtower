<script setup lang="ts">
import { NRadioGroup, NRadioButton } from 'naive-ui'
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
    <n-radio-group
        :value="store.graphLayout"
        size="small"
        @update:value="(v: GraphLayout) => store.setLayout(v)"
    >
        <n-radio-button
            v-for="opt in options"
            :key="opt.value"
            :value="opt.value"
            :label="opt.label"
        />
    </n-radio-group>
</template>
