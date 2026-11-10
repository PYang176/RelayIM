<script setup lang="ts">
import {computed, shallowRef, watch} from 'vue'

import {getUserProfile} from '@/api/auth'
import AvatarPreviewDialog from '@/components/AvatarPreviewDialog.vue'
import {useAuthStore} from '@/stores/auth'
import type {Conversation, Friend, UserSummary} from '@/types/api'

const props = defineProps<{
    friend: Friend | null
    groups: Conversation[]
}>()

const emit = defineEmits<{
    message: [userID: string]
    addToGroup: [friend: Friend]
}>()

const auth = useAuthStore()
const profile = shallowRef<UserSummary | null>(null)
const loading = shallowRef(false)
const profileError = shallowRef('')
const avatarPreviewOpen = shallowRef(false)
let requestGeneration = 0

const displayName = computed(
    () =>
        props.friend?.remark ||
        profile.value?.nickname ||
        props.friend?.user.nickname ||
        props.friend?.user.username ||
        '',
)
const avatarUrl = computed(
    () => profile.value?.avatar_url || props.friend?.user.avatar_url || '',
)
const avatarLetter = computed(() => displayName.value.slice(0, 1))
const relationshipActive = computed(
    () => Boolean(props.friend?.relationship_active),
)
const canStartChat = computed(
    () =>
        Boolean(props.friend) &&
        relationshipActive.value &&
        !props.friend?.blocked_by_me,
)
const canAddToGroup = computed(
    () => canStartChat.value && props.groups.length > 0,
)

watch(
    () => props.friend?.user.id ?? '',
    async (userID) => {
        const generation = ++requestGeneration
        profile.value = null
        profileError.value = ''
        avatarPreviewOpen.value = false
        if (!userID) {
            loading.value = false
            return
        }
        loading.value = true
        try {
            const result = await getUserProfile(auth.token, userID)
            if (generation === requestGeneration) {
                profile.value = result
            }
        } catch (cause) {
            if (generation === requestGeneration) {
                profileError.value =
                    cause instanceof Error ? cause.message : '加载联系人资料失败'
            }
        } finally {
            if (generation === requestGeneration) {
                loading.value = false
            }
        }
    },
    {immediate: true},
)

// openAvatar 打开当前联系人头像的大图预览
function openAvatar() {
    if (props.friend) {
        avatarPreviewOpen.value = true
    }
}

// startChat 从联系人资料进入私聊
function startChat() {
    if (props.friend && canStartChat.value) {
        emit('message', props.friend.user.id)
    }
}

// addToGroup 打开群聊邀请选择
function addToGroup() {
    if (props.friend && canAddToGroup.value) {
        emit('addToGroup', props.friend)
    }
}
</script>

<template>
    <section class="contact-detail-panel">
        <div v-if="!friend" class="contact-placeholder">
            <span class="contact-placeholder-mark">联</span>
            <h1>选择一位联系人</h1>
            <p>左侧选择好友后，这里会显示头像、备注和个人资料。</p>
        </div>

        <article v-else class="contact-profile">
            <header class="contact-profile-header">
                <button
                        class="contact-profile-avatar"
                        :aria-label="`放大${displayName}的头像`"
                        type="button"
                        @click="openAvatar"
                >
                    <img
                            v-if="avatarUrl"
                            :alt="`${displayName}的头像`"
                            :src="avatarUrl"
                    />
                    <template v-else>{{ avatarLetter }}</template>
                </button>
                <div class="contact-profile-heading">
                    <h1>{{ displayName }}</h1>
                    <p>@{{ profile?.username || friend.user.username }}</p>
                    <span
                            :class="{inactive: !relationshipActive}"
                            class="contact-relationship-state"
                    >
                        {{ relationshipActive ? '好友关系正常' : '对方已删除你' }}
                    </span>
                </div>
            </header>

            <p v-if="loading" class="contact-profile-state" role="status">
                正在加载资料…
            </p>
            <p v-else-if="profileError" class="form-error" role="alert">
                {{ profileError }}
            </p>

            <dl class="contact-profile-fields">
                <div>
                    <dt>好友备注</dt>
                    <dd>{{ friend.remark || '未设置备注' }}</dd>
                </div>
                <div>
                    <dt>个人简介</dt>
                    <dd>{{ profile?.bio || '这个用户还没有填写个人简介。' }}</dd>
                </div>
            </dl>

            <footer class="contact-profile-actions">
                <p v-if="!relationshipActive">
                    双方当前不是好友，无法继续私聊或邀请加入群聊。
                </p>
                <button
                        :disabled="!canAddToGroup"
                        class="ghost-button contact-action-button"
                        type="button"
                        @click="addToGroup"
                >
                    添加到群聊
                </button>
                <button
                        :disabled="!canStartChat"
                        class="primary-button compact contact-action-button"
                        type="button"
                        @click="startChat"
                >
                    发私聊
                </button>
            </footer>
        </article>

        <AvatarPreviewDialog
                :avatar-url="avatarUrl"
                :name="displayName"
                :open="avatarPreviewOpen"
                @close="avatarPreviewOpen = false"
        />
    </section>
