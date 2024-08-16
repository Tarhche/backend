async function index(page: number) {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/roles"),
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

async function create(name: string, description: string, permissions: string[], userUUIDs: string[]) {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/roles"),
        {
            method: "POST",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                name: name,
                description: description,
                permissions: permissions,
                user_uuids: userUUIDs,
            }
        }
    )
}

async function show(uuid: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/roles/${uuid}`),
        {
            method: "GET",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
        }
    )
}

async function update(uuid: string, name: string, description: string, permissions: string[], userUUIDs: string[]) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/roles`),
        {
            method: "PUT",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                uuid: uuid,
                name: name,
                description: description,
                permissions: permissions,
                user_uuids: userUUIDs,
            }
        }
    )
}

async function remove(uuid: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/roles/${uuid}`),
        {
            method: "DELETE",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`}
        }
    )
}

export function useDashboardRoles() {
    return {
        index: index,
        create: create,
        show: show,
        update: update,
        delete: remove,
    }
}
