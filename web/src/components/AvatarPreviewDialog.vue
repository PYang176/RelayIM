<script setup lang="ts">
import {computed, nextTick, shallowRef, useTemplateRef, watch} from 'vue'

const props = defineProps<{
    open: boolean
    name: string
    avatarUrl: string
}>()

const emit = defineEmits<{
    close: []
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const previewUrl = shallowRef('')
const previewName = shallowRef('')
const avatarLetter = computed(() => previewName.value.slice(0, 1))

watch(
    () => props.open,
    async (open) => {
        if (!open) {
            dialog.value?.close()
            return
        }
        previewUrl.value = props.avatarUrl
        previewName.value = props.name || '头像'
        if (dialog.value && !dialog.value.open) {
            dialog.value.showModal()
        }
        await nextTick()
        closeButton.value?.focus()
    },
    {immediate: true},
)

// close 关闭头像预览
function close() {
    emit('close')
}
</script>

<template>
    <dialog
            ref="dialog"
            aria-label="头像预览"
            class="avatar-preview-dialog"
            @cancel.prevent="close"
            @close="close"
    >
        <div class="avatar-preview-card">
            <button
                    ref="closeButton"
                    aria-label="关闭头像预览"
                    class="icon-button avatar-preview-close"
                    type="button"
                    @click="close"
            >
                ×
            </button>
            <img
                    v-if="previewUrl"
                    :alt="`${previewName}的头像`"
                    :src="previewUrl"
            />
            <span v-else class="avatar-preview-letter">{{ avatarLetter }}</span>
            <strong>{{ previewName }}</strong>
        </div>
    </dialog>
</template>

<style scoped>
.avatar-preview-dialog {
    width: min(32rem, calc(100vw - 2rem));
    max-width: none;
    border: 0;
    padding: 0;
    background: transparent;
}

.avatar-preview-dialog::backdrop {
    background: rgba(11, 28, 32, 0.78);
}

.avatar-preview-card {
    position: relative;
    display: grid;
    place-items: center;
    gap: 0.75rem;
    border-radius: var(--radius-md);
    padding: 1rem;
    color: white;
    background: #102d33;
}

.avatar-preview-card img,
.avatar-preview-letter {
    width: min(26rem, calc(100vw - 5rem));
    height: min(26rem, calc(100vw - 5rem));
    border-radius: var(--radius-md);
    object-fit: cover;
}

.avatar-preview-letter {
    display: grid;
    place-items: center;
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-family: var(--font-display);
    font-size: 7rem;
    font-weight: 600;
}

.avatar-preview-card strong {
    font-size: 0.875rem;
}

.avatar-preview-close {
    position: absolute;
    z-index: 1;
    top: 0.75rem;
    right: 0.75rem;
    border-color: rgba(255, 255, 255, 0.35);
    color: white;
    background: rgba(16, 45, 51, 0.75);
}
</style>
