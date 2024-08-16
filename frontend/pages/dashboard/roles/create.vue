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
            <li class="breadcrumb-item">
              <NuxtLink to="/dashboard/roles">نقش ها</NuxtLink>
            </li>
            <li class="breadcrumb-item active" aria-current="page">افزودن</li>
          </ol>
        </nav>

        <div class="row">
          <div class="col-12 mb-4 mb-lg-0">

            <form class="card" action="#" @submit.prevent="createRole()">
              <div class="card-header">افزودن نقش</div>
              <div class="card-body">
                <div class="form-floating mb-3">
                  <input :class="{ 'is-invalid': errors.name }" id="title" class="form-control" type="text"
                         placeholder="عنوان نقش" aria-label="title" v-model="params.name" required>
                  <label for="title">عنوان نقش</label>
                  <div v-if="errors.name" class="invalid-feedback">
                    {{ errors.name }}
                  </div>
                </div>

                <div class="form-floating mb-3">
                  <textarea :class="{ 'is-invalid': errors.description }" id="excerpt" class="form-control"
                            placeholder="توضیحات" v-model="params.description" required></textarea>
                  <label for="excerpt">توضیحات</label>
                  <div v-if="errors.description" class="invalid-feedback">
                    {{ errors.description }}
                  </div>
                </div>

                <div class="row m-0 mb-3">
                  <div v-if="errors.permissions" class="invalid-feedback">
                    {{ errors.permissions }}
                  </div>

                  <div class="form-check col-3" v-for="(permission, index) in params.loadedPermissions" :key="index">
                    <input :class="{ 'is-invalid': errors.permissions }" class="form-check-input" type="checkbox"
                           :value="permission.value" v-model="params.permissions" :id="`permission-${index}`">
                    <label class="form-check-label" :for="`permission-${index}`">{{ permission.name }}</label>
                  </div>
                </div>
                <div class="mb-3">
                  <input :class="{ 'is-invalid': errors.userUUIDs }" class="form-control" type="text"
                         placeholder="کاربرها" v-model="params.userUUIDs" aria-label="user uuids">
                  <div v-if="errors.userUUIDs" class="invalid-feedback">
                    {{ errors.userUUIDs }}
                  </div>
                </div>
              </div>
              <div class="card-footer">
                <button :disabled="params.loading" type="submit" class="btn btn-primary rounded submit px-3">
                  <span v-if="!params.loading">ذخیره کن</span>
                  <div v-else class="spinner-border" role="status">
                    <span class="visually-hidden">Loading...</span>
                  </div>
                </button>
              </div>
            </form>
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
  name: "افزودن نقش"
})

// reflects form parameters
const params = reactive({
  loadedPermissions: null,

  name: null,
  description: null,
  permissions: [],
  userUUIDs: null,
  loading: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
  name: null,
  description: null,
  permissions: null,
  userUUIDs: null,
})

await getPermissions()

function splitByComma(value) {
  if ((typeof value === 'string' || value instanceof String) && (value.length > 0)) {
    return value.split(',')
  }

  return []
}

async function createRole() {
  params.loading = true

  try {
    await useDashboardRoles().create(
        params.name,
        params.description,
        params.permissions,
        splitByComma(params.userUUIDs),
    )

    await navigateTo("/dashboard/roles")
  } catch (error) {
    console.log(error)
  }

  params.loading = false
}

async function getPermissions() {
  try {
    const data = await useDashboardPermissions().index()

    params.loadedPermissions = data.items
  } catch (error) {
    console.log(error)
  }
}
</script>
