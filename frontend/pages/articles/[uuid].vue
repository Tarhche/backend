<template>
  <div v-if="data">
    <div class="container">
        <div class="row justify-content-center py-4" >
            <section class="col-md-12 col-lg-8">
              <h1 class="pb-3">{{ data.title }}</h1>
              <div class="my-3">
                <span class="small">
                  <span class="fa-regular fa-clock"></span>
                  <span class="mx-1">تاریخ انتشار:</span>
                  <time class="text-muted" :datetime="data.published_at">{{ useTime().toAgo(data.published_at) }}</time>
                </span>
              </div>
              <figure v-if="data.cover">
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
                <Comments :data="comments.items"/>
              </aside>
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
import hljs from 'highlight.js'

const {uuid} = useRoute().params;

	const resolveFileUrl = useFilesUrlResolver().resolve
	const data = await $fetch(useApiUrlResolver().resolve(`api/articles/${uuid}`))
const comments = await $fetch( useApiUrlResolver().resolve('api/comments'), {
  query: {
    object_type: 'article',
    object_uuid: uuid,
    page: 1
  }
})
	useHead({
		title: data.title,
		meta: [
			{ name: 'description', content: data.excerpt },
		],
		link: [
			{ rel: 'canonical', href: `/articles/${uuid}` }
		]
  })

	onMounted(()=>hljs.highlightAll())
</script>
