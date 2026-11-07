<script setup lang="ts">
import { computed, shallowRef } from 'vue'

const props = defineProps<{
  disabled: boolean
}>()

const emit = defineEmits<{
  send: [content: string]
}>()

const content = shallowRef('')
const sending = shallowRef(false)
const remaining = computed(() => 8000 - [...content.value].length)

async function submit() {
  const value = content.value.trim()
  if (!value || props.disabled || sending.value) return
  sending.value = true
  try {
    emit('send', value)
    content.value = ''
  } finally {
    sending.value = false
  }
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    void submit()
  }
}
</script>

<template>
  <div class="composer">
    <label class="sr-only" for="message-input">消息内容</label>
    <textarea
      id="message-input"
      v-model="content"
      aria-describedby="message-input-help"
      :disabled="disabled"
      maxlength="8000"
      placeholder="输入消息，Enter 发送，Shift + Enter 换行"
      rows="2"
      @keydown="handleKeydown"
    />
    <div class="composer-footer">
      <span id="message-input-help">{{ remaining }} 字符</span>
      <button
        class="primary-button compact"
        :disabled="disabled || sending || !content.trim()"
        type="button"
        @click="submit"
      >
        发送
      </button>
    </div>
  </div>
</template>
