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
            <li class="breadcrumb-item active" aria-current="page">کاربر ها</li>
          </ol>
        </nav>

        <div class="row">
          <div class="col-12 mb-4 mb-lg-0">
            <div class="card">
              <div class="card-header d-flex justify-content-between">
                <h4>کاربر ها</h4>
                <NuxtLink class="btn btn-primary" to="/dashboard/users/create">کاربر جدید</NuxtLink>
              </div>
              <div class="card-body">
                <div class="table-responsive">
                  <table class="table table-striped table-borderless table-hover align-middle">
                    <thead class="border-bottom">
                    <tr>
                      <th scope="col">#</th>
                      <th scope="col"></th>
                      <th scope="col">نام</th>
                      <th scope="col">ایمیل</th>
                      <th scope="col">نام کاربری</th>
                      <th scope="col">#</th>
                    </tr>
                    </thead>
                    <tbody v-if="!params.pending">
                    <tr v-for="(user, index) in params.data.items" :key="index">
                      <th scope="row">{{ index + 1 }}</th>
                      <td>
                        <img class="rounded" width="60" v-if="user.avatar" :src="useFilesUrlResolver().resolve(user.avatar)" />
                        <span v-else class="text-danger fa fa-times"></span>
                      </td>
                      <td>{{ user.name }}</td>
                      <td>
                        <span v-if="user.name">{{ user.email }}</span>
                        <span v-else class="text-danger fa fa-times"></span>
                      </td>
                      <td>
                        <span v-if="user.name">{{ user.username }}</span>
                        <span v-else class="text-danger fa fa-times"></span>
                      </td>
                      <td>
                        <NuxtLink :to="`/dashboard/users/edit/${user.uuid}`" class="btn mx-1 btn-sm btn-primary">
                          <span class="fa fa-pen"></span>
                        </NuxtLink>
                        <button @click.prevent="deleteUser(user.uuid)" type="button"
                                class="btn mx-1 btn-sm btn-danger">
                          <span class="fa fa-trash"></span>
                        </button>
                      </td>
                    </tr>
                    <tr v-if="params.data.items.length == 0">
                      <td colspan="6">
                        <p class="m-2">هیچ کاربری وجود ندارد</p>
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
  name: "کاربر ها"
})

const params = reactive({
  data: [],
  pending: true,
  error: null,
})

await load((useRoute().query.page) || 1)

async function load(page: number) {
  const {data, pending, error} = await useAsyncData(
      'dashboard.users.index',
      () => useDashboardUsers().index(page)
  )

  params.data = data
  params.pending = pending
  params.error = error
}

async function deleteUser(uuid: string) {
  if (!confirm('آیا میخواهید این کاربر را حذف کنید؟')) {
    return
  }

  await useDashboardUsers().delete(uuid)

  params.data.items = params.data.items.filter((user) => user.uuid != uuid)
}
</script>
