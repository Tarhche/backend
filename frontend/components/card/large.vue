<script>
    export default {
        props: ['cover', 'href', 'title', 'excerpt', 'publishedAt']
    }
</script>

<template>
    <div class="card border-0 mb-4 box-shadow h-xl-300">              
        <div :style="{backgroundImage: cover, height: '150px', backgroundSize: 'cover', backgroundRepeat: 'no-repeat'}"></div>               
        <div class="card-body px-0 pb-0 d-flex flex-column align-items-start">
            <h2 class="h4 fw-bold">
                <a class="text-dark" :href="href">{{ title }}</a>
            </h2>
            <p class="card-text">{{ excerpt }}</p>
            <div>
                <small class="d-block">
                    <a class="text-muted" href="./author.html">Favid Rick</a>
                </small>
                <small class="text-muted">{{ publishedAt }} &middot; 5 min read</small>
            </div>
        </div>
    </div>
</template>