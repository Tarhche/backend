<script setup>
import {useTarcheApi} from "~/store/tarche.js";

const store = useTarcheApi()
store.fetchHomeData()
const url = useApiUrlResolver().resolve(`api/home`)
const {pending, data, error} = await useFetch(url, {
  pick: ['all', 'popular', 'elements']
});
if (error.value) {
  console.log(error.value.message, error.value.statusCode)
}
const responses = computed(()=> store.getHome)
</script>

<template>
  <div v-if="responses">
    <template v-if="responses?.elements" v-for="element in responses.elements">
      <Jumbotron v-if="element.type=='jumbotron'" :body="element.body"/>
      <Featured v-if="element.type=='featured'" :body="element.body"/>
    </template>
    <div class="container">
      <div class="row justify-content-between">
        <div class="col-md-8">
          <h5 class="fw-bold spanborder"><span>جدیدترین‌ ها</span></h5>
          <template v-if="responses.all.length" v-for="{uuid, cover, title, excerpt, published_at} in responses.all">
            <CardMedium :cover="cover" :href="`/articles/${uuid}`" :title="title" :excerpt="excerpt"
                        :publishedAt="published_at"/>
          </template>
          <p v-else class="alert alert-info">No data!</p>
        </div>
        <div class="col-md-4">
          <h5 class="fw-bold spanborder"><span>پر‌بازدیدترین‌ ها</span></h5>
          <CardList v-if="responses.popular.length">
            <template v-for="{uuid, title, tags, published_at} in responses.popular">
              <CardListItem :href="`/articles/${uuid}`" :title="title" :tags="tags" :publishedAt="published_at"/>
            </template>
          </CardList>
          <p v-else class="alert alert-info">No data!</p>
        </div>
      </div>
    </div>
  </div>
  <div v-else>
    <loading-loader/>
  </div>
</template>