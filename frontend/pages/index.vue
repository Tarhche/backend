<template>
  <div >
    <template v-if="homeData?.elements" v-for="(element,index) in homeData.elements" :key="index">
      <Jumbotron v-if="element.type === 'jumbotron'" :body="element.body"/>
      <Featured v-if="element.type === 'featured'" :body="element.body"/>
    </template>
    <div class="container">
      <div class="row justify-content-between">
        <div class="col-md-8">
          <h5 class="fw-bold spanborder"><span>جدیدترین‌ ها</span></h5>
          <template v-if="homeData.all.length" v-for="({uuid, cover, title, excerpt, published_at},index) in homeData.all"
                    :key="index">
            <CardMedium v-if="homeData" :cover="cover" :href="`/articles/${uuid}`" :title="title" :excerpt="excerpt"
                        :publishedAt="published_at"/>
          </template>

          <p v-else class="alert alert-info">No data!</p>
          <skeleton-loader-medium v-if="!homeData.all.length && !homeData.all " v-for="(item) in 3" :key="item"/>
        </div>
        <div class="col-md-4">
          <h5 class="fw-bold spanborder"><span>پر‌بازدیدترین‌ ها</span></h5>
          <CardList v-if="homeData.popular.length">
            <template v-for="({uuid, title, tags, published_at} , index) in homeData.popular" :key="index">
              <CardListItem :href="`/articles/${uuid}`" :title="title" :tags="tags" :publishedAt="published_at"/>
            </template>
          </CardList>
          <p v-else class="alert alert-info">No data!</p>
          <skeleton-loader-card-list v-if="!homeData.popular && !homeData.popular.length" >
            <skeleton-loader-card-list-item v-for="item in 5" :key="item" />
          </skeleton-loader-card-list>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
useHead({
  title: "صفحه اصلی",
  meta: [
    {name: 'description', content: 'طرح‌چه'},
  ],
  link: [
    {rel: 'canonical', href: '/'}
  ]
})

const { data, error } = await useAsyncData(
	'pages.index',
	() => $fetch(useApiUrlResolver().resolve("api/home"))
)
const homeData = computed(()=>data.value)
</script>
