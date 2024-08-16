async function index(page: number) {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/comments"),
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

async function create(body:string, objectUUID:string, objectType:string, parentUUID?:string, approvedAt?:string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/comments"),
        {
            method: "POST",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                body: body,
                parent_uuid: parentUUID,
                approved_at: approvedAt,
                object_uuid: objectUUID,
                object_type: objectType,
            }
        }
    )
}

async function show(uuid: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/comments/${uuid}`),
        {
            method: "GET",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
        }
    )
}

async function update(uuid: string, body:string, objectUUID:string, objectType:string, parentUUID?:string, approvedAt?:string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/comments`),
        {
            method: "PUT",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                uuid: uuid,
                body: body,
                parent_uuid: parentUUID,
                approved_at: approvedAt,
                object_uuid: objectUUID,
                object_type: objectType,
            }
        }
    )
}

async function remove(uuid: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/comments/${uuid}`),
        {
            method: "DELETE",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`}
        }
    )
}

export function useDashboardComments() {
    return {
        index: index,
        create: create,
        show: show,
        update: update,
        delete: remove,
    }
}
