<script setup lang="ts">

</script>

<template>
  <div class="search" dir="rtl">
    <div class="row">
      <div class="card rounded py-3">
        <div class="card-header bg-white border-0">
          <div class="title mt-4"><h1 class="search-title text-center">جستوجوی پیشرفته</h1></div>
        </div>
        <div class="card-body">
          <div class="form">
            <form action="" >
              <div class="flex flex-row w-100 row mx-auto">
                <div class="col-md-4" >
                  <label for="author-name"  class="form-label mt-2">نام نویسنده : </label>
                  <select name="" class="form-select" id="author-name">
                    <option value="عیسی خانزادی"></option>
                    <option value="مهدی خانزادی"></option>
                    <option value="خانزادی"></option>
                  </select>
                </div>
                <div class="col-md-4">
                  <label for="titer-name" class="form-label mt-2">تیتر : </label>
                  <input type="text" id="titer-name" class="form-control rounded">
                </div>
                <div class="col-md-4">
                  <label for="date" class="form-label mt-2">تاریخ : </label>
                  <input type="text" id="date" class="form-control rounded">
                </div>
                <div class="mt-4 ">
                  <button class="w-100 py-1 bg-primary rounded text-light">جستوجو</button>
                </div>
              </div>
              <div>
              </div>
            </form>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.search-title {
  font-size: 18px;
}
</style>