import {createRouter, createWebHistory} from 'vue-router'

import {useAuthStore} from '@/stores/auth'

const router = createRouter({
    history: createWebHistory(),
    routes: [
        {
            path: '/',
            name: 'home',
            component: () => import('@/views/RootView.vue'),
            meta: {public: true},
        },
        {
            path: '/login',
            name: 'login',
            redirect: (to) => ({
                name: 'home',
                query: {
                    auth: 'login',
                    ...(typeof to.query.redirect === 'string'
                        ? {redirect: to.query.redirect}
                        : {}),
                },
            }),
            meta: {public: true},
        },
        {
            path: '/chat',
            redirect: '/',
        },
        {
            path: '/:pathMatch(.*)*',
            redirect: '/',
        },
    ],
})

router.beforeEach((to) => {
    const auth = useAuthStore()
    if (!to.meta.public && !auth.isAuthenticated) {
        return {name: 'home', query: {auth: 'login', redirect: to.fullPath}}
    }
    return true
})

export default router
