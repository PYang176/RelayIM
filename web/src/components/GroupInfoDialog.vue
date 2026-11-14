<script setup lang="ts">
import {computed, nextTick, useTemplateRef, watch} from 'vue'

import GroupProfileForm from '@/components/GroupProfileForm.vue'
import type {ConversationMember} from '@/types/api'

const props = defineProps<{
    open: boolean
    groupName: string
    groupAvatarUrl: string
    members: ConversationMember[]
    currentUserId: string
    loading: boolean
    busy: boolean
    avatarBusy: boolean
    error: string
    avatarError: string
}>()

const emit = defineEmits<{
    close: []
    viewProfile: [member: ConversationMember]
    openMembers: []
    openInvitations: []
    openManagement: []
    save: [payload: {name: string}]
    uploadAvatar: [file: File]
    removeAvatar: []
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const currentMember = computed(() =>
    props.members.find((member) => member.user_id === props.currentUserId),
)
const isOwner = computed(() => currentMember.value?.role === 3)
const canManage = computed(() => (currentMember.value?.role ?? 0) >= 2)
const previewMembers = computed(() => props.members.slice(0, 12))
const avatarLetter = computed(() => props.groupName.trim().slice(0, 1) || '群')

watch(
    () => props.open,
    async (open) => {
        if (open) {
            dialog.value?.showModal()
            await nextTick()
            closeButton.value?.focus()
        } else {
            dialog.value?.close()
        }
    },
    {immediate: true},
)

// memberName 计算群成员昵称或用户名的展示名称
function memberName(member: {nickname: string; username: string}) {
    return member.nickname || member.username
}

// roleLabel 把成员角色编码转换为中文标签
function roleLabel(role: number) {
    if (role === 3) return '群主'
    if (role === 2) return '管理员'
    return '成员'
}

// close 关闭群资料对话框
function close() {
    if (!props.busy && !props.avatarBusy) {
        emit('close')
    }
}
</script>

<template>
    <dialog
            ref="dialog"
            class="group-dialog group-info-dialog"
            aria-labelledby="group-info-title"
            @cancel.prevent="close"
            @close="close"
    >
        <section class="group-dialog-card">
            <header class="group-dialog-header">
                <div>
                    <h2 id="group-info-title">群资料</h2>
                    <p>查看群信息和成员，管理操作从这里进入独立页面。</p>
                </div>
                <button
                        ref="closeButton"
                        class="icon-button"
                        :disabled="busy || avatarBusy"
                        aria-label="关闭群资料"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <p v-if="error" class="form-error" role="alert">{{ error }}</p>

            <section class="group-overview" aria-labelledby="group-overview-name">
                <span class="group-overview-avatar">
                    <img
                            v-if="groupAvatarUrl"
                            :alt="`${groupName}的群头像`"
                            :src="groupAvatarUrl"
                    />
                    <template v-else>{{ avatarLetter }}</template>
                </span>
                <div class="group-overview-copy">
                    <h3 id="group-overview-name">{{ groupName }}</h3>
                    <p>
                        {{ members.length }} 位成员 ·
                        {{ roleLabel(currentMember?.role ?? 1) }}
                    </p>
                </div>
                <div class="group-overview-actions">
                    <button
                            class="ghost-button"
                            type="button"
                            @click="emit('openMembers')"
                    >
                        查看群成员
                    </button>
                    <button
                            v-if="canManage"
                            class="ghost-button"
                            type="button"
                            @click="emit('openInvitations')"
                    >
                        邀请成员
                    </button>
                </div>
            </section>

            <section
                    class="group-info-section"
                    aria-labelledby="group-member-preview-title"
            >
                <div class="group-info-section-heading">
                    <div>
                        <h3 id="group-member-preview-title">群聊成员</h3>
                        <p>先显示前 12 位成员，完整名单在“群成员”中查看。</p>
                    </div>
                    <button
                            class="text-button"
                            type="button"
                            @click="emit('openMembers')"
                    >
                        查看全部 {{ members.length }} 人
                    </button>
                </div>

                <p v-if="loading" role="status">加载群成员中…</p>
                <div v-else class="group-member-gallery">
                    <button
                            v-for="member in previewMembers"
                            :key="member.user_id"
                            class="group-gallery-member"
                            type="button"
                            @click="emit('viewProfile', member)"
                    >
                        <span class="group-gallery-avatar">
                            <img
                                    v-if="member.avatar_url"
                                    :alt="memberName(member)"
                                    :src="member.avatar_url"
                            />
                            <template v-else>
                                {{ memberName(member).slice(0, 1) }}
                            </template>
                        </span>
                        <strong>{{ memberName(member) }}</strong>
                        <small>{{ roleLabel(member.role) }}</small>
                    </button>
                    <button
                            v-if="canManage"
                            class="group-gallery-member group-gallery-action"
                            type="button"
                            @click="emit('openInvitations')"
                    >
                        <span class="group-gallery-avatar">+</span>
                        <strong>邀请成员</strong>
                        <small>发送群邀请</small>
                    </button>
                </div>
            </section>

            <section class="group-info-section">
                <div class="group-info-section-heading">
                    <div>
                        <h3>群组功能</h3>
                        <p>资料、成员、邀请和治理使用独立入口。</p>
                    </div>
                </div>
                <div class="group-info-shortcuts">
                    <button type="button" @click="emit('openMembers')">
                        <strong>群成员</strong>
                        <small>查看完整成员名单和公开资料</small>
                    </button>
                    <button
                            v-if="canManage"
                            type="button"
                            @click="emit('openInvitations')"
                    >
                        <strong>群邀请</strong>
                        <small>邀请好友并查看邀请处理记录</small>
                    </button>
                    <button type="button" @click="emit('openManagement')">
                        <strong>群管理</strong>
                        <small>处理成员权限、退群和群主治理</small>
                    </button>
                </div>
            </section>

            <GroupProfileForm
                    v-if="isOwner"
                    :avatar-url="groupAvatarUrl"
                    :avatar-busy="avatarBusy"
                    :avatar-error="avatarError"
                    :busy="busy"
                    :group-name="groupName"
                    @remove-avatar="emit('removeAvatar')"
                    @save="emit('save', $event)"
                    @upload-avatar="emit('uploadAvatar', $event)"
            />
            <p v-else class="group-info-readonly">
                群名称和群头像由群主维护，你可以在“群成员”和“群管理”中查看公开信息。
            </p>
        </section>
    </dialog>
</template>

<style scoped>
.group-info-dialog {
    width: min(44rem, calc(100vw - 2rem));
}

.group-overview {
    display: grid;
    grid-template-columns: 4.25rem minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.875rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
    padding: 1rem;
    background: var(--surface-muted);
}

.group-overview-avatar,
.group-gallery-avatar {
    display: grid;
    place-items: center;
    overflow: hidden;
    border-radius: 50%;
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-weight: 800;
}

.group-overview-avatar {
    width: 4.25rem;
    height: 4.25rem;
    font-size: 1.35rem;
}

.group-overview-avatar img,
.group-gallery-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.group-overview-copy h3,
.group-overview-copy p {
    margin: 0;
}

.group-overview-copy h3 {
    font-family: var(--font-display);
    font-size: 1.45rem;
}

.group-overview-copy p {
    margin-top: 0.25rem;
    color: var(--muted);
    font-size: 0.75rem;
}

.group-overview-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0.5rem;
}

