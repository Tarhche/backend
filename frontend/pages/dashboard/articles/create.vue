<template>
  <div class="container">
    <div class="row mt-4">
      <article class="article col col-md-10 mx-auto">
        <dashboard-articles-create/>
      </article>
    </div>
  </div>
</template>
<script setup>
definePageMeta({
  layout:"dashboard"
})
</script>
<style scoped>
.article{
  margin-top: 10vh;
}
</style>