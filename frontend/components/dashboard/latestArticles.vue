<template>
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
            <td>
              <span v-if="useTime().isZeroDate(article.published_at)" class="fa fa-times text-danger"></span>
              <span v-else>{{ useTime().toAgo(article.published_at) }}</span>
            </td>
            <td>
              <NuxtLink :to="`/articles/${article.uuid}`" class="btn mx-1 btn-sm btn-primary">
                <span class="fa fa-eye"></span>
              </NuxtLink>
              <NuxtLink :to="`/dashboard/articles/edit/${article.uuid}`" class="btn mx-1 btn-sm btn-primary">
                <span class="fa fa-pen"></span>
              </NuxtLink>
              <button @click.prevent="deleteArticle(article.uuid)" type="button"
                      class="btn mx-1 btn-sm btn-danger">
                <span class="fa fa-trash"></span>
              </button>
            </td>
          </tr>
          <tr v-if="data.items.length == 0">
            <td colspan="5">
              <p class="m-2">هیچ مقاله ای وجود ندارد</p>
            </td>
          </tr>
          </tbody>
        </table>
      </div>
      <p class="text-center">
        <NuxtLink v-if="!pending && data.pagination.total_pages > 1" to="/dashboard/articles">مشاهده بیشتر
        </NuxtLink>
      </p>
    </div>
  </div>
</template>

<script lang="ts" setup>
const {data, pending, error} = await useAsyncData(
    'dashboard.articles.index',
    () => useDashboardArticles().index(1)
)

async function deleteArticle(uuid: string) {
  if (!confirm('آیا میخواهید این مقاله را حذف کنید؟')) {
    return
  }

  await useDashboardArticles().delete(uuid)

  data.items = data.items.filter((article) => article.uuid != article)
}
</script>
