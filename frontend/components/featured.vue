<template>
    <div class="container">
        <div class="row my-4">
            <div class="col-lg-6">
                <CardLarge :cover="main.body.cover" :href="`/articles/${main.body.uuid}`" :title="main.body.title" :excerpt="main.body.excerpt" :publishedAt="main.body.published_at" />
            </div>
            <div class="col-lg-6">
                <div class="flex-md-row mb-4 box-shadow h-xl-300">
                    <template v-for="item in aside">
                        <CardSmall :cover="item.body.cover" :href="`/articles/${item.body.uuid}`" :title="item.body.title" :excerpt="item.body.excerpt" :publishedAt="item.body.published_at" />
                    </template>
                </div>
            </div>
        </div>
    </div>
</template>

<script setup>
    const props = defineProps(["body"])
    const {main, aside} = props.body
</script>