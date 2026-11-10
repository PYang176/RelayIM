<script setup lang="ts">
import {computed, nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import {getUserProfile} from '@/api/auth'
import {getRelationshipStatus} from '@/api/friends'
import AvatarPreviewDialog from '@/components/AvatarPreviewDialog.vue'
import FriendRequestForm from '@/components/FriendRequestForm.vue'
import {useAuthStore} from '@/stores/auth'
import {useFriendshipStore} from '@/stores/friendships'
import type {
    ConversationMember,
    FriendRelationshipStatus,
    UserSummary,
} from '@/types/api'

const props = defineProps<{
    open: boolean
    member: ConversationMember | null
}>()

const emit = defineEmits<{
    close: []
    message: [userID: string]
}>()

const auth = useAuthStore()
const friendships = useFriendshipStore()
const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const profile = shallowRef<UserSummary | null>(null)
const relationship = shallowRef<FriendRelationshipStatus | null>(null)
const loading = shallowRef(false)
const profileError = shallowRef('')
const avatarPreviewOpen = shallowRef(false)
const requestFormOpen = shallowRef(false)
let requestGeneration = 0

const isSelf = computed(
    () => Boolean(props.member) && props.member?.user_id === auth.user?.id,
)
const displayName = computed(
    () =>
        profile.value?.nickname ||
        props.member?.nickname ||
        props.member?.username ||
        '',
)
const avatarUrl = computed(
    () => profile.value?.avatar_url || props.member?.avatar_url || '',
)
const canMessage = computed(
    () => !isSelf.value && Boolean(relationship.value?.can_message),
)
const canRequest = computed(
    () => !isSelf.value && Boolean(relationship.value?.can_request),
)
const relationshipLabel = computed(() => {
    if (isSelf.value) return '这是你'
    if (canMessage.value) return '好友关系正常'
    if (relationship.value?.blocked_by_me) return '你已拉黑对方'
    if (relationship.value?.has_pending_request) {
        return relationship.value.requested_by_me
            ? '好友申请已发送'
            : '对方已申请添加好友'
    }
    if (relationship.value && !relationship.value.can_request) {
        return '当前无法添加好友'
    }
    return '尚未成为好友'
})
const actionLabel = computed(() => {
    if (isSelf.value) return '这是你'
    if (relationship.value?.blocked_by_me) return '已拉黑'
    if (relationship.value?.has_pending_request) {
        return relationship.value.requested_by_me
            ? '申请已发送'
            : '等待你处理'
    }
    if (relationship.value && !relationship.value.can_request) {
        return '暂不可添加'
    }
    return '申请添加好友'
})

watch(
    () => [props.open, props.member?.user_id ?? ''] as const,
    async ([open]) => {
        if (!open) {
            dialog.value?.close()
            return
        }
        requestFormOpen.value = false
        avatarPreviewOpen.value = false
        resetMemberProfile()
        if (dialog.value && !dialog.value.open) {
            dialog.value.showModal()
        }
        await nextTick()
        closeButton.value?.focus()
        await loadMemberData()
    },
    {immediate: true},
)

// resetMemberProfile 清空上一个成员的资料和关系状态
function resetMemberProfile() {
    profile.value = null
    relationship.value = null
    profileError.value = ''
    loading.value = true
}

// loadMemberData 并行读取成员公开资料与好友关系状态
async function loadMemberData() {
    const member = props.member
    if (!member) {
        loading.value = false
        return
    }
    const generation = ++requestGeneration
    friendships.clearError()
    try {
        if (isSelf.value) {
            profile.value = auth.user
            relationship.value = null
            return
        }
        const [userProfile, relationshipStatus] = await Promise.all([
            getUserProfile(auth.token, member.user_id),
            getRelationshipStatus(auth.token, member.user_id),
        ])
        if (generation === requestGeneration) {
            profile.value = userProfile
            relationship.value = relationshipStatus
        }
    } catch (cause) {
        if (generation === requestGeneration) {
            profileError.value =
                cause instanceof Error ? cause.message : '加载成员资料失败'
        }
    } finally {
        if (generation === requestGeneration) {
            loading.value = false
        }
    }
}

// openAvatar 打开当前成员头像的大图预览
function openAvatar() {
    if (displayName.value) {
        avatarPreviewOpen.value = true
    }
}

// close 关闭成员资料浮层
function close() {
    if (!friendships.actionBusy) {
        emit('close')
    }
}

// startChat 从成员资料发起私聊
function startChat() {
    if (props.member && canMessage.value) {
        emit('message', props.member.user_id)
    }
}

// openRequestForm 展开好友申请理由输入
function openRequestForm() {
    if (canRequest.value) {
        friendships.clearError()
        requestFormOpen.value = true
    }
}

// submitFriendRequest 提交好友申请并刷新关系动作
async function submitFriendRequest(remark: string) {
    if (!props.member || friendships.actionBusy) return
    if (await friendships.sendRequest(props.member.user_id, remark)) {
        requestFormOpen.value = false
        await loadMemberData()
    }
}

// roleLabel 把群成员角色编码转换为中文标签
function roleLabel(role: number) {
    if (role === 3) return '群主'
    if (role === 2) return '管理员'
    return '群成员'
}
</script>

<template>
    <dialog
            ref="dialog"
            aria-label="成员资料"
            class="member-profile-dialog"
            @cancel.prevent="close"
            @close="close"
    >
        <section class="member-profile-card">
            <header class="member-profile-header">
                <span class="member-profile-role">
                    {{ member ? roleLabel(member.role) : '群成员' }}
                </span>
                <button
                        ref="closeButton"
                        :disabled="friendships.actionBusy"
                        aria-label="关闭成员资料"
                        class="icon-button"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <div v-if="loading" class="member-profile-state" role="status">
                正在加载成员资料…
            </div>
            <p v-else-if="profileError" class="form-error" role="alert">
                {{ profileError }}
            </p>

            <template v-else-if="member">
                <div class="member-profile-hero">
                    <button
                            :aria-label="`放大${displayName}的头像`"
                            class="member-profile-avatar"
                            type="button"
                            @click="openAvatar"
                    >
                        <img
                                v-if="avatarUrl"
                                :alt="`${displayName}的头像`"
                                :src="avatarUrl"
                        />
                        <span v-else>{{ displayName.slice(0, 1) }}</span>
                    </button>
                    <div class="member-profile-copy">
                        <h2>{{ displayName }}</h2>
                        <p>@{{ profile?.username || member.username }}</p>
                        <span
                                :class="{
                                    muted:
                                        !canMessage &&
                                        !relationship?.has_pending_request,
                                }"
                                class="member-relationship-state"
                        >
                            {{ relationshipLabel }}
                        </span>
                    </div>
                </div>

                <section class="member-profile-bio" aria-label="个人简介">
                    <h3>个人简介</h3>
                    <p>
                        {{ profile?.bio || '这个用户还没有填写个人简介。' }}
                    </p>
                </section>

                <FriendRequestForm
                        v-if="requestFormOpen"
                        :busy="friendships.actionBusy"
                        :error="friendships.error"
                        :target-name="displayName"
                        @cancel="requestFormOpen = false"
                        @submit="submitFriendRequest"
                />

                <footer v-else class="member-profile-actions">
                    <p v-if="!canMessage">
                        {{
                            relationship?.has_pending_request
                                ? '好友申请正在处理中，请等待双方确认。'
                                : '成为好友后可以直接发起私聊。'
                        }}
                    </p>
                    <button
                            v-if="canMessage"
                            class="primary-button compact"
                            type="button"
                            @click="startChat"
                    >
                        发私聊
                    </button>
                    <button
                            v-else-if="canRequest"
                            class="primary-button compact"
                            type="button"
                            @click="openRequestForm"
                    >
                        {{ actionLabel }}
                    </button>
                    <button v-else class="ghost-button" disabled type="button">
                        {{ actionLabel }}
                    </button>
                </footer>
            </template>

            <AvatarPreviewDialog
                    :avatar-url="avatarUrl"
                    :name="displayName"
                    :open="avatarPreviewOpen"
                    @close="avatarPreviewOpen = false"
            />
        </section>
    </dialog>
