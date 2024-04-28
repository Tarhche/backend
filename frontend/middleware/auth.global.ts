export default defineNuxtRouteMiddleware((to, from) => {
    if (! to.path.startsWith("/dashboard")) {
        return
    }

    if (! useAuth().isLogin()) {
        return navigateTo("/auth/login")
    }
});