<template>
  <div>
    <skeleton-loader-articleuuid v-if="!result"/>
    <div class="container" v-else >
        <div class="row justify-content-center py-4" >
            <section class="col-md-12 col-lg-8">
              <figure v-if="result.cover">
                <img class="pb-4 image-zoomable" :src="resolveFileUrl(result.cover)" :alt="result.title">
              </figure>
              <h1 class="pb-4">{{ data.title }}</h1>
                <article class="article-post" v-html="data.body"></article>
                <div v-if="result.tags" class="card-text" dir="rtl">
                    <a class="hashtag" :href="`/hashtags/${tag}`" :key="index" v-for="(tag, index) in result.tags">{{ tag }}</a>
                </div>
            </section>
        </div>
    </div>
    <div v-if="result.elements">
      <template v-for="(element, index) in result.elements" :key="index">
        <Jumbotron :key="index" v-if="element.type === 'jumbotron'" :body="element.body" />
        <Featured :key="index" v-if="element.type === 'featured'" :body="element.body" />
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
	import hljs from 'highlight.js'

	const {uuid} = useRoute().params;

	const resolveFileUrl = useFilesUrlResolver().resolve
  const data = await $fetch(useApiUrlResolver().resolve(`api/articles/${uuid}`))

  const result = computed(()=>data)
	useHead({
		title: data.title,
		meta: [
			{ name: 'description', content: data.excerpt },
		],
		link: [
			{ rel: 'canonical', href: `/articles/${uuid}` }
		]
    })

	onMounted(hljs.highlightAll)
</script>
