<script setup>
    const { uuid: articleUUID } = useRoute().params;
    const url = useApiUrlResolver().resolve(`api/articles/${articleUUID}`)
    const { pending, data } = await useFetch(url, {
        pick: ['cover', 'title', 'body', 'published_at', 'tags']
    });
</script>

<template>
    <div class="container">
        <div class="pt-4 pb-4">
            <div class="row justify-content-center">
                <div class="col-md-12 col-lg-8">
                    <article v-if="!pending" class="article-post" v-html="data.body"></article>
                </div>
            </div>
        </div>

        <div v-if="!pending" class="pt-4 pb-4">
            <h5 class="font-weight-bold spanborder"><span>Read next</span></h5>
            <div class="row">
                <div class="col-lg-6">
                    <div class="card border-0 mb-4 box-shadow h-xl-300">
                        <div style="background-image: url(./assets/img/demo/3.jpg); height: 150px; background-size: cover; background-repeat: no-repeat;">
                        </div>
                        <div class="card-body px-0 pb-0 d-flex flex-column align-items-start">
                            <h2 class="h4 font-weight-bold">
                            <a class="text-dark" href="#">Brain Stimulation Relieves Depression Symptoms</a>
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
                </div>
                <div class="col-lg-6">
                    <div class="flex-md-row mb-4 box-shadow h-xl-300">
                        <div class="mb-3 d-flex align-items-center">
                            <img height="80" src="./assets/img/demo/blog4.jpg">
                            <div class="pl-3">
                                <h2 class="mb-2 h6 font-weight-bold">
                                <a class="text-dark" href="./article.html">Nasa's IceSat space laser makes height maps of Earth</a>
                                </h2>
                                <div class="card-text text-muted small">
                                    Jake Bittle in LOVE/HATE
                                </div>
                                <small class="text-muted">Dec 12 · 5 min read</small>
                            </div>
                        </div>
                        <div class="mb-3 d-flex align-items-center">
                            <img height="80" src="./assets/img/demo/blog5.jpg">
                            <div class="pl-3">
                                <h2 class="mb-2 h6 font-weight-bold">
                                <a class="text-dark" href="./article.html">Underwater museum brings hope to Lake Titicaca</a>
                                </h2>
                                <div class="card-text text-muted small">
                                    Jake Bittle in LOVE/HATE
                                </div>
                                <small class="text-muted">Dec 12 · 5 min read</small>
                            </div>
                        </div>
                        <div class="mb-3 d-flex align-items-center">
                            <img height="80" src="./assets/img/demo/blog6.jpg">
                            <div class="pl-3">
                                <h2 class="mb-2 h6 font-weight-bold">
                                <a class="text-dark" href="./article.html">Sun-skimming probe starts calling home</a>
                                </h2>
                                <div class="card-text text-muted small">
                                    Jake Bittle in LOVE/HATE
                                </div>
                                <small class="text-muted">Dec 12 · 5 min read</small>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>
