
<template>
  <div class="container" >
    <div class="row justify-content-center py-4" aria-hidden="true">
      <section class="col-md-12 col-lg-8 placeholder-glow ">
        <figure class="placeholder col-12 rounded" style="height: 400px;"  >
          <img class="pb-4 image-zoomable " src="" alt="">
        </figure>
        <h1 class="pb-4"><span class="placeholder rounded col-10"></span></h1>
        <article class="article-post placeholder-glow" >
          <div class="mt-4" v-for="(item ) in 5" :key="item">
            <span class="placeholder rounded col-5"></span>
            <span class="placeholder rounded col-12"></span>
            <span class="placeholder rounded col-12"></span>
            <span class="placeholder rounded col-12"></span>
            <span class="placeholder rounded col-12"></span>
            <span class="placeholder rounded col-12"></span>
            <span class="placeholder rounded col-12"></span>
          </div>
        </article>
        <div class="card-text">
          <a class="hashtag placeholder-glow" href="" >
            <span class="placeholder col-2 mx-1"></span>
            <span class="placeholder col-1 mx-1"></span>
          </a>
        </div>
      </section>
    </div>
  </div>
</template>
