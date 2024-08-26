async function index(page: number) {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/my/comments"),
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

async function show(uuid: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/my/comments/${uuid}`),
        {
            method: "GET",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
        }
    )
}

async function update(uuid: string, body:string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/my/comments`),
        {
            method: "PUT",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                uuid: uuid,
                body: body,
            }
        }
    )
}

async function remove(uuid: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/my/comments/${uuid}`),
        {
            method: "DELETE",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`}
        }
    )
}

export function useDashboardMyComments() {
    return {
        index: index,
        show: show,
        update: update,
        delete: remove,
    }
}
