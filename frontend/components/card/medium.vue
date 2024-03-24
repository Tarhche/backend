<script setup>
    const props = defineProps(["cover", "href", "title", "excerpt", "publishedAt"])
    const resolveFileUrl = filesUrlResolver().resolve
    const {cover, href, title, excerpt, publishedAt} = props
</script>

<template>
    <div class="mb-3 d-flex  flex-column flex-md-row justify-content-md-between card border-0 rounded-2  overflow-hidden">
      <div class="card-header col-md-5 col-lg-4 col-xl-3 bg-transparent p-0 border-0 overflow-hidden rounded-0">
        <figure class=" w-100 h-100">
          <img class="w-100 h-100" :src="resolveFileUrl(cover)" :alt="title">
        </figure>
      </div>
        <div class=" card-body px-1 px-md-3 py-2 py-md-1  col-md-6 col-lg-7 col-xl-8  ">
            <h2 class="mb-2 h4 fw-bold">
                <a class="text-dark" :href="href">{{ title }}</a>
            </h2>
            <p class="card-text-summary">{{ excerpt }}</p>
          <div class="card-info d-flex flex-column gap-1 mt-2">
            <div class="card-text text-muted small">Jake Bittle in SCIENCE</div>
            <small class="text-muted">{{ publishedAt }} &middot; 5 min read</small>
          </div>
        </div>
    </div>
</template>
<style scoped>
  p{
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-box-orient: vertical;
    line-clamp: 2;
    -webkit-line-clamp: 2;
  text-align: justify;
  }
  figure img{
    transition: 0.3s;
  }
  figure img:hover{
    scale: 1.05;
  }
  h2{
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-box-orient: vertical;
    line-clamp: 1;
    -webkit-line-clamp: 1;
  }
  @media screen and (max-width: 995.9px) {
    h2{
      font-size: 1.2rem;
    }
    .card-text-summary{
      font-size: 0.9rem ;
      letter-spacing: 0 ;
    }
  }
</style>