</template>

<style scoped>
.member-profile-dialog {
    width: min(34rem, calc(100vw - 2rem));
    max-width: none;
    max-height: min(42rem, calc(100dvh - 2rem));
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-md);
    padding: 0;
    color: var(--ink);
    background: var(--surface);
}

.member-profile-dialog::backdrop {
    background: rgba(21, 52, 59, 0.48);
}

.member-profile-card {
    max-height: inherit;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
    padding: 1.25rem;
    overflow-y: auto;
}

.member-profile-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
}

.member-profile-role {
    border-radius: 999px;
    padding: 0.25rem 0.625rem;
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-size: 0.6875rem;
    font-weight: 700;
}

.member-profile-state {
    padding: 2rem 0;
    color: var(--muted);
    text-align: center;
}

.member-profile-hero {
    display: flex;
    align-items: center;
    gap: 1.125rem;
}

.member-profile-avatar {
    width: 6.5rem;
    height: 6.5rem;
    display: grid;
    flex: 0 0 auto;
    place-items: center;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-md);
    padding: 0;
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-family: var(--font-display);
    font-size: 2.5rem;
    font-weight: 600;
    overflow: hidden;
}

.member-profile-avatar:hover {
    border-color: var(--signal);
}

.member-profile-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.member-profile-copy h2,
.member-profile-copy p {
    margin: 0;
}

.member-profile-copy h2 {
    font-family: var(--font-display);
    font-size: 1.5rem;
}

.member-profile-copy > p {
    color: var(--muted);
    font-family: var(--font-data);
    font-size: 0.75rem;
}

.member-relationship-state {
    display: inline-block;
    margin-top: 0.5rem;
    border-radius: 999px;
    padding: 0.1875rem 0.5rem;
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-size: 0.6875rem;
    font-weight: 700;
}

.member-relationship-state.muted {
    color: var(--muted);
    background: var(--surface-muted);
}

.member-profile-bio {
    border-block: 1px solid var(--line);
    padding: 1rem 0;
}

.member-profile-bio h3,
.member-profile-bio p {
    margin: 0;
}

.member-profile-bio h3 {
    color: var(--muted);
    font-size: 0.75rem;
}

.member-profile-bio p {
    margin-top: 0.5rem;
    font-size: 0.8125rem;
    line-height: 1.7;
    white-space: pre-wrap;
}

.member-profile-actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    gap: 0.75rem;
}

.member-profile-actions p {
    flex: 1 1 14rem;
    margin: 0;
    color: var(--muted);
    font-size: 0.75rem;
    line-height: 1.6;
}

@media (max-width: 32rem) {
    .member-profile-dialog {
        width: 100vw;
        height: 100dvh;
        max-height: none;
        margin: 0;
        border: 0;
        border-radius: 0;
    }

    .member-profile-card {
        min-height: 100%;
        padding: 1rem;
    }

    .member-profile-avatar {
        width: 5.5rem;
        height: 5.5rem;
        font-size: 2rem;
    }

    .member-profile-actions button {
        width: 100%;
    }
}
</style>
