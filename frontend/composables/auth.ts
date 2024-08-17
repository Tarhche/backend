import {jwtDecode} from "jwt-decode"

const cookieName = "auth"

const homePage = '/'
const loginPage = '/auth/login'
const dashboardPage = '/dashboard'

async function login(identity: string, password: string) {
    const {data, error} = await useFetch(
        useApiUrlResolver().resolve("api/auth/login"),
        {
            method: "POST",
            body: {
                "identity": identity,
                "password": password
            }
        }
    )

    if (error.value) {
        console.log({
            statusCode: error.value.statusCode,
            data: error.value.data,
        })

        throw new Error("identity or password is wrong");
    }

    persist(data.value.access_token, data.value.refresh_token)

    await navigateTo({path: dashboardPage})
}

async function refresh() {
    const token = refreshToken()
    if (!token) {
        return
    }

    const {data, error} = await useFetch(
        useApiUrlResolver().resolve("api/auth/token/refresh"),
        {
            method: "POST",
            body: {
                "token": token,
            }
        }
    )

    if (error.value) {
        console.log({
            statusCode: error.value.statusCode,
            data: error.value.data,
        })

        throw new Error("cannot refresh access token");
    }

    persist(data.value.access_token, data.value.refresh_token)
}

async function logout() {
    const cookie = useCookie(cookieName)
    if (cookie.value) {
        cookie.value = null
    }

    await navigateTo({path: homePage})
}

async function register(identity: string) {
    const {data, error} = await useFetch(
        useApiUrlResolver().resolve("api/auth/register"),
        {
            method: "POST",
            body: {
                "identity": identity,
            }
        }
    )

    if (error.value) {
        console.log({
            statusCode: error.value.statusCode,
            data: error.value.data,
        })

        throw new Error("registration failed");
    }
}

async function verify(token: string, name:string, username:string, password:string, repassword:string) {
    const {data, error} = await useFetch(
        useApiUrlResolver().resolve("api/auth/verify"),
        {
            method: "POST",
            body: {
                "token": token,
                "name": name,
                "username": username,
                "password": password,
                "repassword": repassword,
            }
        }
    )

    if (error.value) {
        console.log({
            statusCode: error.value.statusCode,
            data: error.value.data,
        })

        throw new Error("verification failed");
    }
}

function accessToken() {
    const authTokens = retrieve()

    if (!authTokens) {
        return
    }

    if (!authTokens.access) {
        return
    }

    return authTokens.access
}

function refreshToken() {
    const authTokens = retrieve()

    if (!authTokens) {
        return
    }

    if (!authTokens.refresh) {
        return
    }

    return authTokens.refresh
}

function isLogin(): Boolean {
    return !!accessToken()
}

async function forgotPassword(identity: string) {
    const {data, error} = await useFetch(
        useApiUrlResolver().resolve("api/auth/password/forget"),
        {
            method: "POST",
            body: {
                "identity": identity,
            }
        }
    )

    if (error.value) {
        console.log({
            statusCode: error.value.statusCode,
            data: error.value.data,
        })

        throw new Error("identity is wrong");
    }
}

async function resetPassword(token: string, password: string) {
    const {data, error} = await useFetch(
        useApiUrlResolver().resolve("api/auth/password/reset"),
        {
            method: "POST",
            body: {
                "token": token,
                "password": password,
            }
        }
    )

    if (error.value) {
        console.log({
            statusCode: error.value.statusCode,
            data: error.value.data,
        })

        throw new Error("reset-password token is either invalid or expired");
    }
}

export function useAuth() {
    return {
        login: login,
        refresh: refresh,
        logout: logout,
        accessToken: accessToken,
        refreshToken: refreshToken,
        isLogin: isLogin,
        forgotPassword: forgotPassword,
        resetPassword: resetPassword,
        register: register,
        verify: verify,
    }
}


function persist(accessToken: string, refreshToken: string) {
    const {exp} = jwtDecode(refreshToken)

    // wrap access and refresh token in an unified object
    const auth = {
        access: accessToken,
        refresh: refreshToken
    }

    const authTokenBase64 = btoa(JSON.stringify(auth))

    const expiresAt = new Date
    expiresAt.setTime(exp * 1000)
    const utcExpirationDate = expiresAt.toUTCString()

    document.cookie = `${cookieName}=${authTokenBase64};expires=${utcExpirationDate};path=/;SameSite=Strict`
}

function retrieve() {
    const cookie = useCookie(cookieName)

    if (!cookie || !cookie.value) {
        return null
    }

    return JSON.parse(atob(cookie.value))
}
