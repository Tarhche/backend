<template>
    <li>
        <div class="py-1">
            <h6 class="fw-bold">
                <a :href="href" class="text-dark">{{ title }}</a>
            </h6>
            <div v-if="tags" class="card-text">
                <a class="hashtag" :href="`/hashtags/${tag}`" v-for="(tag , index) in tags" :key="index">{{ tag }}</a>
            </div>
        </div>
    </li>
</template>

<script setup>
    const props = defineProps(["href", "title", "tags", "publishedAt"])
    const {href, title, tags, publishedAt} = props
</script>
