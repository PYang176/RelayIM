<script setup lang="ts">
import type { FriendRequest } from '@/types/api'

const props = defineProps<{
  request: FriendRequest
  received: boolean
  busy: boolean
}>()

const emit = defineEmits<{
  accept: []
  reject: []
}>()

const statusLabels: Record<number, string> = {
  1: '等待处理',
  2: '已同意',
  3: '已拒绝',
  4: '已撤回',
}
</script>

<template>
  <article class="request-row">
    <span class="friend-avatar">
      {{ (request.user.nickname || request.user.username).slice(0, 1) }}
    </span>
    <div class="friend-copy">
      <strong>{{ request.user.nickname || request.user.username }}</strong>
      <small>@{{ request.user.username }}</small>
      <p v-if="request.remark">{{ request.remark }}</p>
      <span v-if="!received" class="request-status">
        {{ statusLabels[request.status] ?? '未知状态' }}
      </span>
    </div>
    <div v-if="received" class="friend-actions">
      <button :disabled="busy" type="button" @click="emit('accept')">同意</button>
      <button :disabled="busy" type="button" @click="emit('reject')">拒绝</button>
    </div>
  </article>
</template>
