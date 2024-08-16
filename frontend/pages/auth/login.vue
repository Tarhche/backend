<template>
  <div class="container">
    <div class="row justify-content-center ">
      <div class="col-md-12 col-lg-10">
        <div class="d-flex">
          <div class="w-100 d-none d-md-block" id="login-cover"></div>
          <div class="w-100 mt-3 mt-0 p-4">
            <h3 class="mb-4 text-center ">ورود به پنل کاربری</h3>
            <form action="#" class="signin-form d-flex flex-column" @submit.prevent="login()">
              <div class="form-group mt-2">
                <input :class="{ 'is-invalid': errors.identity }" type="text" placeholder="ایمیل یا نام کاربری"
                       class="input form-control py-2 " v-model="params.identity" required>
                <div v-if="errors.identity" class="invalid-feedback">
                  {{ errors.identity }}
                </div>
              </div>
              <div class="form-group my-4">
                <input :class="{ 'is-invalid': errors.password }" type="password" placeholder="کلمه عبور"
                       class="input form-control py-2 " v-model="params.password" required>
                <div v-if="errors.password" class="invalid-feedback">
                  {{ errors.password }}
                </div>
              </div>
              <div class="form-group">
                <button :disabled="params.loading" type="submit"
                        class="form-control btn btn-primary rounded submit px-3">
                  <span v-if="!params.loading">ورود</span>
                  <div v-else class="spinner-border" role="status">
                    <span class="visually-hidden">Loading...</span>
                  </div>
                </button>
              </div>
              <div class="form-group d-flex mt-2">
                <label class="d-flex align-items-center gap-1 checkbox-primary">
                  <input type="checkbox" checked>
                  <span>من را به خاطر بسپار</span>
                </label>
              </div>
              <div
                  class="form-group d-flex flex-column flex-md-row  mt-2 pt-2 justify-content-md-end gap-3 align-items-md-center">
                <div>
                  <nuxt-link to="/auth/forgot-password" class="btn btn-outline-danger w-100 btn-sm ">بازیابی کلمه عبور
                  </nuxt-link>
                </div>
                <div>
                  <nuxt-link to="/auth/register" class="btn btn-outline-success w-100 btn-sm ">ثبت نام</nuxt-link>
                </div>
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
  name: 'ورود به پنل کاربری',
  meta: [
    {name: 'description', content: 'ورود به پنل کاربری'},
  ],
  link: [
    {rel: 'canonical', href: `/auth/login`}
  ]
})

// reflects form parameters
const params = reactive({
  identity: null,
  password: null,
  loading: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
  identity: null,
  password: null,
})

async function login() {
  try {
    params.loading = true
    await useAuth().login(params.identity, params.password)
  } catch (error) {
    console.log(error)
    errors.identity = "نام کاربری یا کلمه عبور اشتباه است"
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