.group-info-section {
    display: grid;
    gap: 0.875rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
    padding: 1rem;
}

.group-info-section-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
}

.group-info-section-heading h3,
.group-info-section-heading p {
    margin: 0;
}

.group-info-section-heading h3 {
    font-size: 1rem;
}

.group-info-section-heading p {
    margin-top: 0.25rem;
    color: var(--muted);
    font-size: 0.6875rem;
}

.group-member-gallery {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(5.25rem, 1fr));
    gap: 0.75rem;
}

.group-gallery-member {
    min-width: 0;
    display: grid;
    justify-items: center;
    gap: 0.25rem;
    border: 0;
    border-radius: var(--radius-sm);
    padding: 0.5rem 0.25rem;
    color: var(--ink);
    background: transparent;
    font: inherit;
    cursor: pointer;
}

.group-gallery-member:hover {
    background: var(--surface-muted);
}

.group-gallery-avatar {
    width: 3rem;
    height: 3rem;
}

.group-gallery-member strong,
.group-gallery-member small {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.group-gallery-member strong {
    font-size: 0.75rem;
}

.group-gallery-member small {
    color: var(--muted);
    font-size: 0.625rem;
}

.group-gallery-action .group-gallery-avatar {
    border: 1px dashed var(--line-strong);
    color: var(--signal-strong);
    background: white;
    font-size: 1.5rem;
}

.group-info-shortcuts {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.625rem;
}

.group-info-shortcuts button {
    display: grid;
    gap: 0.25rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 0.75rem;
    color: var(--ink);
    background: var(--surface-muted);
    text-align: left;
    cursor: pointer;
}

.group-info-shortcuts button:hover {
    border-color: var(--signal);
    background: var(--signal-soft);
}

.group-info-shortcuts strong {
    font-size: 0.75rem;
}

.group-info-shortcuts small,
.group-info-readonly {
    color: var(--muted);
    font-size: 0.6875rem;
    line-height: 1.55;
}

.group-info-readonly {
    margin: 0;
    border: 1px dashed var(--line);
    border-radius: var(--radius-sm);
    padding: 0.75rem;
}

@media (max-width: 36rem) {
    .group-overview {
        grid-template-columns: 4rem minmax(0, 1fr);
    }

    .group-overview-avatar {
        width: 4rem;
        height: 4rem;
    }

    .group-overview-actions {
        grid-column: 1 / -1;
        justify-content: flex-start;
    }

    .group-info-section-heading {
        flex-direction: column;
    }

    .group-info-shortcuts {
        grid-template-columns: 1fr;
    }
}
</style>
