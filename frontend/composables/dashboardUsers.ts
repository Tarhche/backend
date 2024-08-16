async function index(page: number) {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/users"),
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

async function create(email: string, name: string, password: string, avatar?: string, username?: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/users"),
        {
            method: "POST",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                email: email,
                name: name,
                password: password,
                avatar: avatar,
                username: username,
            }
        }
    )
}

async function show(uuid: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/users/${uuid}`),
        {
            method: "GET",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
        }
    )
}

async function update(uuid: string, email: string, name: string, avatar?: string, username?: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/users`),
        {
            method: "PUT",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                uuid: uuid,
                email: email,
                name: name,
                avatar: avatar,
                username: username,
            }
        }
    )
}

async function updatePassword(uuid: string, password: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/users/password`),
        {
            method: "PUT",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                uuid: uuid,
                new_password: password,
            }
        }
    )
}

async function remove(uuid: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/users/${uuid}`),
        {
            method: "DELETE",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`}
        }
    )
}

export function useDashboardUsers() {
    return {
        index: index,
        create: create,
        show: show,
        update: update,
        delete: remove,
        updatePassword: updatePassword,
    }
}
