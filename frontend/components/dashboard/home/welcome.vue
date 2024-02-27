<script setup lang="ts">

</script>

<template>
  <div class="container">
    <div class="row">
      <div class="col-md-12">
        <div class="au-breadcrumb3">
          <div class="au-breadcrumb-left">
            <span class="au-breadcrumb-span">مکان شما : </span>
            <ul class="list-unstyled list-inline au-breadcrumb__list">
              <li class="list-inline-item active">
                <nuxt-link class="text-decoration-none" to="">  خانه </nuxt-link>
              </li>
              <li class="list-inline-item seprate mx-1">
                <span> / </span>
              </li>
              <li class="list-inline-item">داشبورد</li>
            </ul>
          </div>
        </div>
      </div>
    </div>
    <div class="row">
      <div class="col-md-12">
        <div class="welcome2-inner m-t-60">
          <div class="welcome2-greeting">
            <h1 class="title-6">Hi
              <span>John</span>, Welcome back</h1>
            <p>Lorem ipsum dolor sit amet, consectetur adipiscing elit</p>
          </div>
          <form class="form-header form-header2 flex-row-reverse" action="" method="post">
            <input class="au-input au-input--w435" type="text" name="search"
                   placeholder="دنبال چی میگردی...؟">
            <button class="au-btn--submit" type="submit">
              <i class="zmdi zmdi-search"></i>
            </button>
          </form>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>

</style>