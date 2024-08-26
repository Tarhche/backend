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
            <div v-if="params.pending" class="spinner-border spinner-border" role="status">
              <span class="sr-only">Loading...</span>
            </div>
            <button v-else @click.prevent="toggleBookmark()" v-if="useAuth().isLogin()" title="بوک مارک کنید و بعدا بخوانید" type="button" class="btn mx-1">
              <span class="fa-bookmark" :class="{'fa-solid': params.bookmarked, 'fa-regular': !params.bookmarked}"></span>
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

const params = reactive({
  bookmarked: false,
  pending: false,
})

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

if (useAuth().isLogin()) {
  await isBookmarked()
}

async function isBookmarked() {
  try {
    params.pending = true;

    const data = await useUser().$fetch(useApiUrlResolver().resolve('api/bookmarks/exists'), {
      method: 'POST',
      headers: {authorization: `Bearer ${useAuth().accessToken()}`},
      body: {object_type: "article", object_uuid: uuid},
    })
    params.bookmarked = data.exist
  } catch (e) {
    params.error = true;
    console.log(e)
  }
  params.pending = false;
}

async function toggleBookmark() {
  try {
    params.pending = true;

    await useUser().$fetch(useApiUrlResolver().resolve('api/bookmarks'), {
      method: 'PUT',
      headers: {authorization: `Bearer ${useAuth().accessToken()}`},
      body: {
        keep: !params.bookmarked,
        title: data.title,
        object_type: "article",
        object_uuid: uuid
      },
    })
    params.bookmarked = !params.bookmarked
  } catch (e) {
    params.error = true;
    console.log(e)
  }
  params.pending = false;
}
</script>
