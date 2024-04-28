<template>
  <div class="container">
    <div class="row bg-lightblue mb-4">
      <div class="col-md-6 p-5">
        <h1 class="display-7 fw-bold">{{ title }}</h1>
        <p class="fs-5">{{ excerpt }}</p>
        <a :href="`/articles/${uuid}`" class="btn btn-dark btn-lg">ادامه مطلب</a>
      </div>
      <div class="col-md-6 d-none d-md-block" :style="{backgroundSize: 'cover', backgroundImage: `url(${coverUrl})`}"></div>
    </div>
  </div>
</template>

<script setup>
  const props = defineProps(["body"])
  const {uuid, cover, href, title, excerpt, publishedAt} = props.body.body
  const coverUrl = cover ? useFilesUrlResolver().resolve(cover) : null
</script>
