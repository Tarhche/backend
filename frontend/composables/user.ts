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

async function updateProfile(email:string, name?:string, username?:string, avatar?:string) {
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

async function updatePassword(currentPassword:string, newPassword:string) {
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