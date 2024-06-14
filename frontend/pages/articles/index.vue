<template>
    <div v-if="!pending">
        <div class="container mt-5 mb-5">
            <div class="row">
                <div class="col-8 mx-auto">
                    <h1 class="fw-bold spanborder"><span>All articles</span></h1>
                    <template v-if="data.items.length > 0" v-for="({uuid, cover, title, excerpt, published_at} , index) in data.items" :key="index">
                        <CardMedium :cover="cover" :href="`/articles/${uuid}`" :title="title" :excerpt="excerpt" :publishedAt="published_at" />
                    </template>
                    <p v-else class="alert alert-info">No data!</p>
                </div>
            </div>
        </div>
    </div>
</template>

<script setup>
    useHead({
        title: `مقاله ها`,
        meta: [
            { name: 'description', content: `مقاله ها` },
        ],
        link: [
            { rel: 'canonical', href: '/articles' }
        ]
    })

    const url = useApiUrlResolver().resolve("api/articles")
    const { pending, data } = await useFetch(url, {
        pick: ['items', 'pagination']
    })
</script>