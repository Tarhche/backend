async function index() {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/permissions"),
        {
            method: "GET",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`}
        }
    )
}

export function useDashboardPermissions() {
    return {
        index: index,
    }
}
