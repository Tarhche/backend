<template>
  <div class="container">
    <div class="row justify-content-center ">
      <div class="col-md-12 col-lg-10">
        <div class="d-flex">
          <div class="w-100 d-none d-md-block" id="register-cover"></div>
          <div class="w-100 mt-3 mt-0 p-4">
              <h3 class="mb-4 text-center ">تکمیل ثبت نام</h3>
              <form action="#" class="signin-form d-flex flex-column" @submit.prevent="verify()">
                <div class="form-group my-2">
                  <input :class="{ 'is-invalid': errors.name }" type="text" placeholder="نام"
                         class="input form-control py-2 " v-model="params.name" required>
                  <div v-if="errors.name" class="invalid-feedback">
                    {{ errors.name }}
                  </div>
                </div>

                <div class="form-group my-2">
                  <input :class="{ 'is-invalid': errors.username }" type="text" placeholder="نام کاربری (یوزرنیم)"
                         class="input form-control py-2 " v-model="params.username" required>
                  <div v-if="errors.username" class="invalid-feedback">
                    {{ errors.username }}
                  </div>
                </div>

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
                <div v-if="params.succeed">
                  <p class="alert alert-success">
                    <span>ثبت نام شما انجام شد </span>
                    <span>وارد حساب کاربری خود شوید </span>
                    &nbsp;
                    <NuxtLink class="btn btn-outline-primary" to="/auth/login">صفحه ورود</NuxtLink>
                  </p>
                </div>
                <div class="form-group">
                  <button :disabled="params.loading || params.succeed" type="submit"
                          class="form-control btn btn-primary rounded submit px-3">
                    <span v-if="!params.loading">تکمیل ثبت نام</span>
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
  name: 'تکمیل ثبت نام',
  meta: [
    {name: 'description', content: 'تکمیل ثبت نام'},
  ],
  link: [
    {rel: 'canonical', href: `/auth/verify`}
  ]
})

// reflects form parameters
const params = reactive({
  token: null,
  name: null,
  username: null,
  password: null,
  repassword: null,
  loading: false,
  succeed: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
  name: null,
  username: null,
  password: null,
  repassword: null,
})

async function verify() {
  if (useRoute().query.token) {
    params.token = useRoute().query.token
  }

  // validate token
  if (!params.token) {
    errors.password = "توکن مرتبط با ثبت نام یافت نشد"

    return
  }

  // validate repassword
  if (params.password != params.repassword) {
    errors.repassword = "کلمه عبور و تکرار آن باید یکسان باشند"

    return
  }

  try {
    params.loading = true
    params.succeed = false
    await useAuth().verify(
        params.token,
        params.name,
        params.username,
        params.password,
        params.repassword
    )
    params.succeed = true
  } catch (error) {
    console.log(error)
    errors.username = "نام کاربری از قبل موجود است. یک نام کاربری دیگر انتخاب کنید"
  }

  params.loading = false
}
</script>


<style scoped>
#register-cover {
  background-image: url('/img/register-bg.jpg');
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
