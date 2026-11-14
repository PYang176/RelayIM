<script setup lang="ts">
import {useAuthStore} from '@/stores/auth'
import ChatView from '@/views/ChatView.vue'
import HomeView from '@/views/HomeView.vue'

const auth = useAuthStore()
</script>

<template>
    <ChatView v-if="auth.isAuthenticated"/>
    <HomeView v-else/>
</template>
