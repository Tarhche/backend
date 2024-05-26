<template>
    <div class="card border-0 mb-4 box-shadow h-xl-300">              
        <div :style="{backgroundImage: `url(${coverUrl})`, height: '150px', backgroundSize: 'cover', backgroundRepeat: 'no-repeat'}"></div>               
        <div class="card-body px-0 pb-0 d-flex flex-column align-items-start">
            <h2 class="h4 fw-bold">
                <a class="text-dark" :href="href">{{ title }}</a>
            </h2>
            <p class="card-text">{{ excerpt }}</p>
            <div>
                <small class="d-block">
                    <a class="text-muted" href="#">Favid Rick</a>
                </small>
                <time :datetime="publishedAt" class="text-muted small">{{ useTime().toAgo(publishedAt) }}</time>
            </div>
        </div>
    </div>
</template>

<script setup>
    const props = defineProps(["cover", "href", "title", "excerpt", "publishedAt"])
    const {cover, href, title, excerpt, publishedAt} = props
    const coverUrl = cover ? useFilesUrlResolver().resolve(cover) : null
</script>
