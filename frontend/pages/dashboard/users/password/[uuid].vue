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
              <NuxtLink to="/dashboard/users">کاربرها</NuxtLink>
            </li>
            <li class="breadcrumb-item">
              <NuxtLink :to="`/dashboard/users/edit/${uuid}`">ویرایش کاربر</NuxtLink>
            </li>
            <li class="breadcrumb-item active" aria-current="page">تغییر کلمه عبور</li>
          </ol>
        </nav>

        <form @submit.prevent="updatePassword()" class="card">
          <div class="card-body">
            <div class="row mb-3">
              <label for="new_password" class="col-sm-2 col-form-label">کلمه عبور جدید</label>
              <div class="col-sm-10">
                <input :class="{ 'is-invalid': errors.newPassword }" type="password" placeholder="کلمه عبور جدید"
                       class="form-control" id="new_password" v-model="params.newPassword" required>
                <div v-if="errors.newPassword" class="invalid-feedback">
                  {{ errors.newPassword }}
                </div>
              </div>
            </div>

            <div class="row mb-3">
              <label for="repassword" class="col-sm-2 col-form-label">تکرار کلمه عبور جدید</label>
              <div class="col-sm-10">
                <input :class="{ 'is-invalid': errors.newRePassword }" type="password"
                       placeholder="تکرار کلمه عبور جدید" class="form-control" id="repassword"
                       v-model="params.newRePassword" required>
                <div v-if="errors.newRePassword" class="invalid-feedback">
                  {{ errors.newRePassword }}
                </div>
              </div>
            </div>
          </div>
          <div class="card-footer">
            <button :disabled="params.loading" type="submit" class="btn btn-primary rounded submit px-3">
              <span v-if="!params.loading">تغییر کلمه عبور</span>
              <div v-else class="spinner-border" role="status">
                <span class="visually-hidden">Loading...</span>
              </div>
            </button>
          </div>
        </form>

      </main>
    </div>
  </div>
</template>

<script lang="ts" setup>
definePageMeta({
  layout: 'dashboard',
})

useHead({
  name: "تغییر کلمه عبور کاربر"
})

// user's uuid
const {uuid} = useRoute().params

// reflects form parameters
const params = reactive({
  newPassword: null,
  newRePassword: null,
  loading: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
  newPassword: null,
  newRePassword: null,
})

function resetErrors() {
  errors.newPassword = null
  errors.newRePassword = null
}

async function updatePassword() {
  resetErrors()

  if (!params.newPassword || params.newPassword.length == 0) {
    errors.newPassword = "پسوورد جدید را وارد کنید"

    return
  }

  if (params.newRePassword != params.newPassword) {
    errors.newRePassword = "پسوورد جدید و تکرار آن باید یکسان باشند"

    return
  }

  params.loading = true

  try {
    await useDashboardUsers().updatePassword(uuid, params.newPassword)

    params.newPassword = null;
    params.newRePassword = null;
  } catch (error) {
    console.log(error)
  }

  params.loading = false
}
</script>
