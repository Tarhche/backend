<template>
  <div class="container">
    <div class="row justify-content-center ">
      <div class="col-md-12 col-lg-10">
        <div class="d-flex">
          <div class="w-100 d-none d-md-block" id="login-cover"></div>
          <div class="w-100 mt-3 mt-0 p-4">
            <h3 class="mb-4 text-center ">تغییر کلمه عبور</h3>
            <form action="#" class="signin-form d-flex flex-column" @submit.prevent="resetPassword()">
              <div class="form-group my-2">
                <input :class="{ 'is-invalid': errors.password }" type="text" placeholder="کلمه عبور جدید"
                       class="input form-control py-2 " v-model="params.password" required>
                <div v-if="errors.password" class="invalid-feedback">
                  {{ errors.password }}
                </div>
              </div>
              <div class="form-group my-2">
                <input :class="{ 'is-invalid': errors.repassword }" type="text" placeholder="تکرار کلمه عبور جدید"
                       class="input form-control py-2 " v-model="params.repassword" required>
                <div v-if="errors.repassword" class="invalid-feedback">
                  {{ errors.repassword }}
                </div>
              </div>
              <div class="form-group">
                <p v-if="params.succeed" class="text-info">
                  <span>کلمه عبور شما تغییر کرد </span>
                  <span>میتوانید با استفاده از کلمه عبور خود از طریق </span>
                  &nbsp;
                  <NuxtLink class="btn btn-outline-primary" to="/auth/login">صفحه ورود</NuxtLink>
                  &nbsp;
                  <span>وارد حساب کاربری خود شوید </span>
                </p>
                <button :disabled="params.loading" type="submit"
                        class="form-control btn btn-primary rounded submit px-3">
                  <span v-if="!params.loading">ذخیره کن</span>
                  <div v-else class="spinner-border" role="status">
                    <span class="visually-hidden">Loading...</span>
                  </div>
                </button>
              </div>
            </form>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
useHead({
  name: 'تغییر کلمه عبور',
  meta: [
    {name: 'description', content: 'تغییر کلمه عبور'},
  ],
  link: [
    {rel: 'canonical', href: `/auth/reset-password`}
  ]
})

// reflects form parameters
const params = reactive({
  token: null,
  password: null,
  repassword: null,
  loading: false,
  succeed: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
  password: null,
  repassword: null,
})

async function resetPassword() {
  if (useRoute().query.token) {
    params.token = useRoute().query.token
  }

  // validate token
  if (!params.token) {
    errors.password = "توکن مرتبط با بازیابی کلمه عبور یافت نشد"

    return
  }

  // validate repassword
  if (params.password != params.repassword) {
    errors.repassword = "کلمه عبور جدید و تکرار آن باید یکسان باشند"

    return
  }

  try {
    params.loading = true
    params.succeed = false
    await useAuth().resetPassword(params.token, params.password)
    params.succeed = true
  } catch (error) {
    console.log(error)
    errors.password = "توکن بازیابی کلمه عبور منقضی شده است"
  }

  params.loading = false
}
</script>


<style scoped>
#login-cover {
  background-image: url('/img/login-bg.jpg');
  background-size: cover;
  background-repeat: no-repeat;
  background-position: center top;
  overflow: hidden;
  border-radius: 3px;
}

h3 {
  color: #313131;
}

input::placeholder {
  color: #9a9999;
  font-size: 0.9rem;
}
</style>