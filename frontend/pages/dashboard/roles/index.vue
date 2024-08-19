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
            <li class="breadcrumb-item active" aria-current="page">نقش ها</li>
          </ol>
        </nav>

        <div class="row">
          <div class="col-12 mb-4 mb-lg-0">
            <div class="card">
              <div class="card-header d-flex justify-content-between">
                <h4>نقش ها</h4>
                <NuxtLink class="btn btn-primary" to="/dashboard/roles/create">نقش جدید</NuxtLink>
              </div>
              <div class="card-body">
                <div class="table-responsive">
                  <table class="table table-striped table-borderless table-hover align-middle">
                    <thead class="border-bottom">
                    <tr>
                      <th scope="col">#</th>
                      <th scope="col">عنوان</th>
                      <th scope="col">توضیحات</th>
                      <th scope="col">#</th>
                    </tr>
                    </thead>
                    <tbody v-if="!params.pending">
                    <tr v-for="(role, index) in params.data.items" :key="index">
                      <th scope="row">{{ index + 1 }}</th>
                      <td>{{ role.name }}</td>
                      <td>{{ role.description }}</td>
                      <td>
                        <NuxtLink :to="`/dashboard/roles/edit/${role.uuid}`" class="btn mx-1 btn-sm btn-primary">
                          <span class="fa fa-pen"></span>
                        </NuxtLink>
                        <button @click.prevent="deleteRole(role.uuid)" type="button" class="btn mx-1 btn-sm btn-danger">
                          <span class="fa fa-trash"></span>
                        </button>
                      </td>
                    </tr>
                    <tr v-if="params.data.items.length == 0">
                      <td colspan="4">
                        <p class="m-2">هیچ نقشی وجود ندارد</p>
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
  name: "نقش ها"
})

const params = reactive({
  data: [],
  pending: true,
  error: null,
})

await load((useRoute().query.page) || 1)

async function load(page: number) {
  const {data, pending, error} = await useAsyncData(
      'dashboard.roles.index',
      () => useDashboardRoles().index(page)
  )

  params.data = data
  params.pending = pending
  params.error = error
}

async function deleteRole(uuid: string) {
  if (!confirm('آیا میخواهید این نقش را حذف کنید؟')) {
    return
  }

  await useDashboardRoles().delete(uuid)

  params.data.items = params.data.items.filter((role) => role.uuid != uuid)
}
</script>
