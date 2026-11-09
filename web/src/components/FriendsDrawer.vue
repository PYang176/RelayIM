<script setup lang="ts">
import {nextTick, shallowRef, useTemplateRef, watch} from 'vue'

import FriendRequestRow from '@/components/FriendRequestRow.vue'
import FriendRow from '@/components/FriendRow.vue'
import {useFriendshipStore} from '@/stores/friendships'

const props = defineProps<{
    open: boolean
}>()

const emit = defineEmits<{
    close: []
    openDirect: [userID: string]
}>()

const dialog = useTemplateRef<HTMLDialogElement>('dialog')
const closeButton = useTemplateRef<HTMLButtonElement>('closeButton')
const friendships = useFriendshipStore()
const activeTab = shallowRef<'friends' | 'requests' | 'blocked'>('friends')
const searchQuery = shallowRef('')
const requestMode = shallowRef<'received' | 'sent'>('received')
const requestingUserID = shallowRef('')
const requestRemark = shallowRef('')
const requestReasonError = shallowRef('')

watch(
    () => props.open,
    async (open) => {
        if (open) {
            dialog.value?.showModal()
            await friendships.loadAll()
            await nextTick()
            closeButton.value?.focus()
        } else {
            dialog.value?.close()
        }
    },
    {immediate: true},
)

async function submitSearch() {
    await friendships.search(searchQuery.value)
}

function close() {
    emit('close')
}

function beginRequest(userID: string) {
    requestingUserID.value = userID
    requestRemark.value = ''
    requestReasonError.value = ''
}

function cancelRequest() {
    requestingUserID.value = ''
    requestRemark.value = ''
    requestReasonError.value = ''
}

async function submitRequest(userID: string) {
    const remark = requestRemark.value.trim()
    if (!remark) {
        requestReasonError.value = '请填写好友申请理由'
        return
    }
    if (await friendships.sendRequest(userID, remark)) {
        cancelRequest()
    }
}
</script>

