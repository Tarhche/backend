<template>
  <div v-if="data">
    <div class="container">
        <div class="row justify-content-center py-4" >
            <section class="col-md-12 col-lg-8">
              <figure v-if="data.cover">
                <img class="w-100 pb-4 image-zoomable" :src="resolveFileUrl(data.cover)" :alt="data.title">
              </figure>
              <h1 class="pb-4">{{ data.title }}</h1>
                <article class="article-post" v-html="data.body"></article>
                <div v-if="data.tags" class="card-text">
                    <a class="hashtag" :href="`/hashtags/${tag}`" :key="index" v-for="(tag, index) in data.tags">{{ tag }}</a>
                </div>
            </section>
        </div>
    </div>

    <div v-if="data.elements">
      <template v-for="(element, index) in data.elements">
        <Jumbotron :key="index" v-if="element.type == 'jumbotron'" :body="element.body" />
        <Featured :key="index" v-if="element.type == 'featured'" :body="element.body" />
      </template>
    </div>
  </div>
</template>

<script setup>
    const {uuid} = useRoute().params;
    const resolveFileUrl = useFilesUrlResolver().resolve

    const data = await $fetch(useApiUrlResolver().resolve(`api/articles/${uuid}`))
</script>
