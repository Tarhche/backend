<script setup>
    const props = defineProps(["cover", "href", "title", "excerpt", "publishedAt"])
    const resolveFileUrl = filesUrlResolver().resolve
    const {cover, href, title, excerpt, publishedAt} = props
</script>

<template>
    <div class="mb-3 d-flex column-gap-3 justify-content-between">
        <div class="pr-3">
            <h2 class="mb-2 h4 fw-bold">
                <a class="text-dark" :href="href">{{ title }}</a>
            </h2>
            <p>{{ excerpt }}</p>
            <div class="card-text text-muted small">Jake Bittle in SCIENCE</div>
            <small class="text-muted">{{ publishedAt }} &middot; 5 min read</small>
        </div>
        <img :style="{height: '120px'}" :src="resolveFileUrl(cover)">
    </div>
</template>