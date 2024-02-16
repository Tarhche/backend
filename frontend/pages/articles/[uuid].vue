<script setup>
    const { uuid: articleUUID } = useRoute().params;
    const url = useApiUrlResolver().resolve(`api/articles/${articleUUID}`)
    const resolveFileUrl = filesUrlResolver().resolve
    const { pending, data } = await useFetch(url, {
        pick: ['cover', 'title', 'body', 'published_at', 'tags', 'elements']
    });
</script>

<template>
    <div v-if="!pending">
        <div class="container">
            <div class="row justify-content-center py-4">
                <section class="col-md-12 col-lg-8">
                    <img class="w-100 pb-4 image-zoomable" :src="resolveFileUrl(data.cover)" alt="">
                    <h1 class="pb-4">{{ data.title }}</h1>
                    <article class="article-post" v-html="data.body"></article>
                    <div v-if="data.tags" class="card-text">
                        <a class="hashtag" :href="`/hashtags/${tag}`" v-for="tag in data.tags">{{ tag }}</a>
                    </div>
                </section>
            </div>
        </div>

        <template v-for="element in data.elements">
            <Jumbotron v-if="element.type=='jumbotron'" :body="element.body" />
            <Featured v-if="element.type=='featured'" :body="element.body" />
        </template>
    </div>
</template>
