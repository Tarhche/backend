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
            <li class="breadcrumb-item active" aria-current="page">کامنت ها</li>
          </ol>
        </nav>

        <div class="row">
          <div class="col-12 mb-4 mb-lg-0">
            <div class="card">
              <div class="card-header d-flex justify-content-between">
                <h4>کامنت ها</h4>
              </div>
              <div class="card-body">
                <div class="table-responsive">
                  <table class="table table-striped table-borderless table-hover align-middle">
                    <thead class="border-bottom">
                    <tr>
                      <th scope="col">#</th>
                      <th scope="col">محتوا</th>
                      <th scope="col">تاریخ انتشار</th>
                      <th scope="col">تاریخ ثبت</th>
                      <th scope="col">نویسنده</th>
                      <th scope="col">#</th>
                    </tr>
                    </thead>
                    <tbody v-if="!params.pending">
                    <tr v-for="(comment, index) in params.data.items" :key="index">
                      <th scope="row">{{ index + 1 }}</th>
                      <td>{{ trim(comment.body, 25) }}</td>
                      <td>
                        <span v-if="useTime().isZeroDate(comment.approved_at)" class="fa fa-times text-danger"></span>
                        <span v-else>{{ useTime().toFormat(comment.approved_at) }}</span>
                      </td>
                      <td>
                        <span v-if="useTime().isZeroDate(comment.created_at)" class="fa fa-times text-danger"></span>
                        <span v-else>{{ useTime().toAgo(comment.created_at) }}</span>
                      </td>
                      <td>
                        <span v-if="comment.author && comment.author.uuid">
                          <img class="rounded" width="40" :src="useFilesUrlResolver().resolve(comment.author.avatar)"/>
                          <small class="mx-1">{{ comment.author.name }}</small>
                        </span>
                        <span v-else class="text-danger">کاربر ناشناس</span>
                      </td>
                      <td>
                        <NuxtLink :to="`/${comment.object_type}s/${comment.object_uuid}`" target="_blank" class="btn mx-1 btn-sm btn-primary">
                          <span class="fa fa-eye"></span>
                        </NuxtLink>
                        <NuxtLink :to="`/dashboard/comments/edit/${comment.uuid}`" class="btn mx-1 btn-sm btn-primary">
                          <span class="fa fa-pen"></span>
                        </NuxtLink>
                        <button @click.prevent="deleteComment(comment.uuid)" type="button"
                                class="btn mx-1 btn-sm btn-danger">
                          <span class="fa fa-trash"></span>
                        </button>
                      </td>
                    </tr>
                    <tr v-if="params.data.items.length == 0">
                      <td colspan="6">
                        <p>هیچ کامنتی وجود ندارد</p>
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
  name: "کامنت ها"
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
      'dashboard.comments.index',
      () => useDashboardComments().index(page)
  )

  params.data = data
  params.pending = pending
  params.error = error
}

async function deleteComment(uuid: string) {
  if (!confirm('آیا میخواهید این کامنت را حذف کنید؟')) {
    return
  }

  await useDashboardComments().delete(uuid)

  params.data.items = params.data.items.filter((comment) => comment.uuid != uuid)
}
</script>
