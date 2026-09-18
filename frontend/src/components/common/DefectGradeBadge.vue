<script setup lang="ts">
import { computed } from 'vue';
const props = defineProps<{ grade: string }>();
const labels: Record<string, string> = { general: '一般缺陷', severe: '严重缺陷' };
const label = computed(() => labels[props.grade] || props.grade);
</script>
<template>
  <span :class="`severity severity--${grade === 'severe' ? 'critical' : 'medium'}`">{{ label }}</span>
</template>
