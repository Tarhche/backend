async function index(page: number) {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/my/bookmarks"),
        {
            method: "GET",
            params: {
                page: page,
            },
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`}
        }
    )
}

async function remove(type: string, uuid: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/my/bookmarks`),
        {
            method: "DELETE",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                object_type: type,
                object_uuid: uuid,
            }
        }
    )
}

export function useDashboardMyBookmarks() {
    return {
        index: index,
        delete: remove,
    }
}