</template>

<style scoped>
.contact-detail-panel {
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    background-color: #f1f5f4;
    background-image:
        linear-gradient(rgba(21, 52, 59, 0.035) 1px, transparent 1px),
        linear-gradient(90deg, rgba(21, 52, 59, 0.035) 1px, transparent 1px);
    background-size: 2rem 2rem;
}

.contact-placeholder {
    flex: 1;
    display: grid;
    place-content: center;
    justify-items: center;
    gap: 0.5rem;
    padding: 2rem;
    text-align: center;
}

.contact-placeholder-mark {
    width: 5rem;
    height: 5rem;
    display: grid;
    place-items: center;
    border: 1px solid var(--line);
    border-radius: var(--radius-md);
    color: var(--signal-strong);
    background: color-mix(in srgb, var(--signal-soft) 75%, white);
    font-family: var(--font-display);
    font-size: 2rem;
}

.contact-placeholder h1,
.contact-placeholder p {
    margin: 0;
}

.contact-placeholder h1 {
    font-size: 1.125rem;
}

.contact-placeholder p {
    color: var(--muted);
    font-size: 0.75rem;
}

.contact-profile {
    width: min(46rem, 100%);
    min-height: 100%;
    display: flex;
    flex-direction: column;
    margin: 0 auto;
    padding: clamp(1.5rem, 4vw, 3rem);
}

.contact-profile-header {
    display: flex;
    align-items: center;
    gap: 1.25rem;
}

.contact-profile-avatar {
    width: 7.5rem;
    height: 7.5rem;
    display: grid;
    place-items: center;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-md);
    padding: 0;
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-family: var(--font-display);
    font-size: 3rem;
    font-weight: 600;
    overflow: hidden;
}

.contact-profile-avatar:hover {
    border-color: var(--signal);
}

.contact-profile-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.contact-profile-heading h1,
.contact-profile-heading p {
    margin: 0;
}

.contact-profile-heading h1 {
    font-size: 1.5rem;
}

.contact-profile-heading p {
    color: var(--muted);
    font-family: var(--font-data);
    font-size: 0.75rem;
}

.contact-relationship-state {
    display: inline-block;
    margin-top: 0.5rem;
    border-radius: 999px;
    padding: 0.1875rem 0.5rem;
    color: var(--signal-strong);
    background: var(--signal-soft);
    font-size: 0.6875rem;
    font-weight: 700;
}

.contact-relationship-state.inactive {
    color: var(--danger);
    background: #f8e8e8;
}

.contact-profile-state {
    margin: 1.5rem 0 0;
    color: var(--muted);
    font-size: 0.8125rem;
}

.contact-profile-fields {
    display: grid;
    gap: 0.75rem;
    margin: 2rem 0 0;
}

.contact-profile-fields > div {
    display: grid;
    grid-template-columns: 6rem minmax(0, 1fr);
    gap: 1rem;
    border-bottom: 1px solid var(--line);
    padding: 0 0 0.875rem;
}

.contact-profile-fields dt {
    color: var(--muted);
    font-size: 0.75rem;
    font-weight: 700;
}

.contact-profile-fields dd {
    margin: 0;
    font-size: 0.8125rem;
    line-height: 1.7;
    white-space: pre-wrap;
}

.contact-profile-actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 0.625rem;
    margin-top: auto;
    border-top: 1px solid var(--line);
    padding-top: 1rem;
}

.contact-profile-actions p {
    flex: 1;
    margin: 0;
    color: var(--danger);
    font-size: 0.75rem;
}

.contact-action-button {
    min-width: 7rem;
}

@media (max-width: 48rem) {
    .contact-profile {
        padding: 1.25rem 1rem 5rem;
    }

    .contact-profile-avatar {
        width: 5.5rem;
        height: 5.5rem;
        font-size: 2rem;
    }

    .contact-profile-actions {
        flex-wrap: wrap;
    }
}
</style>
