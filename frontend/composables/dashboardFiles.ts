async function index() {
    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/files"),
        {
            method: "GET",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`}
        }
    )
}

async function create(file: File) {
    const formData = new FormData();
    formData.append('file', file);

    return useUser().$fetch(
        useApiUrlResolver().resolve("api/dashboard/files"),
        {
            method: "POST",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: formData,
        }
    )
}

async function show(uuid: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/files/${uuid}`),
        {
            method: "GET",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
        }
    )
}

async function update(uuid: string, title: string, excerpt: string, body: string, tags: string[], cover?: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/files`),
        {
            method: "PUT",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`},
            body: {
                uuid: uuid,
                title: title,
                excerpt: excerpt,
                body: body,
                tags: tags,
                cover: cover,
            }
        }
    )
}

async function remove(uuid: string) {
    return useUser().$fetch(
        useApiUrlResolver().resolve(`api/dashboard/files/${uuid}`),
        {
            method: "DELETE",
            lazy: true,
            headers: {authorization: `Bearer ${useAuth().accessToken()}`}
        }
    )
}

export function useDashboardFiles() {
    return {
        index: index,
        create: create,
        show: show,
        update: update,
        delete: remove,
    }
}
