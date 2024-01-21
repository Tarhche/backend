<script>
    export default {
        props: ['cover', 'href', 'title', 'excerpt', 'publishedAt']
    }
</script>

<template>
    <div class="mb-3 d-flex justify-content-between">
        <div class="pe-3">
            <h2 class="mb-1 h4 fw-bold">
                <a class="text-dark" :href="href">{{ title }}</a>
            </h2>
            <p>{{ excerpt }}</p>
            <div class="card-text text-muted small">Jake Bittle in SCIENCE</div>
            <small class="text-muted">{{ publishedAt }} &middot; 5 min read</small>
        </div>
        <img :style="{height: '120px'}" :src="cover">
    </div>
</template>