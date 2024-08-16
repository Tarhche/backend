<template>
  <div class="container">
    <div class="row justify-content-center ">
      <div class="col-md-12 col-lg-10">
        <div class="d-flex">
          <div class="w-100 d-none d-md-block" id="login-cover"></div>
          <div class="w-100 mt-3 mt-0 p-4">
            <h3 class="mb-4 text-center ">بازیابی کلمه عبور</h3>
            <form action="#" class="signin-form d-flex flex-column" @submit.prevent="forgotPassword()">
              <div class="form-group my-2">
                <input :class="{ 'is-invalid': errors.identity }" type="text" placeholder="ایمیل یا نام کاربری"
                       class="input form-control py-2 " v-model="params.identity" required>
                <div v-if="errors.identity" class="invalid-feedback">
                  {{ errors.identity }}
                </div>
              </div>
              <div class="form-group">
                <p v-if="params.succeed" class="text-success">لینک بازیابی کلمه عبور به ایمیل شما ارسال شد</p>
                <button :disabled="params.loading" type="submit"
                        class="form-control btn btn-primary rounded submit px-3">
                  <span v-if="!params.loading">درخواست بازیابی</span>
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
  name: 'بازیابی کلمه عبور',
  meta: [
    {name: 'description', content: 'بازیابی کلمه عبور'},
  ],
  link: [
    {rel: 'canonical', href: `/auth/forgot-password`}
  ]
})

// reflects form parameters
const params = reactive({
  identity: null,
  loading: false,
  succeed: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
  identity: null,
})

async function forgotPassword() {
  try {
    params.loading = true
    params.succeed = false
    await useAuth().forgotPassword(params.identity)
    params.succeed = true
  } catch (error) {
    console.log(error)
    errors.identity = "نام کاربری یا ایمیل وارد شده مورد قبول نیست"
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