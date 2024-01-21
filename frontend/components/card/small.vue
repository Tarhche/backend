<script>
    export default {
        props: ['cover', 'href', 'title', 'excerpt', 'publishedAt']
    }
</script>

<template>
    <div class="mb-3 d-flex align-items-center">
        <img :style="{height: '80px'}" :src="cover">
        <div class="ps-3">
            <h2 class="mb-2 h6 fw-bold">
            <a class="text-dark" :href="href">{{ title }}</a>
            </h2>
            <div class="card-text text-muted small">{{ excerpt }}</div>
            <small class="text-muted"> {{ publishedAt }} &middot; 5 min read</small>
        </div>
    </div>
</template>