<template>
  <div v-if="data">
    <div class="container">
      <div class="row justify-content-center py-4">
        <section class="col-md-12 col-lg-8">
          <h1 class="pb-3">{{ data.title }}</h1>
          <div class="d-flex my-2 justify-content-between">
            <div>
              <span class="small">
                <span class="fa-regular fa-clock"></span>
                <span class="mx-1">تاریخ انتشار:</span>
                <time class="text-muted" :datetime="data.published_at">{{ useTime().toAgo(data.published_at) }}</time>
              </span>
            </div>
            <button v-if="useAuth().isLogin()" title="بوک مارک کنید و بعدا بخوانید" type="button" class="btn mx-1">
              <span class="fa-regular fa-bookmark"></span>
            </button>
          </div>
          <figure v-if="data.video">
            <video-player width="100%" :video="resolveFileUrl(data.video)" :poster="resolveFileUrl(data.cover)"/>
            <figcaption class="alert alert-secondary my-3 text-wrap">
              <span class="fa-solid fa-book fa-flip-horizontal fa-xl"></span>
              {{ data.excerpt }}
            </figcaption>
          </figure>
          <figure v-else v-if="data.cover">
            <img class="image-zoomable" :src="resolveFileUrl(data.cover)" :alt="data.title">
            <figcaption class="alert alert-secondary my-3 text-wrap">
              <span class="fa-solid fa-book fa-flip-horizontal fa-xl"></span>
              {{ data.excerpt }}
            </figcaption>
          </figure>
          <article class="article-post" v-html="data.body"></article>
          <div v-if="data.tags" class="card-text">
            <a class="hashtag" :href="`/hashtags/${tag}`" :key="index" v-for="(tag, index) in data.tags">{{ tag }}</a>
          </div>
          <aside class="mt-5">
            <Comments objectType="article" :objectUUID="uuid"/>
          </aside>
        </section>
      </div>
    </div>
    <div v-if="data.elements">
      <template v-for="(element, index) in data.elements" :key="index">
        <Jumbotron :key="index" v-if="element.type === 'jumbotron'" :body="element.body"/>
        <Featured :key="index" v-if="element.type === 'featured'" :body="element.body"/>
      </template>
    </div>
  </div>
</template>

<script setup>
import hljs from 'highlight.js'

const {uuid} = useRoute().params;

const resolveFileUrl = useFilesUrlResolver().resolve
const data = await $fetch(useApiUrlResolver().resolve(`api/articles/${uuid}`))

useHead({
  name: data.title,
  meta: [
    {name: 'description', content: data.excerpt},
  ],
  link: [
    {rel: 'canonical', href: `/articles/${uuid}`}
  ]
})

onMounted(hljs.highlightAll)
</script>
