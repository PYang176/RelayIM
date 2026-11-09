<script setup lang="ts">
import {computed, shallowRef, useTemplateRef} from 'vue'

const props = defineProps<{
    disabled: boolean
    uploading: boolean
    uploadProgress: number
}>()

const emit = defineEmits<{
    send: [content: string]
    sendFile: [file: File]
}>()

const content = shallowRef('')
const sending = shallowRef(false)
const attachmentError = shallowRef('')
const fileInput = useTemplateRef<HTMLInputElement>('fileInput')
const remaining = computed(() => 8000 - [...content.value].length)
const maxAttachmentBytes = 25 * 1024 * 1024

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

function chooseFile() {
    attachmentError.value = ''
    fileInput.value?.click()
}

function handleFile(event: Event) {
    const input = event.target as HTMLInputElement
    const file = input.files?.[0]
    input.value = ''
    if (!file) return
    if (file.size <= 0) {
        attachmentError.value = '不能上传空文件'
        return
    }
    if (file.size > maxAttachmentBytes) {
        attachmentError.value = '附件不能超过 25 MB'
        return
    }
    attachmentError.value = ''
    emit('sendFile', file)
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
        <p v-if="attachmentError" class="composer-error" role="alert">
            {{ attachmentError }}
        </p>
        <div class="composer-footer">
            <span id="message-input-help">{{ remaining }} 字符</span>
            <div class="composer-actions">
                <input
                        ref="fileInput"
                        class="sr-only"
                        type="file"
                        @change="handleFile"
                />
                <button
                        class="ghost-button"
                        :disabled="disabled || uploading"
                        title="图片或文件，最大 25 MB"
                        type="button"
                        @click="chooseFile"
                >
                    {{ uploading ? `${uploadProgress}%` : '图片/文件' }}
                </button>
                <button
                        class="primary-button compact"
                        :disabled="
                            disabled ||
                            sending ||
                            uploading ||
                            !content.trim()
                        "
                        type="button"
                        @click="submit"
                >
                    发送
                </button>
            </div>
        </div>
    </div>
</template>
