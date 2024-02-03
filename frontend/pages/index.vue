<script setup>
    const url = useApiUrlResolver().resolve(`api/home`)
    const { pending, data } = await useFetch(url, {
        pick: ['featured', 'all', 'popular']
    });
</script>

<template>
    <div>
        <Header></Header>

        <div v-if="!pending" class="container">
            <div class="pt-4 pb-4">
                <div class="row">
                    <div class="col-lg-6">
                        <CardLarge />
                    </div>
                    <div class="col-lg-6">
                        <div class="flex-md-row mb-4 box-shadow h-xl-300">
                            <template v-for="{uuid, cover, title, excerpt, published_at} in data.featured">
                                <CardSmall :cover="cover" :href="`/articles/${uuid}`" :title="title" :excerpt="excerpt" :publishedAt="published_at" />
                            </template>
                        </div>
                    </div>
                </div>
            </div>

            <div class="row justify-content-between">
                <div class="col-md-8">
                    <h5 class="fw-bold spanborder"><span>All Stories</span></h5>
                    <template v-for="{uuid, cover, title, excerpt, published_at} in data.all">
                        <CardMedium :cover="cover" :href="`/articles/${uuid}`" :title="title" :excerpt="excerpt" :publishedAt="published_at" />
                    </template>
                </div>
                <div class="col-md-4 ps-4">
                    <h5 class="fw-bold spanborder"><span>Popular</span></h5>
                    <CardList>
                        <template v-for="{uuid, cover, title, excerpt, published_at} in data.popular">
                            <CardListItem :cover="cover" :href="`/articles/${uuid}`" :title="title" :excerpt="excerpt" :publishedAt="published_at" />
                        </template>
                     </CardList>
                </div>
            </div>
        </div>
    </div>
</template>
