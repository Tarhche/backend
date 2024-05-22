
async function index() {
    return useUser().$fetch(
		useApiUrlResolver().resolve("api/dashboard/articles"),
		{
            method: "GET",
	    	lazy: true,
    		headers: {authorization: `Bearer ${useAuth().accessToken()}`}
  		}
	)
}

async function create(title:string, excerpt:string, body:string, tags:string[], publishedAt?:string, cover?:string) {
    return useUser().$fetch(
		useApiUrlResolver().resolve("api/dashboard/articles"),
		{
            method: "POST",
	    	lazy: true,
    		headers: {authorization: `Bearer ${useAuth().accessToken()}`},
			body: {
				title: title,
				excerpt: excerpt,
				body: body,
				tags: tags,
				published_at: publishedAt,
				cover: cover,
			}
  		}
	)
}

async function show(uuid:string) {
    return useUser().$fetch(
		useApiUrlResolver().resolve(`api/dashboard/articles/${uuid}`),
		{
            method: "GET",
	    	lazy: true,
    		headers: {authorization: `Bearer ${useAuth().accessToken()}`},
  		}
	)
}

async function update(uuid:string, title:string, excerpt:string, body:string, tags:string[], publishedAt?:string, cover?:string) {
    return useUser().$fetch(
		useApiUrlResolver().resolve(`api/dashboard/articles`),
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
				published_at: publishedAt,
				cover: cover,
			}
  		}
	)
}

async function remove(uuid:string) {
    return useUser().$fetch(
		useApiUrlResolver().resolve(`api/dashboard/articles/${uuid}`),
		{
            method: "DELETE",
	    	lazy: true,
    		headers: {authorization: `Bearer ${useAuth().accessToken()}`}
  		}
	)
}

export function useDashboardArticles() {
    return {
        index: index,
        create: create,
		show: show,
        update: update,
        delete: remove,
    }
}
