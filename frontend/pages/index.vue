<script setup>
    const url = useApiUrlResolver().resolve(`api/home`)

    const { pending, data, error } = await useFetch(url, {
        pick: ['all', 'popular', 'elements']
    });
</script>

<template>
    <div v-if="!pending">
        <template v-if="data?.elements" v-for="element in data.elements">
            <Jumbotron v-if="element.type=='jumbotron'" :body="element.body" />
            <Featured v-if="element.type=='featured'" :body="element.body" />
        </template>
        <div class="container">
            <div class="row justify-content-between">
                <div class="col-md-8">
                    <h5 class="fw-bold spanborder"><span>جدیدترین‌ ها</span></h5>
                    <template v-if="data.all.length" v-for="{uuid, cover, title, excerpt, published_at} in data.all">
                        <CardMedium :cover="cover" :href="`/articles/${uuid}`" :title="title" :excerpt="excerpt" :publishedAt="published_at" />
                    </template>
                    <p v-else class="alert alert-info">No data!</p>
                </div>
                <div class="col-md-4">
                    <h5 class="fw-bold spanborder"><span>پر‌بازدیدترین‌ ها</span></h5>
                    <CardList v-if="data.popular.length">
                        <template v-for="{uuid, title, tags, published_at} in data.popular">
                            <CardListItem :href="`/articles/${uuid}`" :title="title" :tags="tags" :publishedAt="published_at" />
                        </template>
                    </CardList>
                    <p v-else class="alert alert-info">No data!</p>
                </div>
            </div>
        </div>
    </div>
</template>
