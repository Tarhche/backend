<script setup>
  const props = defineProps(["body"])
  const {uuid, cover, href, title, excerpt, publishedAt} = props.body.body
  const coverUrl = cover ? filesUrlResolver().resolve(cover) : null
</script>

<template>
  <div class="jumbotron jumbotron-fluid mb-3 bg-lightblue position-relative">
    <div class="h-100 tofront">
      <div class="row justify-content-between">
        <div class="col-md-6 p-5 align-self-center">
          <h1 class="secondfont mb-3 fw-bold">{{ title }}</h1>
          <p class="mb-3">
            {{ excerpt }}
          </p>
          <a :href="`/articles/${uuid}`" class="btn btn-dark">Read More</a>
        </div>
        <div class="col-md-6 d-none d-md-block pe-0" :style="{backgroundSize: 'cover', backgroundImage: `url(${coverUrl})`}"></div>
      </div>
    </div>
  </div>
</template>