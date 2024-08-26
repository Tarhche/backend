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
              <NuxtLink to="/dashboard/comments">کامنت ها</NuxtLink>
            </li>
            <li class="breadcrumb-item active" aria-current="page">ویرایش</li>
          </ol>
        </nav>

        <div class="row">
          <div class="col-12 mb-4 mb-lg-0">
            <form class="card" action="#" @submit.prevent="updateComment()">
              <div class="card-header">ویرایش کامنت</div>
              <div class="card-body">
                <div class="form-floating mb-3">
                  <textarea :class="{ 'is-invalid': errors.body }" id="excerpt" class="form-control"
                            placeholder="محتوای کامنت" v-model="params.body" required></textarea>
                  <label for="excerpt">محتوای کامنت</label>
                  <div v-if="errors.body" class="invalid-feedback">
                    {{ errors.body }}
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
  name: "ویرایش کامنت"
})

// comment uuid
const {uuid} = useRoute().params

// reflects form parameters
const params = reactive({
  body: null,
  loading: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
  body: null,
})

await showComment()

async function showComment() {
  const ut = useTime()

  try {
    const data = await useDashboardMyComments().show(uuid)

    params.body = data.body
  } catch (error) {
    console.log(error)
  }
}

async function updateComment() {
  params.loading = true

  try {
    await useDashboardMyComments().update(
        uuid,
        params.body,
    )
  } catch (error) {
    console.log(error)
  }

  params.loading = false
}
</script>
