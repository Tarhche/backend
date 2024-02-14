<script setup>
    const props = defineProps(["cover", "href", "title", "excerpt", "tags", "publishedAt"])
    const resolveFileUrl = filesUrlResolver().resolve
    const {cover, href, title, excerpt, tags, publishedAt} = props
</script>

<template>
    <div class="mb-3 d-flex justify-content-between">
        <div class="pe-3">
            <h2 class="mb-1 h4 fw-bold">
                <a class="text-dark" :href="href">{{ title }}</a>
            </h2>
            <p>{{ excerpt }}</p>
            <div class="card-text text-muted small">Jake Bittle in SCIENCE</div>
            <div v-if="tags" class="card-text mb-1">
                <a class="pe-1 hashtag" :href="`/hashtags/${tag}`" v-for="tag in tags">{{ tag }}</a>
            </div>
            <small class="text-muted">{{ publishedAt }} &middot; 5 min read</small>
        </div>
        <img :style="{height: '120px'}" :src="resolveFileUrl(cover)">
    </div>
</template>