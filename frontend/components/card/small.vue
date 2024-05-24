<template>
    <div class="mb-3 d-flex gap-3 align-items-center">
        <img :style="{height: '100px'}" :src="resolveFileUrl(cover)" v-if="cover">
        <div>
            <h2 class="mb-2 h6 fw-bold">
            <a class="text-dark" v-if="cover && href"  :href="href">{{ cover }}</a>
            </h2>
            <div class="card-text text-muted small" v-if="excerpt" >{{ excerpt }}</div>
            <small class="text-muted" v-if="publishedAt" > {{ publishedAt }} &middot; 5 min read</small>
        </div>
    </div>
</template>

<script setup>
    const props = defineProps(["cover", "href", "title", "excerpt", "publishedAt"])
    const resolveFileUrl = useFilesUrlResolver().resolve
    const {cover, href, title, excerpt, publishedAt} = props
</script>