<template>
    <dialog
            ref="dialog"
            class="friends-dialog"
            aria-labelledby="friends-title"
            @cancel.prevent="close"
            @close="close"
    >
        <div class="friends-drawer">
            <header class="friends-header">
                <div>
                    <h2 id="friends-title">联系人</h2>
                    <p>搜索用户、处理申请和设置独立好友备注。</p>
                </div>
                <button
                        ref="closeButton"
                        class="icon-button"
                        aria-label="关闭联系人"
                        type="button"
                        @click="close"
                >
                    ×
                </button>
            </header>

            <div class="friends-tabs" role="group" aria-label="联系人页面">
                <button
                        :aria-pressed="activeTab === 'friends'"
                        :class="{ active: activeTab === 'friends' }"
                        type="button"
                        @click="activeTab = 'friends'"
                >
                    好友 {{ friendships.friends.length
                    }}{{ friendships.friendHasMore ? '+' : '' }}
                </button>
                <button
                        :aria-pressed="activeTab === 'requests'"
                        :class="{ active: activeTab === 'requests' }"
                        type="button"
                        @click="activeTab = 'requests'"
                >
                    申请 {{ friendships.receivedRequests.length
                    }}{{ friendships.receivedHasMore ? '+' : '' }}
                </button>
                <button
                        :aria-pressed="activeTab === 'blocked'"
                        :class="{ active: activeTab === 'blocked' }"
                        type="button"
                        @click="activeTab = 'blocked'"
                >
                    拉黑 {{ friendships.blockedUsers.length }}
                </button>
            </div>

            <p v-if="friendships.error" class="form-error" role="alert">
                {{ friendships.error }}
                <button class="text-button" type="button" @click="friendships.clearError">
                    关闭
                </button>
            </p>

            <template v-if="activeTab === 'friends'">
                <form class="friend-search" role="search" @submit.prevent="submitSearch">
                    <label class="sr-only" for="friend-search-input">搜索用户</label>
                    <input
                            id="friend-search-input"
                            v-model="searchQuery"
                            maxlength="50"
                            placeholder="输入用户名或昵称"
                            required
                    />
                    <button :disabled="friendships.searching" type="submit">
                        {{ friendships.searching ? '搜索中' : '搜索' }}
                    </button>
                </form>

                <section v-if="friendships.searchResults.length" class="drawer-section">
                    <h3>搜索结果</h3>
                    <article
                            v-for="user in friendships.searchResults"
                            :key="user.id"
                            class="request-row"
                    >
            <span class="friend-avatar">
              {{ (user.nickname || user.username).slice(0, 1) }}
            </span>
                        <div class="friend-copy">
                            <strong>{{ user.nickname || user.username }}</strong>
                            <small>@{{ user.username }}</small>
                        </div>
                        <div
                                v-if="requestingUserID === user.id"
                                class="friend-inline-form"
                        >
                            <label class="sr-only" :for="`request-remark-${user.id}`">
                                好友申请理由
                            </label>
                            <input
                                    :id="`request-remark-${user.id}`"
                                    v-model="requestRemark"
                                    maxlength="200"
                                    placeholder="填写申请理由"
                                    @keyup.enter="submitRequest(user.id)"
                            />
                            <button
                                    :disabled="friendships.actionBusy"
                                    type="button"
                                    @click="submitRequest(user.id)"
                            >
                                发送申请
                            </button>
                            <button type="button" @click="cancelRequest">取消</button>
                            <span
                                    v-if="requestReasonError"
                                    class="form-error"
                                    role="alert"
                            >
                                {{ requestReasonError }}
                            </span>
                        </div>
                        <div v-else class="friend-actions">
                            <button
                                    :disabled="friendships.actionBusy"
                                    type="button"
                                    @click="beginRequest(user.id)"
                            >
                                添加
                            </button>
                            <button
                                    :disabled="friendships.actionBusy"
                                    type="button"
                                    @click="friendships.blockUser(user.id)"
                            >
                                拉黑
                            </button>
                        </div>
                    </article>
                </section>

                <section class="drawer-section drawer-list" aria-live="polite">
                    <h3>好友</h3>
                    <p v-if="friendships.loading" role="status">加载好友中…</p>
                    <p v-else-if="friendships.friends.length === 0" role="status">
                        还没有好友。搜索用户并发送申请。
                    </p>
                    <FriendRow
                            v-for="friend in friendships.friends"
                            :key="friend.friendship_id"
                            :busy="friendships.actionBusy"
                            :friend="friend"
                            @message="emit('openDirect', $event)"
                            @block="friendships.blockUser(friend.user.id)"
                            @remove="friendships.removeFriend(friend.user.id)"
                            @unblock="friendships.unblockUser(friend.user.id)"
                            @update-remark="friendships.updateRemark(friend.user.id, $event)"
                    />
                    <button
                            v-if="friendships.friendHasMore"
                            class="text-button"
                            :disabled="friendships.loadingMore"
                            type="button"
                            @click="friendships.loadMoreFriends"
                    >
                        {{ friendships.loadingMore ? '加载中…' : '加载更多好友' }}
                    </button>
                </section>
            </template>

            <template v-else-if="activeTab === 'requests'">
                <div class="friends-tabs compact" role="group" aria-label="申请方向">
                    <button
                            :aria-pressed="requestMode === 'received'"
                            :class="{ active: requestMode === 'received' }"
                            type="button"
                            @click="requestMode = 'received'"
                    >
                        收到
                    </button>
                    <button
                            :aria-pressed="requestMode === 'sent'"
                            :class="{ active: requestMode === 'sent' }"
                            type="button"
                            @click="requestMode = 'sent'"
                    >
                        发出
                    </button>
                </div>

                <section class="drawer-section drawer-list" aria-live="polite">
                    <p
                            v-if="
              requestMode === 'received'
                ? friendships.receivedRequests.length === 0
                : friendships.sentRequests.length === 0
            "
                            role="status"
                    >
                        暂无{{ requestMode === 'received' ? '收到' : '发出' }}的好友申请。
                    </p>
                    <FriendRequestRow
                            v-for="request in requestMode === 'received'
              ? friendships.receivedRequests
              : friendships.sentRequests"
                            :key="request.id"
                            :busy="friendships.actionBusy"
                            :received="requestMode === 'received'"
                            :request="request"
                            @accept="friendships.acceptRequest(request.id)"
                            @reject="friendships.rejectRequest(request.id)"
                    />
                    <button
                            v-if="
                                requestMode === 'received'
                                    ? friendships.receivedHasMore
                                    : friendships.sentHasMore
                            "
                            class="text-button"
                            :disabled="friendships.loadingMore"
                            type="button"
                            @click="
                                requestMode === 'received'
                                    ? friendships.loadMoreReceivedRequests()
                                    : friendships.loadMoreSentRequests()
                            "
                    >
                        {{ friendships.loadingMore ? '加载中…' : '加载更多申请' }}
                    </button>
                </section>
            </template>

            <section v-else class="drawer-section drawer-list" aria-live="polite">
                <h3>已拉黑用户</h3>
                <p v-if="friendships.blockedUsers.length === 0" role="status">
                    当前没有拉黑用户。
                </p>
                <article
                        v-for="item in friendships.blockedUsers"
                        :key="item.user.id"
                        class="request-row"
                >
                    <span class="friend-avatar">
                        {{ (item.user.nickname || item.user.username).slice(0, 1) }}
                    </span>
                    <div class="friend-copy">
                        <strong>{{ item.user.nickname || item.user.username }}</strong>
                        <small>@{{ item.user.username }}</small>
                    </div>
                    <button
                            :disabled="friendships.actionBusy"
                            type="button"
                            @click="friendships.unblockUser(item.user.id)"
                    >
                        取消拉黑
                    </button>
                </article>
            </section>
        </div>
    </dialog>
</template>
