<script setup>
    const props = defineProps(["href", "title", "tags", "publishedAt"])
    const {href, title, tags, publishedAt} = props
</script>

<template>
    <li>
        <span class="py-1">
            <h6 class="fw-bold">
                <a :href="href" class="text-dark text-decoration-none">{{ title }}</a>
            </h6>
            <div v-if="tags" class="card-text">
                <a class="hashtag" :href="`/hashtags/${tag}`" v-for="tag in tags">{{ tag }}</a>
            </div>
            <small class="text-muted">{{ publishedAt }}</small>
        </span>
    </li>
</template>