<script setup lang="ts">
import {
    computed,
    nextTick,
    onBeforeUnmount,
    onMounted,
    shallowRef,
    useTemplateRef,
    watch,
} from 'vue'

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
const emojiOpen = shallowRef(false)
const fileInput = useTemplateRef<HTMLInputElement>('fileInput')
const messageInput = useTemplateRef<HTMLTextAreaElement>('messageInput')
const emojiPicker = useTemplateRef<HTMLElement>('emojiPicker')
const remaining = computed(() => 8000 - [...content.value].length)
const maxAttachmentBytes = 25 * 1024 * 1024
const commonEmojis = [
    '😀',
    '😂',
    '😊',
    '😍',
    '😘',
    '😎',
    '🤔',
    '😭',
    '😡',
    '🥳',
    '😴',
    '🤝',
    '👍',
    '👎',
    '👏',
    '🙏',
    '💪',
    '👌',
    '🎉',
    '🎂',
    '❤️',
    '💔',
    '🔥',
    '✨',
    '💯',
    '✅',
    '❌',
    '👀',
    '☕',
    '🌙',
    '☀️',
    '🚀',
]

// submit 校验输入并发送文本消息
async function submit() {
    const value = content.value.trim()
    if (!value || props.disabled || sending.value) return
    sending.value = true
    try {
        emit('send', value)
        content.value = ''
        emojiOpen.value = false
    } finally {
        sending.value = false
    }
}

// handleKeydown 处理快捷键发送和输入法组合键
function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && emojiOpen.value) {
        emojiOpen.value = false
        return
    }
    if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault()
        void submit()
    }
}

// toggleEmoji 打开或收起表情选择面板
function toggleEmoji() {
    if (props.disabled) return
    emojiOpen.value = !emojiOpen.value
}

// insertEmoji 在输入框光标位置插入表情并恢复焦点
function insertEmoji(emoji: string) {
    if (props.disabled) return
    const input = messageInput.value
    const start = input?.selectionStart ?? content.value.length
    const end = input?.selectionEnd ?? start
    content.value =
        content.value.slice(0, start) + emoji + content.value.slice(end)
    void nextTick(() => {
        const nextInput = messageInput.value
        if (!nextInput) return
        nextInput.focus()
        const cursor = start + emoji.length
        nextInput.setSelectionRange(cursor, cursor)
    })
}

// closeEmojiOnOutside 点击表情面板外部时收起面板
function closeEmojiOnOutside(event: PointerEvent) {
    const target = event.target
    if (
        target instanceof Node &&
        !emojiPicker.value?.contains(target)
    ) {
        emojiOpen.value = false
    }
}

// chooseFile 打开本地文件选择器
function chooseFile() {
    attachmentError.value = ''
    fileInput.value?.click()
}

// handleFile 校验附件大小并触发文件发送
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

watch(
    () => props.disabled,
    (disabled) => {
        if (disabled) {
            emojiOpen.value = false
        }
    },
)

onMounted(() => {
    document.addEventListener('pointerdown', closeEmojiOnOutside)
})

onBeforeUnmount(() => {
    document.removeEventListener('pointerdown', closeEmojiOnOutside)
})
</script>

<template>
    <div class="composer">
        <label class="sr-only" for="message-input">消息内容</label>
        <textarea
                ref="messageInput"
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
                <div ref="emojiPicker" class="emoji-picker">
                    <button
                            :aria-controls="'emoji-panel'"
                            :aria-expanded="emojiOpen"
                            :disabled="disabled"
                            aria-label="选择表情"
                            class="ghost-button emoji-trigger"
                            type="button"
                            @click="toggleEmoji"
                    >
                        <span aria-hidden="true">☺</span>
                        表情
                    </button>
                    <div
                            v-if="emojiOpen"
                            id="emoji-panel"
                            aria-label="选择表情"
                            class="emoji-panel"
                            role="group"
                    >
                        <header class="emoji-panel-header">
                            <strong>常用表情</strong>
                            <button
                                    aria-label="关闭表情面板"
                                    type="button"
                                    @click="emojiOpen = false"
                            >
                                ×
                            </button>
                        </header>
                        <div class="emoji-grid">
                            <button
                                    v-for="emoji in commonEmojis"
                                    :key="emoji"
                                    :aria-label="`插入表情 ${emoji}`"
                                    type="button"
                                    @click="insertEmoji(emoji)"
                            >
                                {{ emoji }}
                            </button>
                        </div>
                    </div>
                </div>
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
