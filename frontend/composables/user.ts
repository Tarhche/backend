import type {UseFetchOptions} from 'nuxt/app'
import {defu} from 'defu'

const auth = useAuth()

function userFetch<T>(url: string, options: UseFetchOptions<T> = {}) {
    const defaults: UseFetchOptions<T> = {
        retry: 2,
        retryStatusCodes: [401, 403],

        onRequest({options}) {
            if (!auth.isLogin()) {
                return
            }

            options.headers = {Authorization: `Bearer ${auth.accessToken()}`}
        },

        async onResponseError({response, options}) {
            if (!this.retryStatusCodes.includes(response.status)) {
                return
            }

            await auth.refresh()
        }
    }

    return $fetch(url, defu(options, defaults))
}

async function profile() {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/profile"),
        {
            method: "GET",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`}
        }
    )
}

async function updateProfile(email: string, name?: string, username?: string, avatar?: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/profile"),
        {
            method: "PUT",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                email: email,
                name: name,
                username: username,
                avatar: avatar,
            }
        }
    )
}

async function updatePassword(currentPassword: string, newPassword: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/password"),
        {
            method: "PUT",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                current_password: currentPassword,
                new_password: newPassword,
            }
        }
    )
}

async function roles() {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/profile/roles"),
        {
            method: "GET",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
        }
    )
}

async function permissions(): Promise<string[]> {
    const data = await roles()
    const p:string[] = []

    for(const role of data.items) {
        if (!role.permissions || role.permissions.length == 0) {
            continue
        }

        p.push(...role.permissions)
    }

    return p
}

export function useUser() {
    return {
        $fetch: userFetch,
        profile: profile,
        updateProfile: updateProfile,
        updatePassword: updatePassword,
        roles: roles,
        permissions: permissions,
    }
}

export function useGuest() {
    return {
        $fetch: $fetch,
    }
}