<template>
  <div class="container">
    <div class="row">
      <dashboardSidebar class="col-md-3 ml-sm-auto"/>
      <main class="col-md-9 ml-sm-auto">

        <nav aria-label="breadcrumb">
          <ol class="breadcrumb">
            <li class="breadcrumb-item">
              <NuxtLink to="/dashboard">داشبورد</NuxtLink>
            </li>
            <li class="breadcrumb-item active" aria-current="page">بوکمارک ها</li>
          </ol>
        </nav>

        <div class="row">
          <div class="col-12 mb-4 mb-lg-0">
            <div class="card">
              <div class="card-header d-flex justify-content-between">
                <h4>بوکمارک ها</h4>
              </div>
              <div class="card-body">
                <div class="table-responsive">
                  <table class="table table-striped table-borderless table-hover align-middle">
                    <thead class="border-bottom">
                    <tr>
                      <th scope="col">#</th>
                      <th scope="col">عنوان</th>
                      <th scope="col">تاریخ بوکمارک شدن</th>
                      <th scope="col">#</th>
                    </tr>
                    </thead>
                    <tbody v-if="!params.pending">
                    <tr v-for="(bookmark, index) in params.data.items" :key="index">
                      <th scope="row">{{ index + 1 }}</th>
                      <td>{{ bookmark.title }}</td>
                      <td>
                        <span v-if="useTime().isZeroDate(bookmark.created_at)" class="fa fa-times text-danger"></span>
                        <span v-else>{{ useTime().toAgo(bookmark.created_at) }}</span>
                      </td>
                      <td>
                        <NuxtLink :to="`/${bookmark.object_type}s/${bookmark.object_uuid}`" target="_blank" class="btn mx-1 btn-sm btn-primary">
                          <span class="fa fa-eye"></span>
                        </NuxtLink>
                        <button @click.prevent="deleteBookmark(bookmark.object_type, bookmark.object_uuid)" type="button"
                                class="btn mx-1 btn-sm btn-danger">
                          <span class="fa fa-trash"></span>
                        </button>
                      </td>
                    </tr>
                    <tr v-if="params.data.items.length == 0">
                      <td colspan="6">
                        <p class="m-2">هیچ بوکمارکی وجود ندارد</p>
                      </td>
                    </tr>
                    </tbody>
                  </table>
                </div>
                <nav v-if="!params.pending" aria-label="Page navigation example">
                  <Pagination @paginate="load" :current="params.data.pagination.current_page"
                              :pages="params.data.pagination.total_pages"/>
                </nav>
              </div>
            </div>
          </div>
        </div>
      </main>
    </div>
  </div>
</template>

<script lang="ts" setup>
definePageMeta({
  layout: 'dashboard',
})

useHead({
  name: "بوکمارک ها"
})

const params = reactive({
  data: [],
  pending: true,
  error: null,
})

await load((useRoute().query.page) || 1)

function trim(str, maxLength): string {
  if (str.length > maxLength) {
    return str.substring(0, maxLength) + "...";
  }

  return str;
}

async function load(page: number) {
  const {data, pending, error} = await useAsyncData(
      'dashboard.my.bookmarks.index',
      () => useDashboardMyBookmarks().index(page)
  )

  params.data = data
  params.pending = pending
  params.error = error
}

async function deleteBookmark(type: string, uuid: string) {
  if (!confirm('آیا میخواهید این بوکمارک را حذف کنید؟')) {
    return
  }

  await useDashboardMyBookmarks().delete(type, uuid)

  params.data.items = params.data.items.filter((bookmark) => bookmark.object_type === type && bookmark.object_uuid != uuid)
}
</script>
