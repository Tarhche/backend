<script setup>
    const baseUrl = "http://app/"

    const { data } = await useFetch(baseUrl + "api/home");
    const { featured, all, popular } = toRaw(data.value)
</script>

<template>
    <div>
        <Header></Header>

        <div class="container pt-4 pb-4">
            <div class="row">
                <div class="col-lg-6">
                    <CardLarge />
                </div>
                <div class="col-lg-6">
                    <div class="flex-md-row mb-4 box-shadow h-xl-300">
                        <template v-for="{uuid, cover, title, excerpt, published_at} in featured">
                            <CardSmall :cover="cover" :href="uuid" :title="title" :excerpt="excerpt" :publishedAt="published_at" />
                        </template>
                    </div>
                </div>
            </div>
        </div>

        <div class="container">
            <div class="row justify-content-between">
                <div class="col-md-8">
                    <h5 class="fw-bold spanborder"><span>All Stories</span></h5>
                    <template v-for="{uuid, cover, title, excerpt, published_at} in all">
                        <CardMedium :cover="cover" :href="uuid" :title="title" :excerpt="excerpt" :publishedAt="published_at" />
                    </template>
                </div>
                <div class="col-md-4 ps-4">
                    <h5 class="fw-bold spanborder"><span>Popular</span></h5>
                    <CardList>
                        <template v-for="{uuid, cover, title, excerpt, published_at} in all">
                            <CardListItem :cover="cover" :href="uuid" :title="title" :excerpt="excerpt" :publishedAt="published_at" />
                        </template>
                     </CardList>
                </div>
            </div>
        </div>
    </div>
</template>
