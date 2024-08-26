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
            <li class="breadcrumb-item active" aria-current="page">تنظیمات</li>
          </ol>
        </nav>

        <div class="row">
          <div class="col-12 mb-4 mb-lg-0">

            <form class="card" action="#" @submit.prevent="updateConfig()">
              <div class="card-header">ویرایش تنظیمات</div>
              <div class="card-body">
                <div class="row mb-3">
                  <label for="password" class="col-sm-2 col-form-label">نقش پیش فرض کاربران جدید</label>
                  <div class="col-sm-10">
                    <input :class="{ 'is-invalid': errors.userDefaultRoles }" class="form-control" type="text"
                           placeholder="نقش پیش فرض برای کاربران جدید" v-model="params.userDefaultRoles" aria-label="role uuids">
                    <div v-if="errors.userDefaultRoles" class="invalid-feedback">
                      {{ errors.userDefaultRoles }}
                    </div>
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
import {useDashboardConfig} from "~/composables/dashboardConfig";

definePageMeta({
  layout: 'dashboard',
})

useHead({
  name: "تنظیمات"
})

// role uuid
const {uuid} = useRoute().params

// reflects form parameters
const params = reactive({
  revision: null,
  userDefaultRoles: [],
  loading: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
  userDefaultRoles: null,
})

await showConfig()

function splitByComma(value) {
  if ((typeof value === 'string' || value instanceof String) && (value.length > 0)) {
    return value.split(',')
  }

  return []
}

async function showConfig() {
  try {
    const data = await useDashboardConfig().show()

    params.revision = data.revision
    params.userDefaultRoles = data.user_default_roles.join(',')
  } catch (error) {
    console.log(error)
  }
}

async function updateConfig() {
  params.loading = true

  try {
    await useDashboardConfig().update(
        splitByComma(params.userDefaultRoles),
    )
  } catch (error) {
    console.log(error)
  }

  params.loading = false
}
</script>
