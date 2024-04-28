import type { UseFetchOptions } from 'nuxt/app'
import { defu } from 'defu'

const auth = useAuth()

function userFetch<T> (url: string, options: UseFetchOptions<T> = {}) {
    const defaults: UseFetchOptions<T> = {
        retry: 2,
        retryStatusCodes: [401, 403],

        onRequest({ options }) {
            if (!auth.isLogin()) {
                return
            }

            options.headers = { Authorization: `Bearer ${ auth.accessToken() }` }
        },

        async onResponseError({ response, options }) {
            if (! this.retryStatusCodes.includes(response.status)) {
                return
            }

            await auth.refresh()
        }
    }

    return $fetch(url, defu(options, defaults))
}

function profile() {

}

function updateProfile() {
    
}

function updatePassword() {

}

export function useUser() {
    return {
        $fetch: userFetch,
        profile: profile,
        updateProfile: updateProfile,
        updatePassword: updatePassword,
    }
}

export function useGuest() {
    return {
        $fetch: $fetch,
    }
}