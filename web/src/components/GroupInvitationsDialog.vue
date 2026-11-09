<script setup lang="ts">
import {nextTick, useTemplateRef, watch} from 'vue'

import {useGroupInvitationStore} from '@/stores/group-invitations'

const props = defineProps<{
    open: boolean
}>()

const emit = defineEmits<{
    close: []
    accepted: [conversationID: string]
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const invitations = useGroupInvitationStore()

watch(
    () => props.open,
    async (open) => {
        if (open) {
            dialog.value?.showModal()
            await invitations.loadReceived()
            await nextTick()
            closeButton.value?.focus()
        } else {
            dialog.value?.close()
        }
    },
    {immediate: true},
)

function close() {
    emit('close')
}

function formatExpiry(value: string) {
    return new Intl.DateTimeFormat('zh-CN', {
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
    }).format(new Date(value))
}

async function accept(invitationID: string) {
    const result = await invitations.acceptInvitation(invitationID)
    if (result) {
        emit('accepted', result.conversation_id)
    }
}
</script>

<template>
    <dialog
            ref="dialog"
            class="friends-dialog"
            aria-labelledby="group-invitations-title"
            @cancel.prevent="close"
            @close="close"
    >
        <div class="friends-drawer">
            <header class="friends-header">
                <div>
                    <h2 id="group-invitations-title">群邀请</h2>
                    <p>同意后才会加入群聊，邀请 7 天内有效。</p>
                </div>
                <button
                        ref="closeButton"
                        class="icon-button"
                        aria-label="关闭群邀请"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <p v-if="invitations.error" class="form-error" role="alert">
                {{ invitations.error }}
                <button
                        class="text-button"
                        type="button"
                        @click="invitations.clearError"
                >
                    关闭
                </button>
            </p>

            <section class="drawer-section drawer-list" aria-live="polite">
                <p v-if="invitations.loading" role="status">加载群邀请中…</p>
                <p
                        v-else-if="invitations.invitations.length === 0"
                        role="status"
                >
                    暂无待处理群邀请。
                </p>
                <article
                        v-for="invitation in invitations.invitations"
                        :key="invitation.invitation_id"
                        class="request-row"
                >
                    <span class="friend-avatar">
                        {{ invitation.conversation_name.slice(0, 1) }}
                    </span>
                    <div class="friend-copy">
                        <strong>{{ invitation.conversation_name }}</strong>
                        <small>
                            {{ invitation.inviter_name }} 邀请 ·
                            {{ formatExpiry(invitation.expires_at) }} 失效
                        </small>
                    </div>
                    <div class="friend-actions">
                        <button
                                :disabled="invitations.actionBusy"
                                type="button"
                                @click="accept(invitation.invitation_id)"
                        >
                            同意
                        </button>
                        <button
                                :disabled="invitations.actionBusy"
                                type="button"
                                @click="
                                    invitations.rejectInvitation(
                                        invitation.invitation_id,
                                    )
                                "
                        >
                            拒绝
                        </button>
                    </div>
                </article>
                <button
                        v-if="invitations.hasMore"
                        class="text-button"
                        :disabled="invitations.loadingMore"
                        type="button"
                        @click="invitations.loadMoreReceived"
                >
                    {{ invitations.loadingMore ? '加载中…' : '加载更多群邀请' }}
                </button>
            </section>
        </div>
    </dialog>
</template>
