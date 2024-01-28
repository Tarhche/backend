<script setup>
    const url = useApiUrlResolver().resolve("api/articles")
    const { pending, data } = await useFetch(url, {
        pick: ['items', 'pagination']
    });
</script>

<template>
    <div class="container mt-5 mb-5">
        <div v-if="!pending" class="row">
            <div class="col-md-8">
                <h5 class="font-weight-bold spanborder"><span>Featured</span></h5>
                <div class="card border-0 mb-5 box-shadow">
                    <div style="background-image: url(./assets/img/demo/1.jpg); height: 350px; background-size: cover; background-repeat: no-repeat;">
                    </div>
                    <div class="card-body px-0 pb-0 d-flex flex-column align-items-start">
                        <h2 class="h2 font-weight-bold">
                        <a class="text-dark" href="./article.html">Brain Stimulation Relieves Depression Symptoms</a>
                        </h2>
                        <p class="card-text">
                            Researchers have found an effective target in the brain for electrical stimulation to improve mood in people suffering from depression.
                        </p>
                        <div>
                            <small class="d-block"><a class="text-muted" href="./author.html">Favid Rick</a></small>
                            <small class="text-muted">Dec 12 · 5 min read</small>
                        </div>
                    </div>
                </div>
                <h5 class="font-weight-bold spanborder"><span>Latest</span></h5>

                <template v-for="{uuid, cover, title, excerpt, published_at} in data.articles">
                    <CardMedium :cover="cover" :href="`/articles/${uuid}`" :title="title" :excerpt="excerpt" :publishedAt="published_at" />
                </template>
            </div>
            <div class="col-md-4 pl-4">
                <div class="sticky-top">
                    <h5 class="font-weight-bold spanborder"><span>Popular</span></h5>
                    <ol class="list-featured">
                        <li>
                        <span>
                        <h6 class="font-weight-bold">
                        <a href="./article.html" class="text-dark">Did Supernovae Kill Off Large Ocean Animals?</a>
                        </h6>
                        <p class="text-muted">
                            Jake Bittle in SCIENCE
                        </p>
                        </span>
                        </li>
                        <li>
                        <span>
                        <h6 class="font-weight-bold">
                        <a href="./article.html" class="text-dark">Humans Reversing Climate Clock: 50 Million Years</a>
                        </h6>
                        <p class="text-muted">
                            Jake Bittle in SCIENCE
                        </p>
                        </span>
                        </li>
                        <li>
                        <span>
                        <h6 class="font-weight-bold">
                        <a href="./article.html" class="text-dark">Unprecedented Views of the Birth of Planets</a>
                        </h6>
                        <p class="text-muted">
                            Jake Bittle in SCIENCE
                        </p>
                        </span>
                        </li>
                        <li>
                        <span>
                        <h6 class="font-weight-bold">
                        <a href="./article.html" class="text-dark">Effective New Target for Mood-Boosting Brain Stimulation Found</a>
                        </h6>
                        <p class="text-muted">
                            Jake Bittle in SCIENCE
                        </p>
                        </span>
                        </li>
                    </ol>
                </div>
            </div>
        </div>
    </div>
</template>