async function show() {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/config`),
        {
            method: "GET",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
        }
    )
}

async function update(userDefaultRoles: string[]) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/config`),
        {
            method: "PUT",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                user_default_roles: userDefaultRoles,
            }
        }
    )
}

export function useDashboardConfig() {
    return {
        show: show,
        update: update,
    }
}
