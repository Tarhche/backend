<template>

  <div class="container">
    <div class="row">
      <dashboardSidebar class="col-md-3 ml-sm-auto"/>
      <main class="col-md-9 ml-sm-auto">
        <!--
          <nav aria-label="breadcrumb">
            <ol class="breadcrumb">
    <li class="breadcrumb-item"><a href="#">Home</a></li>
    <li class="breadcrumb-item active" aria-current="page">Overview</li>
            </ol>
          </nav>
          <h1 class="h2">Dashboard</h1>
          <p>This is the homepage of a simple admin interface which is part of a tutorial written on Themesberg</p>
        -->

        <div class="row">
          <div class="col-12 mb-4 mb-lg-0">
            <div class="card">
              <div class="card-header">جدیدترین مقالات</div>
              <div class="card-body">
                <div class="table-responsive">
                  <table class="table table-striped table-borderless table-hover align-middle">
                    <thead class="border-bottom">
                    <tr>
                      <th scope="col">#</th>
                      <th scope="col">عنوان</th>
                      <th scope="col">تاریخ انتشار</th>
                      <th scope="col">#</th>
                    </tr>
                    </thead>
                    <tbody v-if="!pending">
                    <tr v-for="(article, index) in data.items" :key="index">
                      <th scope="row">{{ index + 1 }}</th>
                      <td>{{ article.title }}</td>
                      <td>{{ article.published_at }}</td>
                      <td>
                        <NuxtLink :to="`/articles/${article.uuid}`" class="btn mx-1 btn-sm btn-primary">
                          <span class="fa fa-eye"></span>
                        </NuxtLink>
                        <NuxtLink :to="`/dashboard/articles/edit/${article.uuid}`" class="btn mx-1 btn-sm btn-primary">
                          <span class="fa fa-pen"></span>
                        </NuxtLink>
                        <button @click.prevent="showModal(article)" type="button"
                                class="btn mx-1 btn-sm btn-danger" data-bs-toggle="modal"
                                data-bs-target="#staticBackdrop">
                          <span class="fa fa-trash"></span>
                        </button>
                      </td>
                    </tr>
                    <tr v-if="data.items.length == 0">
                      <td colspan="5">
                        <p>هیچ مقاله ای وجود ندارد</p>
                      </td>
                    </tr>
                    </tbody>
                  </table>
                </div>
                <NuxtLink v-if="!pending && data.pagination.total_pages > 1" to="/dashboard/articles">مشاهده بیشتر
                </NuxtLink>
              </div>
            </div>
          </div>
        </div>
          <transition name="modal">
            <dashboard-confirm @result="deleteArticle" :data="articleTitle" v-if="show"/>
          </transition>
      </main>
    </div>
  </div>
  <!-- Button trigger modal -->

  <!-- Modal -->
</template>

<script lang="ts" setup>

definePageMeta({
  layout: 'dashboard',
})
const show = ref(false)
const articleTitle = ref()
const feedbackModal = ref()
const {data, pending, error} = await useAsyncData(
    'dashboard.articles.index',
    useDashboardArticles().index
)
function showModal(article:object ){
show.value = true
  articleTitle.value = article
}
async function deleteArticle( result:boolean, uuid: string) {
  if (result){
  await useDashboardArticles().delete(uuid)
  data.value.items = data.value.items.filter((article) => article.uuid != uuid)
    show.value=false
  }else {
    show.value = false
    return
  }

}
</script>
<style scoped lang="scss">
.modal-enter-active {
  animation: modal-in 0.5s forwards;
}
.modal-leave-active {
  animation: modal-in 0.3s reverse forwards;
}
@keyframes modal-in {
  0% {
    transform:translateY(-10px);
    opacity: 0;
  }

  100% {
    transform: translateY(0px);
    opacity: 1;
  }
}</style>