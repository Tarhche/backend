<script setup>
    definePageMeta({layout: "dashboard"})

</script>

<template>
  <div class="container">
    <div class="row">
      <aside class="menu-sidebar d-none d-lg-block col-md-3 position-relative end-0 top-0 ">
          <dashboard-home-side-bar-menu/>
      </aside>
      <main class="col-md-9 ">
        <div class="search-article">
          <dashboard-articles-search/>
        </div>
        <article class="article ">
          <dashboard-articles-list/>
        </article>

      </main>


    </div>
  </div>
</template>

<style scoped>
.row{
  margin-top: 10vh;
}
</style>