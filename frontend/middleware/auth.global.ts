export default defineNuxtRouteMiddleware((to, from) => {
    if (! to.path.startsWith("/dashboard")) {
        return
    }

    const cookie = useCookie("jwt")
    if (!cookie.value) {
        return navigateTo("/auth/login")
    }
});