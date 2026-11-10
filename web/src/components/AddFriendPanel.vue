<script setup lang="ts">
import {shallowRef} from 'vue'

import {useFriendshipStore} from '@/stores/friendships'

const emit = defineEmits<{
    viewProfile: [userID: string]
}>()

const friendships = useFriendshipStore()
const searchQuery = shallowRef('')
const requestingUserID = shallowRef('')
const requestRemark = shallowRef('')
const requestReasonError = shallowRef('')
const hasSearched = shallowRef(false)

// submitSearch 搜索用户名或昵称并展示可添加用户
async function submitSearch() {
    const query = searchQuery.value.trim()
    if (!query) {
        friendships.clearSearch()
        hasSearched.value = false
        return
    }
    requestingUserID.value = ''
    requestReasonError.value = ''
    await friendships.search(query)
    hasSearched.value = true
}

// beginRequest 为指定用户展开好友申请理由输入
function beginRequest(userID: string) {
    requestingUserID.value = userID
    requestRemark.value = ''
    requestReasonError.value = ''
}

// cancelRequest 收起好友申请理由输入
function cancelRequest() {
    requestingUserID.value = ''
    requestRemark.value = ''
    requestReasonError.value = ''
}

// submitRequest 提交带申请理由的好友申请
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
    <section class="add-friend-panel" aria-labelledby="add-friend-title">
        <div class="add-friend-intro">
            <h3 id="add-friend-title">查找账号</h3>
            <p>输入完整或部分用户名、昵称，搜索后再发送好友申请。</p>
        </div>

        <form class="friend-search" role="search" @submit.prevent="submitSearch">
            <label class="sr-only" for="add-friend-search">搜索用户名或昵称</label>
            <input
                    id="add-friend-search"
                    v-model="searchQuery"
                    autocomplete="off"
                    maxlength="50"
                    placeholder="用户名或昵称"
                    type="search"
            />
            <button :disabled="friendships.searching" type="submit">
                {{ friendships.searching ? '搜索中' : '搜索' }}
            </button>
        </form>

        <p v-if="friendships.error" class="form-error" role="alert">
            {{ friendships.error }}
            <button class="text-button" type="button" @click="friendships.clearError">
                关闭
            </button>
        </p>

        <p
                v-if="hasSearched && !friendships.searching && !friendships.searchResults.length"
                class="add-friend-empty"
                role="status"
        >
            没有找到匹配的用户
        </p>

        <div v-if="friendships.searchResults.length" class="search-result-list">
            <article
                    v-for="user in friendships.searchResults"
                    :key="user.id"
                    class="search-result-row"
            >
                <button
                        class="friend-avatar search-result-avatar"
                        :aria-label="`查看${user.nickname || user.username}的资料`"
                        type="button"
                        @click="emit('viewProfile', user.id)"
                >
                    <img
                            v-if="user.avatar_url"
                            :alt="user.nickname || user.username"
                            :src="user.avatar_url"
                    />
                    <template v-else>
                        {{ (user.nickname || user.username).slice(0, 1) }}
                    </template>
                </button>
                <div class="friend-copy">
                    <strong>{{ user.nickname || user.username }}</strong>
                    <small>@{{ user.username }}</small>
                </div>
                <button
                        v-if="requestingUserID !== user.id"
                        :disabled="friendships.actionBusy"
                        class="primary-button compact"
                        type="button"
                        @click="beginRequest(user.id)"
                >
                    添加好友
                </button>
            </article>

            <form
                    v-if="requestingUserID"
                    class="friend-request-form"
                    @submit.prevent="submitRequest(requestingUserID)"
            >
                <label :for="`request-remark-${requestingUserID}`">
                    好友申请理由
                </label>
                <input
                        :id="`request-remark-${requestingUserID}`"
                        v-model="requestRemark"
                        maxlength="200"
                        placeholder="简单说明你为什么要添加对方"
                />
                <span v-if="requestReasonError" class="form-error" role="alert">
                    {{ requestReasonError }}
                </span>
                <div class="friend-request-actions">
                    <button type="button" @click="cancelRequest">取消</button>
                    <button :disabled="friendships.actionBusy" type="submit">
                        {{ friendships.actionBusy ? '发送中' : '发送申请' }}
                    </button>
                </div>
            </form>
        </div>
    </section>
</template>

<style scoped>
.add-friend-panel {
    display: grid;
    gap: 1rem;
}

.add-friend-intro {
    display: grid;
    gap: 0.25rem;
}

.add-friend-intro h3,
.add-friend-intro p {
    margin: 0;
}

.add-friend-intro h3 {
    font-family: var(--font-display);
    font-size: 1.25rem;
    font-weight: 600;
}

.add-friend-intro p,
.add-friend-empty {
    color: var(--muted);
    font-size: 0.75rem;
}

.add-friend-empty {
    margin: 0;
    border: 1px dashed var(--line);
    border-radius: var(--radius-sm);
    padding: 1rem;
    text-align: center;
}

.search-result-list {
    display: grid;
    gap: 0.5rem;
}

.search-result-row {
    display: grid;
    grid-template-columns: 2.5rem minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.75rem;
    border-bottom: 1px solid var(--line);
    padding: 0.75rem 0;
}

.search-result-avatar {
    border: 0;
    padding: 0;
    cursor: pointer;
    overflow: hidden;
}

.search-result-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
}

.search-result-avatar:focus-visible {
    outline: 2px solid var(--focus);
    outline-offset: 2px;
}

.friend-request-form {
    display: grid;
    gap: 0.5rem;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 0.75rem;
    background: var(--surface-muted);
}

.friend-request-form label {
    font-size: 0.75rem;
    font-weight: 700;
}

.friend-request-form input {
    width: 100%;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 0.625rem 0.75rem;
}

.friend-request-actions {
    display: flex;
    justify-content: flex-end;
    gap: 0.5rem;
}

.friend-request-actions button {
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    padding: 0.4375rem 0.75rem;
    background: white;
}

.friend-request-actions button:last-child {
    border-color: var(--signal);
    color: white;
    background: var(--signal);
}
</style>
